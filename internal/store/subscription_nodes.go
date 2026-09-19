// SPDX-License-Identifier: GPL-3.0-or-later

package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"github.com/rehuony/sing-box-panel/internal/subscription"
)

var (
	ErrSubscriptionNodeNotFound = errors.New("subscription node not found")
	ErrSubscriptionNodeInvalid  = errors.New("invalid subscription node")
)

const MaximumManualNodeBytes = 256 << 10

type ManualSubscriptionNode struct {
	ID        string
	Revision  int64
	Outbound  json.RawMessage
	CreatedAt time.Time
	UpdatedAt time.Time
}

type SubscriptionNodeVisibility struct {
	Hidden   bool
	Revision int64
}

// Node controls are copied in the same transaction as source versions and core
// identity. Rendering must not mix visibility from a different publication view.
type SubscriptionNodeControls struct {
	ManualNodes []ManualSubscriptionNode
	Visibility  map[string]SubscriptionNodeVisibility
}

func (s *Store) SaveManualSubscriptionNode(ctx context.Context, node ManualSubscriptionNode, expected int64) (ManualSubscriptionNode, error) {
	if validateSubscriptionID(node.ID, "node") != nil || expected < 0 || len(node.Outbound) > MaximumManualNodeBytes {
		return ManualSubscriptionNode{}, ErrSubscriptionNodeInvalid
	}
	parsed, _, err := subscription.ParseSource(subscription.SourceFormatSingBoxJSON, node.Outbound, "manual")
	if err != nil || len(parsed) != 1 {
		return ManualSubscriptionNode{}, ErrSubscriptionNodeInvalid
	}
	node.Outbound = parsed[0].Outbound
	now := node.UpdatedAt.UTC()
	if now.IsZero() {
		now = time.Now().UTC()
	}
	err = s.WithTx(ctx, func(tx *sql.Tx) error {
		current, err := getManualSubscriptionNode(ctx, tx, node.ID)
		if err != nil && !errors.Is(err, ErrSubscriptionNodeNotFound) {
			return err
		}
		if expected > 0 && errors.Is(err, ErrSubscriptionNodeNotFound) {
			return err
		}
		if current.Revision != expected {
			return ErrSubscriptionConflict
		}
		var count, size int64
		if err := tx.QueryRowContext(ctx, `SELECT count(*), coalesce(sum(length(CAST(outbound_json AS BLOB))),0) FROM subscription_manual_nodes WHERE id <> ?`, node.ID).Scan(&count, &size); err != nil {
			return err
		}
		if count >= subscription.MaximumNodes || size+int64(len(node.Outbound)) > MaximumSubscriptionInputBytes {
			return ErrSubscriptionLimitExceeded
		}
		if err := validateManualDependencies(ctx, tx, node); err != nil {
			return err
		}
		node.Revision = expected + 1
		node.CreatedAt = current.CreatedAt
		if expected == 0 {
			node.CreatedAt = now
		}
		node.UpdatedAt = now
		_, err = tx.ExecContext(ctx, `INSERT INTO subscription_manual_nodes(id, revision, outbound_json, created_at, updated_at) VALUES(?,?,?,?,?)
            ON CONFLICT(id) DO UPDATE SET revision=excluded.revision, outbound_json=excluded.outbound_json, updated_at=excluded.updated_at`,
			node.ID, node.Revision, string(node.Outbound), formatTaskTime(node.CreatedAt), formatTaskTime(now))
		return err
	})
	return node, err
}

