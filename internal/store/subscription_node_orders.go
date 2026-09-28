// SPDX-License-Identifier: GPL-3.0-or-later

package store

import (
	"context"
	"database/sql"
	_ "embed"
	"encoding/json"
	"errors"
	"slices"

	"github.com/rehuony/sing-box-panel/internal/subscription"
)

//go:embed subscription_node_orders.sql
var subscriptionNodeOrdersSchema string

func (s *Store) SetSubscriptionNodeOrder(ctx context.Context, collectionID string, ids []string, expected int64) (subscription.NodeOrder, error) {
	if (collectionID != "manual" && validateSubscriptionID(collectionID, "source") != nil) || ids == nil || len(ids) > subscription.MaximumNodes || expected < 0 || expected >= 9007199254740991 {
		return subscription.NodeOrder{}, ErrSubscriptionNodeInvalid
	}
	seen := make(map[string]bool, len(ids))
	for _, id := range ids {
		if !subscription.ValidKey(id) || seen[id] {
			return subscription.NodeOrder{}, ErrSubscriptionNodeInvalid
		}
		seen[id] = true
	}
	result := subscription.NodeOrder{IDs: slices.Clone(ids), Revision: expected + 1}
	err := s.WithTx(ctx, func(tx *sql.Tx) error {
		if collectionID != "manual" {
			source, err := getSubscriptionSource(ctx, tx, collectionID)
			if err != nil {
				return err
			}
			if source.SourceKind != SubscriptionSourceRemote {
				return ErrSubscriptionNodeInvalid
			}
		}
		var revision int64
		err := tx.QueryRowContext(ctx, `SELECT revision FROM subscription_node_orders WHERE collection_id=?`, collectionID).Scan(&revision)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		if revision != expected {
			return ErrSubscriptionConflict
		}
		raw, err := json.Marshal(ids)
		if err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx, `INSERT INTO subscription_node_orders(collection_id, node_ids_json, revision) VALUES(?,?,?)
			ON CONFLICT(collection_id) DO UPDATE SET node_ids_json=excluded.node_ids_json, revision=excluded.revision`, collectionID, string(raw), result.Revision)
		return err
	})
	return result, err
}

func loadSubscriptionNodeOrders(ctx context.Context, tx *sql.Tx) (map[string]subscription.NodeOrder, error) {
	rows, err := tx.QueryContext(ctx, `SELECT collection_id, node_ids_json, revision FROM subscription_node_orders`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	orders := make(map[string]subscription.NodeOrder)
	for rows.Next() {
		var id, raw string
		var order subscription.NodeOrder
		if err := rows.Scan(&id, &raw, &order.Revision); err != nil {
			return nil, err
		}
		if err := json.Unmarshal([]byte(raw), &order.IDs); err != nil {
			return nil, err
		}
		orders[id] = order
	}
	return orders, rows.Err()
}
