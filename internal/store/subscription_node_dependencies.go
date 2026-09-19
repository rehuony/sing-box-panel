// SPDX-License-Identifier: GPL-3.0-or-later

package store

import (
	"context"
	"database/sql"
	"encoding/json"

	"github.com/rehuony/sing-box-panel/internal/subscription"
)

// Validate under the write transaction so concurrent node edits cannot create
// a dependency cycle. Manual detours are scoped to this collection, matching
// the renderer's source-scoped tag resolution.
func validateManualDependencies(ctx context.Context, tx *sql.Tx, candidate ManualSubscriptionNode) error {
	type reference struct {
		Tag    string `json:"tag"`
		Detour string `json:"detour"`
	}
	var next reference
	if err := json.Unmarshal(candidate.Outbound, &next); err != nil {
		return ErrSubscriptionNodeInvalid
	}
	rows, err := tx.QueryContext(ctx, `SELECT outbound_json FROM subscription_manual_nodes WHERE id <> ?`, candidate.ID)
	if err != nil {
		return err
	}
	defer rows.Close()
	edges := map[string]string{next.Tag: next.Detour}
	for rows.Next() {
		var raw string
		if err := rows.Scan(&raw); err != nil {
			return err
		}
		var node reference
		if err := json.Unmarshal([]byte(raw), &node); err != nil {
			return err
		}
		if node.Tag == next.Tag {
			return &subscription.NodeFieldError{Path: "tag", Code: "duplicate_name"}
		}
		edges[node.Tag] = node.Detour
	}
	if err := rows.Err(); err != nil {
		return err
	}
	seen := map[string]bool{}
	for tag := next.Tag; tag != ""; {
		if seen[tag] {
			return &subscription.NodeFieldError{Path: "detour", Code: "dependency_cycle"}
		}
		seen[tag] = true
		detour, found := edges[tag]
		if !found {
			return &subscription.NodeFieldError{Path: "detour", Code: "node_not_found"}
		}
		tag = detour
	}
	return nil
}