func getManualSubscriptionNode(ctx context.Context, q queryRower, id string) (ManualSubscriptionNode, error) {
	value, err := scanManualSubscriptionNode(q.QueryRowContext(ctx, `SELECT id, revision, outbound_json, created_at, updated_at FROM subscription_manual_nodes WHERE id=?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return ManualSubscriptionNode{}, ErrSubscriptionNodeNotFound
	}
	return value, err
}

func scanManualSubscriptionNode(row taskScanner) (ManualSubscriptionNode, error) {
	var node ManualSubscriptionNode
	var raw, created, updated string
	if err := row.Scan(&node.ID, &node.Revision, &raw, &created, &updated); err != nil {
		return node, err
	}
	node.Outbound = json.RawMessage(raw)
	var err error
	node.CreatedAt, err = parseTaskTime(created)
	if err != nil {
		return node, err
	}
	node.UpdatedAt, err = parseTaskTime(updated)
	return node, err
}

func (s *Store) DeleteManualSubscriptionNode(ctx context.Context, id string, expected int64, publicationID string) error {
	if expected < 1 {
		return ErrSubscriptionNodeInvalid
	}
	return s.WithTx(ctx, func(tx *sql.Tx) error {
		node, err := getManualSubscriptionNode(ctx, tx, id)
		if err != nil {
			return err
		}
		if node.Revision != expected {
			return ErrSubscriptionConflict
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM subscription_manual_nodes WHERE id=?`, id); err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx, `DELETE FROM subscription_node_visibility WHERE publication_id=?`, publicationID)
		return err
	})
}

func (s *Store) SetSubscriptionNodeVisibility(ctx context.Context, id string, hidden bool, expected int64) (SubscriptionNodeVisibility, error) {
	if !subscription.ValidKey(id) || expected < 0 {
		return SubscriptionNodeVisibility{}, ErrSubscriptionNodeInvalid
	}
	var result SubscriptionNodeVisibility
	err := s.WithTx(ctx, func(tx *sql.Tx) error {
		var current int64
		err := tx.QueryRowContext(ctx, `SELECT revision FROM subscription_node_visibility WHERE publication_id=?`, id).Scan(&current)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		if current != expected {
			return ErrSubscriptionConflict
		}
		result = SubscriptionNodeVisibility{Hidden: hidden, Revision: current + 1}
		_, err = tx.ExecContext(ctx, `INSERT INTO subscription_node_visibility(publication_id, hidden, revision) VALUES(?,?,?)
            ON CONFLICT(publication_id) DO UPDATE SET hidden=excluded.hidden, revision=excluded.revision`, id, boolInt(hidden), result.Revision)
		return err
	})
	return result, err
}

func loadSubscriptionNodeControls(ctx context.Context, tx *sql.Tx) (SubscriptionNodeControls, error) {
	value := SubscriptionNodeControls{Visibility: make(map[string]SubscriptionNodeVisibility), ManualNodes: []ManualSubscriptionNode{}}
	rows, err := tx.QueryContext(ctx, `SELECT id, revision, outbound_json, created_at, updated_at FROM subscription_manual_nodes ORDER BY created_at, id`)
	if err != nil {
		return value, err
	}
	var total int64
	for rows.Next() {
		node, err := scanManualSubscriptionNode(rows)
		if err != nil {
			_ = rows.Close()
			return value, err
		}
		total += int64(len(node.Outbound))
		if total > MaximumSubscriptionInputBytes || len(value.ManualNodes) >= subscription.MaximumNodes {
			_ = rows.Close()
			return value, ErrSubscriptionLimitExceeded
		}
		value.ManualNodes = append(value.ManualNodes, node)
	}
	if err := rows.Close(); err != nil {
		return value, err
	}
	if err := rows.Err(); err != nil {
		return value, err
	}
	rows, err = tx.QueryContext(ctx, `SELECT publication_id, hidden, revision FROM subscription_node_visibility`)
	if err != nil {
		return value, err
	}
	for rows.Next() {
		var id string
		var visibility SubscriptionNodeVisibility
		if err := rows.Scan(&id, &visibility.Hidden, &visibility.Revision); err != nil {
			_ = rows.Close()
			return value, err
		}
		value.Visibility[id] = visibility
	}
	if err := rows.Close(); err != nil {
		return value, err
	}
	if err := rows.Err(); err != nil {
		return value, err
	}
	return value, nil
}
