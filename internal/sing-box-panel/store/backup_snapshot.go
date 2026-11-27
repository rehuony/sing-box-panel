// SPDX-License-Identifier: GPL-3.0-or-later

package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"

	"github.com/rehuony/sing-box-panel/internal/database"
)

// PanelFile is the panel database's file name in its data directory.
const PanelFile = "panel.db"

// Category is a part of the panel database that a backup can hold and a
// restore can replace on its own.
type Category string

const (
	// CategoryNodes is the fleet: the panel identity nodes trust, the nodes,
	// their enrollments and their configurations.
	CategoryNodes Category = "nodes"
	// CategorySubscriptions is the subscription sources, the node pool and
	// the channels with their groups and exit rules.
	CategorySubscriptions Category = "subscriptions"
	// CategoryRuleSets is the channels' shared rule sets.
	CategoryRuleSets Category = "rule_sets"
	// CategoryKeys is the subscription keys and their users and grants.
	CategoryKeys Category = "keys"
	// CategoryHistory is the nodes' traffic and runtime records.
	CategoryHistory Category = "history"
)

// Categories are the database categories in the order they are shown.
var Categories = []Category{CategoryNodes, CategorySubscriptions, CategoryRuleSets, CategoryKeys, CategoryHistory}

// categoryTables are the tables of each category. Every table of the panel
// database belongs to one category or to installationTables.
var categoryTables = map[Category][]string{
	CategoryNodes:         {"fleet_identity", "nodes", "node_enrollments", "canonical_revisions", "node_configurations"},
	CategorySubscriptions: {"subscription_sources", "subscription_nodes", "subscription_channels", "subscription_channel_groups", "subscription_channel_members", "subscription_channel_routes"},
	CategoryRuleSets:      {"subscription_rule_sets"},
	CategoryKeys:          {"subscription_users", "subscription_user_grants", "subscription_tokens", "subscription_token_secrets"},
	CategoryHistory:       {"runtime_transitions", "traffic_samples", "traffic_checkpoint", "traffic_days", "traffic_periods"},
}

// installationTables belong to one installation and are never backed up or
// restored: sign-ins, interrupted moves and saves, and the release catalog and
// download cache, whose files stay on this host.
var installationTables = []string{"auth_sessions", "data_directory_moves", "panel_settings_file_commits", "catalog_state", "cached_artifacts"}

// historyTables are the tables whose rows each belong to one node.
var historyTables = categoryTables[CategoryHistory]

// SnapshotInfo is what a snapshot of the panel's databases holds.
type SnapshotInfo struct {
	// PanelID is the identity nodes bind their sessions to; empty before any
	// node was enrolled.
	PanelID string `json:"panel_id"`
	// SchemaVersion and LogsSchemaVersion are the databases' migration
	// levels, each 0 without its database.
	SchemaVersion     int            `json:"schema_version,omitempty"`
	LogsSchemaVersion int            `json:"logs_schema_version,omitempty"`
	Counts            SnapshotCounts `json:"counts"`
}

// SnapshotCounts are the records a snapshot holds that an operator
// recognizes, 0 for a category it does not hold.
type SnapshotCounts struct {
	Nodes    int `json:"nodes"`
	Sources  int `json:"sources"`
	Channels int `json:"channels"`
	RuleSets int `json:"rule_sets"`
	Keys     int `json:"keys"`
	// HistoryDays is the days with a node's traffic recorded.
	HistoryDays int `json:"history_days"`
	LogEntries  int `json:"log_entries"`
}

// Snapshot writes into dir the panel database of dataDir with only the
// categories given, when any is, and with logs the log database, each under
// its own name. It never holds installation tables, and it is rebuilt after
// leaving parts out so that their records do not linger in free pages. While
// the panel runs it reads through read-only connections; while it is stopped
// the caller holds its lease.
func Snapshot(ctx context.Context, dataDir, dir string, categories []Category, includeLogs bool) (SnapshotInfo, error) {
	panelID := ""
	if len(categories) > 0 {
		panel := filepath.Join(dir, PanelFile)
		if err := database.Snapshot(ctx, filepath.Join(dataDir, PanelFile), panel); err != nil {
			return SnapshotInfo{}, fmt.Errorf("snapshot panel database: %w", err)
		}
		left := slices.Clone(installationTables)
		for _, category := range Categories {
			if !slices.Contains(categories, category) {
				left = append(left, categoryTables[category]...)
			}
		}
		var err error
		if panelID, err = leaveOut(ctx, panel, left); err != nil {
			return SnapshotInfo{}, fmt.Errorf("leave unselected records out of the snapshot: %w", err)
		}
	}
	if includeLogs {
		if err := database.Snapshot(ctx, filepath.Join(dataDir, LogsFile), filepath.Join(dir, LogsFile)); err != nil {
			return SnapshotInfo{}, fmt.Errorf("snapshot log database: %w", err)
		}
	}
	info, err := InspectSnapshot(ctx, dir)
	if info.PanelID == "" {
		info.PanelID = panelID
	}
	return info, err
}

