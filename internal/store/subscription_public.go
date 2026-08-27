// SPDX-License-Identifier: GPL-3.0-or-later

package store

import (
	"context"
	"database/sql"
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

// PublicSubscriptionState is the minimum immutable state needed to serve one
// public subscription. It intentionally omits token identity and digest.
type PublicSubscriptionState struct {
	AppliedBundleID        string
	SubscriptionSnapshotID string
	Content                []byte
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

// LoadPublicSubscriptionState authenticates a plaintext-derived digest and
// resolves the applied publication in one SQL statement. The query reads
// applied_bundle_id exactly once and joins only through immutable bundle and
// snapshot rows; it never consults current canonical, channel, source, or
// address state.
func (s *Store) LoadPublicSubscriptionState(
	ctx context.Context,
	tokenSHA256 string,
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

	var appliedBundleID, snapshotID, content sql.NullString
	var expiresAt, revokedAt sql.NullString
	err = s.db.QueryRowContext(
		ctx,
		`SELECT t.expires_at, t.revoked_at,
                h.applied_bundle_id, a.subscription_snapshot_id, s.content_json
           FROM subscription_tokens AS t
           JOIN hub_state AS h ON h.singleton = 1
           LEFT JOIN activation_bundles AS a ON a.id = h.applied_bundle_id
           LEFT JOIN subscription_snapshots AS s ON s.id = a.subscription_snapshot_id
          WHERE t.token_sha256 = ?`,
		digest,
	).Scan(&expiresAt, &revokedAt, &appliedBundleID, &snapshotID, &content)
	if errors.Is(err, sql.ErrNoRows) {
		return PublicSubscriptionState{}, ErrSubscriptionTokenNotFound
	}
	if err != nil {
		return PublicSubscriptionState{}, fmt.Errorf("load public subscription state: %w", err)
	}
	if revokedAt.Valid {
		return PublicSubscriptionState{}, ErrSubscriptionTokenInactive
	}
	if expiresAt.Valid {
		expires, parseErr := parseTaskTime(expiresAt.String)
		if parseErr != nil {
			return PublicSubscriptionState{}, fmt.Errorf("parse subscription token expires_at: %w", parseErr)
		}
		if !at.Before(expires) {
			return PublicSubscriptionState{}, ErrSubscriptionTokenInactive
		}
	}
	if !appliedBundleID.Valid || appliedBundleID.String == "" {
		return PublicSubscriptionState{}, ErrNoAppliedBundle
	}
	if !snapshotID.Valid || snapshotID.String == "" || !content.Valid {
		return PublicSubscriptionState{}, errors.New("applied subscription snapshot is inconsistent")
	}
	return PublicSubscriptionState{
		AppliedBundleID:        appliedBundleID.String,
		SubscriptionSnapshotID: snapshotID.String,
		Content:                []byte(content.String),
	}, nil
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
		inputBytes := int64(len(source.Config) + len(source.LatestSnapshot))
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
