// SPDX-License-Identifier: GPL-3.0-or-later

package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// PublicSubscriptionState is one point-in-time authorization and publication
// view: the key and its user, the channel and the pool nodes it may publish,
// read in one transaction.
type PublicSubscriptionState struct {
	TokenID string
	UserID  string
	Channel SubscriptionChannel
	// Pool is the pool in its saved order, only the user's granted nodes for
	// a user-scoped key.
	Pool []SubscriptionPoolNode
}

// ErrNoAppliedBundle means there is nothing to publish: the pool is empty.
var ErrNoAppliedBundle = errors.New("no applied configuration is available")

// NodePublication is one node's applied configuration, the input of its
// inbounds in the pool. Document is the panel's revision the node reports
// running, nil when the panel no longer holds it.
type NodePublication struct {
	NodeID           string
	NodeName         string
	PublicHost       string
	ReportedPublicIP string
	BundleID         string
	ExactCoreVersion string
	ConfigSHA256     string
	RevisionID       string
	Document         []byte
}

// LoadNodePublications reads every node that reports an applied bundle, with
// the revision it runs, ordered by name.
func (s *Store) LoadNodePublications(ctx context.Context) ([]NodePublication, error) {
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	publications, err := loadNodePublications(ctx, tx)
	if err != nil {
		return nil, err
	}
	return publications, tx.Commit()
}

func loadNodePublications(ctx context.Context, tx *sql.Tx) ([]NodePublication, error) {
	rows, err := tx.QueryContext(ctx, `SELECT n.id, n.name, n.public_host, n.reported_public_ip, n.applied_bundle_id,
           coalesce(n.applied_core_version, ''), coalesce(n.applied_config_sha256, ''), coalesce(n.applied_revision_id, ''), r.document_json
         FROM nodes AS n
         LEFT JOIN canonical_revisions AS r ON r.id = n.applied_revision_id AND r.node_id = n.id
         WHERE n.applied_bundle_id IS NOT NULL
         ORDER BY n.name COLLATE NOCASE, n.id`)
	if err != nil {
		return nil, fmt.Errorf("read node publications: %w", err)
	}
	defer rows.Close()
	publications := []NodePublication{}
	for rows.Next() {
		var value NodePublication
		var document sql.NullString
		if err := rows.Scan(&value.NodeID, &value.NodeName, &value.PublicHost, &value.ReportedPublicIP, &value.BundleID,
			&value.ExactCoreVersion, &value.ConfigSHA256, &value.RevisionID, &document); err != nil {
			return nil, err
		}
		if document.Valid {
			value.Document = []byte(document.String)
		}
		publications = append(publications, value)
	}
	return publications, rows.Err()
}

// LoadPublicSubscriptionState authenticates the token and assembles its
// channel and pool in one consistent SQLite read.
func (s *Store) LoadPublicSubscriptionState(
	ctx context.Context,
	tokenSHA256 string,
	channelID string,
	at time.Time,
) (PublicSubscriptionState, error) {
	digest, err := normalizeTokenDigest(tokenSHA256)
	if err != nil {
		return PublicSubscriptionState{}, err
	}
	at, err = requiredUTC(at, "token validation time")
	if err != nil {
		return PublicSubscriptionState{}, err
	}
	if err := validateSubscriptionID(channelID, "channel"); err != nil {
		return PublicSubscriptionState{}, ErrSubscriptionChannelNotFound
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return PublicSubscriptionState{}, fmt.Errorf("begin public subscription read: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var state PublicSubscriptionState
	token, err := scanSubscriptionToken(tx.QueryRowContext(ctx, `SELECT
            t.id, t.user_id, t.label, t.token_sha256, t.enabled, t.expires_at, t.revoked_at,
            t.successful_request_count, t.body_response_count, t.bytes_served,
            t.last_used_at, t.created_at, t.download_limit
        FROM subscription_tokens AS t
        LEFT JOIN subscription_users AS u ON u.id = t.user_id
        WHERE t.token_sha256 = ? AND (t.user_id IS NULL OR u.enabled = 1)`, digest))
	if errors.Is(err, sql.ErrNoRows) {
		return PublicSubscriptionState{}, ErrSubscriptionTokenNotFound
	}
	if err != nil {
		return PublicSubscriptionState{}, fmt.Errorf("authenticate public subscription token: %w", err)
	}
	if !token.Active(at) {
		return PublicSubscriptionState{}, ErrSubscriptionTokenInactive
	}
	state.TokenID, state.UserID = token.ID, token.UserID
	if err := loadSubscriptionPublicationState(ctx, tx, &state, channelID, true); err != nil {
		return PublicSubscriptionState{}, err
	}
	if err := tx.Commit(); err != nil {
		return PublicSubscriptionState{}, fmt.Errorf("commit public subscription read: %w", err)
	}
	return state, nil
}

// LoadSubscriptionPreviewState assembles the same view as a public request,
// starting from an administrator-selected enabled user, or none for the
// channel itself, instead of a token.
func (s *Store) LoadSubscriptionPreviewState(ctx context.Context, userID, channelID string) (PublicSubscriptionState, error) {
	if userID != "" {
		if err := validateSubscriptionID(userID, "user"); err != nil {
			return PublicSubscriptionState{}, ErrSubscriptionUserNotFound
		}
	}
	if err := validateSubscriptionID(channelID, "channel"); err != nil {
		return PublicSubscriptionState{}, ErrSubscriptionChannelNotFound
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return PublicSubscriptionState{}, fmt.Errorf("begin subscription preview read: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	var state PublicSubscriptionState
	if userID != "" {
		if err := tx.QueryRowContext(ctx, `SELECT id FROM subscription_users WHERE id = ? AND enabled = 1`, userID).Scan(&state.UserID); errors.Is(err, sql.ErrNoRows) {
			return PublicSubscriptionState{}, ErrSubscriptionUserNotFound
		} else if err != nil {
			return PublicSubscriptionState{}, err
		}
	}
	if err := loadSubscriptionPublicationState(ctx, tx, &state, channelID, false); err != nil {
		return PublicSubscriptionState{}, err
	}
	if err := tx.Commit(); err != nil {
		return PublicSubscriptionState{}, fmt.Errorf("commit subscription preview read: %w", err)
	}
	return state, nil
}

func loadSubscriptionPublicationState(
	ctx context.Context,
	tx *sql.Tx,
	state *PublicSubscriptionState,
	channelID string,
	publicDelivery bool,
) error {
	var err error
	state.Channel, err = getSubscriptionChannel(ctx, tx, channelID)
	if errors.Is(err, ErrSubscriptionChannelNotFound) || err == nil && publicDelivery && !state.Channel.Enabled {
		return ErrSubscriptionChannelNotFound
	}
	if err != nil {
		return err
	}
	pool, err := loadSubscriptionPool(ctx, tx)
	if err != nil {
		return err
	}
	// Management preview can validate a draft before the pool has nodes;
	// public delivery has nothing to publish.
	if publicDelivery && len(pool) == 0 {
		return ErrNoAppliedBundle
	}
	if state.UserID == "" {
		state.Pool = pool
		return nil
	}
	granted, err := loadSubscriptionUserGrants(ctx, tx, state.UserID)
	if err != nil {
		return err
	}
	allowed := make(map[string]bool, len(granted))
	for _, id := range granted {
		allowed[id] = true
	}
	state.Pool = []SubscriptionPoolNode{}
	for _, node := range pool {
		if allowed[node.ID] {
			state.Pool = append(state.Pool, node)
		}
	}
	return nil
}
