// SPDX-License-Identifier: GPL-3.0-or-later

package store

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

type PublicSubscriptionSourceVersion struct {
	SourceKind      SubscriptionSourceKind
	Name            string
	Enabled         bool
	SourceID        string
	VersionID       string
	NormalizedNodes json.RawMessage
}

// PublicSubscriptionState is one point-in-time authorization and publication
// view. Mutable channel, user, grants, and source pointers are read in the same
// transaction as every node's applied configuration.
type PublicSubscriptionState struct {
	SubscriptionNodeControls
	TokenID string
	UserID  string
	Channel SubscriptionChannel
	Nodes   []NodePublication
	Grants  []string
	Sources []PublicSubscriptionSourceVersion
}

type SubscriptionNodeCatalogState struct {
	SubscriptionNodeControls
	Nodes   []NodePublication
	Sources []PublicSubscriptionSourceVersion
}

// ErrNoAppliedBundle means there is nothing to publish: no node with an
// applied configuration, source or manual node.
var ErrNoAppliedBundle = errors.New("no applied configuration is available")

// NodePublication is one node's applied configuration, the input of its
// local inbounds. Document is the panel's revision the node reports running,
// nil when the panel no longer holds it.
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

// loadNodePublications reads every node that reports an applied bundle,
// with the revision it runs.
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

func (s *Store) LoadSubscriptionNodeCatalogState(ctx context.Context) (SubscriptionNodeCatalogState, error) {
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return SubscriptionNodeCatalogState{}, fmt.Errorf("begin subscription node catalog read: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	var state SubscriptionNodeCatalogState
	state.Sources, err = loadSubscriptionSourceVersions(ctx, tx, false)
	if err != nil {
		return state, err
	}
	state.Nodes, err = loadNodePublications(ctx, tx)
	if err != nil {
		return state, err
	}
	state.SubscriptionNodeControls, err = loadSubscriptionNodeControls(ctx, tx)
	if err != nil {
		return state, err
	}
	if err := tx.Commit(); err != nil {
		return SubscriptionNodeCatalogState{}, err
	}
	return state, nil
}

// LoadPublicSubscriptionState authenticates the token and assembles all live
// authorization and version pointers in one consistent SQLite read.
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

// LoadSubscriptionPreviewState assembles the same live view as a public
// request, but starts from an administrator-selected enabled user instead of a
// token. It is one read transaction so the preview cannot mix versions.
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
	if err != nil || publicDelivery && !state.Channel.Enabled {
		return ErrSubscriptionChannelNotFound
	}

	grantRows, err := tx.QueryContext(ctx, `SELECT node_key FROM subscription_user_node_grants WHERE user_id = ? ORDER BY node_key`, state.UserID)
	if err != nil {
		return fmt.Errorf("read public subscription grants: %w", err)
	}
	for grantRows.Next() {
		var key string
		if err := grantRows.Scan(&key); err != nil {
			_ = grantRows.Close()
			return err
		}
		state.Grants = append(state.Grants, key)
	}
	if err := grantRows.Close(); err != nil {
		return err
	}
	if err := grantRows.Err(); err != nil {
		return err
	}

	state.Sources, err = loadSubscriptionSourceVersions(ctx, tx, true)
	if err != nil {
		return err
	}
	state.Nodes, err = loadNodePublications(ctx, tx)
	if err != nil {
		return err
	}
	state.SubscriptionNodeControls, err = loadSubscriptionNodeControls(ctx, tx)
	if err != nil {
		return err
	}
	// Management preview can validate a draft before the first source exists.
	// Public delivery retains its existing publication availability boundary.
	if publicDelivery && len(state.Nodes) == 0 && len(state.Sources) == 0 && len(state.ManualNodes) == 0 {
		return ErrNoAppliedBundle
	}
	return nil
}

func loadSubscriptionSourceVersions(ctx context.Context, tx *sql.Tx, enabledOnly bool) ([]PublicSubscriptionSourceVersion, error) {
	query := `SELECT s.id, s.name, s.source_kind, s.enabled, v.id, v.normalized_nodes_json FROM subscription_sources AS s
        JOIN subscription_source_versions AS v ON v.id = s.current_version_id AND v.source_id = s.id`
	if enabledOnly {
		query += ` WHERE s.enabled = 1`
	}
	query += ` ORDER BY s.created_at, s.id`
	rows, err := tx.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	sources := []PublicSubscriptionSourceVersion{}
	var total int64
	for rows.Next() {
		var source PublicSubscriptionSourceVersion
		var raw []byte
		if err := rows.Scan(&source.SourceID, &source.Name, &source.SourceKind, &source.Enabled, &source.VersionID, &raw); err != nil {
			return nil, err
		}
		total += int64(len(raw))
		if total > MaximumSubscriptionInputBytes {
			return nil, ErrSubscriptionLimitExceeded
		}
		source.NormalizedNodes = bytes.Clone(raw)
		sources = append(sources, source)
	}
	return sources, rows.Err()
}
