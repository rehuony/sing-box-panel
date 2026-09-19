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

// SubscriptionPreparationInputs is a point-in-time copy of the mutable
// publication controls. Rendering happens only after the short read
// transaction that produced this value has ended.
type SubscriptionPreparationInputs struct {
	Channels []SubscriptionChannel
	Sources  []SubscriptionSource
}

type SubscriptionPreparationLimits struct {
	MaximumChannels   int
	MaximumSources    int
	MaximumInputBytes int64
}

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
// transaction as the applied local startup version.
type PublicSubscriptionState struct {
	SubscriptionNodeControls
	TokenID         string
	UserID          string
	AppliedBundleID string
	Channel         SubscriptionChannel
	Startup         StartupArtifact
	Core            CoreArtifact
	Grants          []string
	Sources         []PublicSubscriptionSourceVersion
}

type SubscriptionNodeCatalogState struct {
	SubscriptionNodeControls
	AppliedBundleID string
	Startup         StartupArtifact
	Core            CoreArtifact
	Sources         []PublicSubscriptionSourceVersion
}

func (s *Store) LoadSubscriptionNodeCatalogState(ctx context.Context) (SubscriptionNodeCatalogState, error) {
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return SubscriptionNodeCatalogState{}, fmt.Errorf("begin subscription node catalog read: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	var state SubscriptionNodeCatalogState
	state.AppliedBundleID, state.Startup, state.Core, err = loadAppliedSubscriptionCore(ctx, tx)
	if err != nil && !errors.Is(err, ErrNoAppliedBundle) {
		return state, err
	}
	state.Sources, err = loadSubscriptionSourceVersions(ctx, tx, false)
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

// LoadSubscriptionPreparationInputs takes one short, consistent database
// snapshot of all enabled channels and sources. No rendering, network, or
// filesystem operation occurs while the transaction is open.
func (s *Store) LoadSubscriptionPreparationInputs(
	ctx context.Context,
	limits SubscriptionPreparationLimits,
) (SubscriptionPreparationInputs, error) {
	if s == nil || s.db == nil {
		return SubscriptionPreparationInputs{}, errors.New("SQLite store is not open")
	}
	if limits.MaximumChannels < 1 || limits.MaximumSources < 1 || limits.MaximumInputBytes < 1 {
		return SubscriptionPreparationInputs{}, errors.New("subscription preparation limits are invalid")
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return SubscriptionPreparationInputs{}, fmt.Errorf("begin subscription input read: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	remainingBytes := limits.MaximumInputBytes
	channels, err := listEnabledSubscriptionChannels(ctx, tx, limits.MaximumChannels, &remainingBytes)
	if err != nil {
		return SubscriptionPreparationInputs{}, err
	}
	sources, err := listEnabledSubscriptionSources(ctx, tx, limits.MaximumSources, &remainingBytes)
	if err != nil {
		return SubscriptionPreparationInputs{}, err
	}
	if err := tx.Commit(); err != nil {
		return SubscriptionPreparationInputs{}, fmt.Errorf("commit subscription input read: %w", err)
	}
	return SubscriptionPreparationInputs{Channels: channels, Sources: sources}, nil
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
	requireEnabledChannel bool,
) error {
	var err error
	state.Channel, err = getSubscriptionChannel(ctx, tx, channelID)
	if err != nil || requireEnabledChannel && !state.Channel.Enabled {
		return ErrSubscriptionChannelNotFound
	}
	state.AppliedBundleID, state.Startup, state.Core, err = loadAppliedSubscriptionCore(ctx, tx)
	if err != nil && (!errors.Is(err, ErrNoAppliedBundle) || state.UserID != "") {
		return err
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
	state.SubscriptionNodeControls, err = loadSubscriptionNodeControls(ctx, tx)
	if err != nil {
		return err
	}
	if state.AppliedBundleID == "" && len(state.Sources) == 0 && len(state.ManualNodes) == 0 {
		return ErrNoAppliedBundle
	}
	return nil
}

func loadAppliedSubscriptionCore(ctx context.Context, tx *sql.Tx) (string, StartupArtifact, CoreArtifact, error) {
	var id sql.NullString
	if err := tx.QueryRowContext(ctx, `SELECT applied_bundle_id FROM hub_state WHERE singleton = 1`).Scan(&id); err != nil {
		return "", StartupArtifact{}, CoreArtifact{}, err
	}
	if !id.Valid || id.String == "" {
		return "", StartupArtifact{}, CoreArtifact{}, ErrNoAppliedBundle
	}
	bundle, err := getActivationBundle(ctx, tx, id.String)
	if err != nil {
		return "", StartupArtifact{}, CoreArtifact{}, err
	}
	startup, err := getStartupArtifact(ctx, tx, bundle.StartupArtifactID)
	if err != nil {
		return "", StartupArtifact{}, CoreArtifact{}, err
	}
	core, err := getCoreArtifact(ctx, tx, startup.CoreArtifactID)
	return id.String, startup, core, err
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

func listEnabledSubscriptionChannels(
	ctx context.Context,
	tx *sql.Tx,
	maximum int,
	remainingBytes *int64,
) ([]SubscriptionChannel, error) {
	rows, err := tx.QueryContext(
		ctx,
		`SELECT `+subscriptionChannelColumns+`
           FROM subscription_channels
          WHERE enabled = 1
	          ORDER BY id ASC
	          LIMIT ?`,
		maximum+1,
	)
	if err != nil {
		return nil, fmt.Errorf("list enabled subscription channels: %w", err)
	}
	defer rows.Close()

	channels := make([]SubscriptionChannel, 0)
	for rows.Next() {
		channel, scanErr := scanSubscriptionChannel(rows)
		if scanErr != nil {
			return nil, fmt.Errorf("scan enabled subscription channel: %w", scanErr)
		}
		if len(channels) >= maximum {
			return nil, fmt.Errorf("%w: too many enabled channels", ErrSubscriptionLimitExceeded)
		}
		if int64(len(channel.Config)) > *remainingBytes {
			return nil, fmt.Errorf("%w: enabled inputs exceed byte budget", ErrSubscriptionLimitExceeded)
		}
		*remainingBytes -= int64(len(channel.Config))
		channels = append(channels, channel)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate enabled subscription channels: %w", err)
	}
	return channels, nil
}

func listEnabledSubscriptionSources(
	ctx context.Context,
	tx *sql.Tx,
	maximum int,
	remainingBytes *int64,
) ([]SubscriptionSource, error) {
	rows, err := tx.QueryContext(
		ctx,
		`SELECT `+subscriptionSourceColumns+`
           FROM subscription_sources
          WHERE enabled = 1
	          ORDER BY id ASC
	          LIMIT ?`,
		maximum+1,
	)
	if err != nil {
		return nil, fmt.Errorf("list enabled subscription sources: %w", err)
	}
	defer rows.Close()

	sources := make([]SubscriptionSource, 0)
	for rows.Next() {
		source, scanErr := scanSubscriptionSource(rows)
		if scanErr != nil {
			return nil, fmt.Errorf("scan enabled subscription source: %w", scanErr)
		}
		if len(sources) >= maximum {
			return nil, fmt.Errorf("%w: too many enabled sources", ErrSubscriptionLimitExceeded)
		}
		inputBytes := int64(len(source.Config))
		if inputBytes > *remainingBytes {
			return nil, fmt.Errorf("%w: enabled inputs exceed byte budget", ErrSubscriptionLimitExceeded)
		}
		*remainingBytes -= inputBytes
		sources = append(sources, source)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate enabled subscription sources: %w", err)
	}
	return sources, nil
}