// leaveOut empties tables in the snapshot at path and rebuilds it, returning
// the panel identity it held first.
func leaveOut(ctx context.Context, path string, tables []string) (panelID string, err error) {
	db, err := database.OpenFile(path, false)
	if err != nil {
		return "", err
	}
	defer func() { err = errors.Join(err, db.Close()) }()
	if err := db.QueryRowContext(ctx, `SELECT coalesce((SELECT panel_id FROM fleet_identity), '')`).Scan(&panelID); err != nil {
		return "", err
	}
	// Each table is emptied as it is; a restore repairs references across categories.
	if _, err := db.ExecContext(ctx, `PRAGMA foreign_keys = OFF`); err != nil {
		return "", err
	}
	for _, table := range tables {
		if _, err := db.ExecContext(ctx, `DELETE FROM "`+table+`"`); err != nil {
			return "", err
		}
	}
	_, err = db.ExecContext(ctx, `VACUUM`)
	return panelID, err
}

// InspectSnapshot verifies the databases in dir, as Snapshot writes them or a
// restore stages them: each is intact, of this panel and no newer than this
// binary, and either may be absent. It reads what they hold.
func InspectSnapshot(ctx context.Context, dir string) (SnapshotInfo, error) {
	var info SnapshotInfo
	err := inspect(ctx, filepath.Join(dir, PanelFile), Schema, func(db *sql.DB, version int) error {
		info.SchemaVersion = version
		if err := db.QueryRowContext(ctx, `SELECT coalesce((SELECT panel_id FROM fleet_identity), '')`).Scan(&info.PanelID); err != nil {
			return fmt.Errorf("read panel identity: %w", err)
		}
		for _, count := range []struct {
			target *int
			query  string
		}{
			{&info.Counts.Nodes, `SELECT count(*) FROM nodes`},
			{&info.Counts.Sources, `SELECT count(*) FROM subscription_sources`},
			{&info.Counts.Channels, `SELECT count(*) FROM subscription_channels`},
			{&info.Counts.RuleSets, `SELECT count(*) FROM subscription_rule_sets`},
			{&info.Counts.Keys, `SELECT count(*) FROM subscription_tokens`},
			{&info.Counts.HistoryDays, `SELECT count(DISTINCT day_start) FROM traffic_days`},
		} {
			if err := db.QueryRowContext(ctx, count.query).Scan(count.target); err != nil {
				return fmt.Errorf("count records: %w", err)
			}
		}
		return nil
	})
	if err != nil {
		return SnapshotInfo{}, fmt.Errorf("panel database: %w", err)
	}
	err = inspect(ctx, filepath.Join(dir, LogsFile), LogSchema, func(db *sql.DB, version int) error {
		info.LogsSchemaVersion = version
		return db.QueryRowContext(ctx, `SELECT count(*) FROM log_entries`).Scan(&info.Counts.LogEntries)
	})
	if err != nil {
		return SnapshotInfo{}, fmt.Errorf("log database: %w", err)
	}
	return info, nil
}

// inspect verifies the database at path against its schema and reads it,
// when the file exists.
func inspect(ctx context.Context, path string, schemaOf func() (database.Schema, error), read func(*sql.DB, int) error) error {
	if _, err := os.Lstat(path); errors.Is(err, os.ErrNotExist) {
		return nil
	} else if err != nil {
		return err
	}
	schema, err := schemaOf()
	if err != nil {
		return err
	}
	db, err := database.OpenFile(path, true)
	if err != nil {
		return err
	}
	defer db.Close()
	version, err := schema.Verify(ctx, db)
	if err != nil {
		return err
	}
	return read(db, version)
}

// UpgradeSnapshot brings the panel database in dir to this binary's schema,
// so that a restore copies its tables column for column.
func UpgradeSnapshot(ctx context.Context, dir string) error {
	path := filepath.Join(dir, PanelFile)
	if _, err := os.Lstat(path); errors.Is(err, os.ErrNotExist) {
		return nil
	}
	schema, err := Schema()
	if err != nil {
		return err
	}
	db, err := database.Open(ctx, path, schema, false)
	if err != nil {
		return err
	}
	_, err = db.ExecContext(ctx, `PRAGMA journal_mode = DELETE`)
	return errors.Join(err, db.Close())
}
