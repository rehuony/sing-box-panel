// SPDX-License-Identifier: GPL-3.0-or-later

package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
)

// migrateSubscriptionPolicies removes retired overrides without decoding native
// template numbers, and assigns priorities in the previous flattened rule order.
func migrateSubscriptionPolicies(ctx context.Context, tx *sql.Tx) error {
	rows, err := tx.QueryContext(ctx, `SELECT id, config_json FROM subscription_channels WHERE json_type(config_json, '$.policy') = 'object'`)
	if err != nil {
		return err
	}
	type channel struct{ id, content string }
	var channels []channel
	for rows.Next() {
		var item channel
		if err := rows.Scan(&item.id, &item.content); err != nil {
			rows.Close()
			return err
		}
		channels = append(channels, item)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	for _, item := range channels {
		var root, policy map[string]json.RawMessage
		if err := json.Unmarshal([]byte(item.content), &root); err != nil {
			return err
		}
		// Explicit policy membership replaces retired channel-level filters.
		delete(root, "exclude_tags")
		delete(root, "exclude_types")
		if err := json.Unmarshal(root["policy"], &policy); err != nil {
			return err
		}
		var organizer map[string]json.RawMessage
		if value := policy["organizer"]; len(value) > 0 {
			if err := json.Unmarshal(value, &organizer); err != nil {
				return err
			}
			if value := organizer["incompatible"]; len(value) > 0 {
				policy["incompatible_nodes"] = value
			}
		}
		delete(policy, "organizer")
		var groups []map[string]json.RawMessage
		if err := json.Unmarshal(policy["groups"], &groups); err != nil {
			return err
		}
		position := 0
		for _, group := range groups {
			delete(group, "default_exit")
			var rules []map[string]json.RawMessage
			if value := group["rules"]; len(value) > 0 {
				if err := json.Unmarshal(value, &rules); err != nil {
					return err
				}
			}
			for _, rule := range rules {
				position += 10
				if _, present := rule["sort_index"]; !present {
					rule["sort_index"] = json.RawMessage(fmt.Sprint(position))
				}
			}
			group["rules"], err = json.Marshal(rules)
			if err != nil {
				return err
			}
		}
		policy["groups"], err = json.Marshal(groups)
		if err != nil {
			return err
		}
		root["policy"], err = json.Marshal(policy)
		if err != nil {
			return err
		}
		content, err := json.Marshal(root)
		if err != nil {
			return err
		}
		if _, err := DecodeSubscriptionChannelConfig(content); err != nil {
			return fmt.Errorf("validate migrated channel %q: %w", item.id, err)
		}
		if _, err := tx.ExecContext(ctx, `UPDATE subscription_channels SET config_json = ? WHERE id = ?`, string(content), item.id); err != nil {
			return err
		}
	}
	return nil
}
