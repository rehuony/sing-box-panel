// SPDX-License-Identifier: GPL-3.0-or-later

package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"path/filepath"
	"slices"
	"strings"

	"github.com/rehuony/sing-box-panel/internal/database"
)

// MergeEffects are the records a restore of some categories changes in the
// categories it keeps, so that every reference still holds. They follow the
// schema's own rules: what refers to a removed record goes with it, and an
// exit that named a removed pool node falls back.
type MergeEffects struct {
	// PoolNodes are pool nodes published from fleet nodes that are gone.
	PoolNodes int `json:"pool_nodes"`
	// ExitRules are exit rules whose rule set is gone.
	ExitRules int `json:"exit_rules"`
	// RejectedExits are exit rules whose exit node is gone; they reject.
	RejectedExits int `json:"rejected_exits"`
	// DirectExits are channels whose final exit node is gone; they go direct.
	DirectExits int `json:"direct_exits"`
	// Grants are users' grants of pool nodes that are gone.
	Grants int `json:"grants"`
	// HistoryNodes are nodes that are gone whose records go with them.
	HistoryNodes int `json:"history_nodes"`
}

// stagedSchema is the name the staged panel database is attached under.
const stagedSchema = "staged"

// effectsQuery counts MergeEffects with each category read from the schema
// it would come from: nodes, subscriptions, rule sets, keys and history, in
// that order. Planning reads the current and staged databases side by side;
// merging reads the merged database alone.
func effectsQuery(nodes, subscriptions, ruleSets, keys, history string) string {
	var historyNodes []string
	for _, table := range historyTables {
		historyNodes = append(historyNodes, "SELECT node_id FROM "+history+"."+table)
	}
	return fmt.Sprintf(`WITH gone AS (
    SELECT id FROM %[2]s.subscription_nodes
     WHERE node_id IS NOT NULL AND node_id NOT IN (SELECT id FROM %[1]s.nodes)
)
SELECT
    (SELECT count(*) FROM gone),
    (SELECT count(*) FROM %[2]s.subscription_channel_routes
      WHERE rule_set_id IS NOT NULL AND rule_set_id NOT IN (SELECT id FROM %[3]s.subscription_rule_sets)),
    (SELECT count(*) FROM %[2]s.subscription_channel_routes
      WHERE exit_node_id IN (SELECT id FROM gone)
        AND (rule_set_id IS NULL OR rule_set_id IN (SELECT id FROM %[3]s.subscription_rule_sets))),
    (SELECT count(*) FROM %[2]s.subscription_channels WHERE default_exit_node_id IN (SELECT id FROM gone)),
    (SELECT count(*) FROM %[4]s.subscription_user_grants
      WHERE node_id IN (SELECT id FROM gone) OR node_id NOT IN (SELECT id FROM %[2]s.subscription_nodes)),
    (SELECT count(*) FROM (%[6]s) WHERE node_id NOT IN (SELECT id FROM %[1]s.nodes))`,
		nodes, subscriptions, ruleSets, keys, history, strings.Join(historyNodes, " UNION "))
}

func countEffects(ctx context.Context, db queryRower, query string) (MergeEffects, error) {
	var effects MergeEffects
	err := db.QueryRowContext(ctx, query).Scan(&effects.PoolNodes, &effects.ExitRules, &effects.RejectedExits,
		&effects.DirectExits, &effects.Grants, &effects.HistoryNodes)
	return effects, err
}

// sourceOf names the schema each category would come from when categories
// are restored.
func sourceOf(categories []Category) func(Category) string {
	return func(category Category) string {
		if slices.Contains(categories, category) {
			return stagedSchema
		}
		return "main"
	}
}

// PlanMerge counts what restoring categories from the snapshot in stageDir
// would change in the panel database of dataDir, without changing either.
// It reads the current database through a read-only connection while the
// panel runs.
func PlanMerge(ctx context.Context, dataDir, stageDir string, categories []Category) (MergeEffects, error) {
	if len(categories) == 0 {
		return MergeEffects{}, nil
	}
	db, err := database.OpenFile(filepath.Join(dataDir, PanelFile), true)
	if err != nil {
		return MergeEffects{}, err
	}
	defer db.Close()
	if _, err := db.ExecContext(ctx, `ATTACH DATABASE ? AS `+stagedSchema, filepath.Join(stageDir, PanelFile)); err != nil {
		return MergeEffects{}, fmt.Errorf("attach the staged database: %w", err)
	}
	from := sourceOf(categories)
	return countEffects(ctx, db, effectsQuery(from(CategoryNodes), from(CategorySubscriptions), from(CategoryRuleSets), from(CategoryKeys), from(CategoryHistory)))
}

// ErrMergeInconsistent is a merge that would leave a reference without its
// record; nothing is changed.
var ErrMergeInconsistent = errors.New("the restored records do not fit this panel's database")

// MergeSnapshot replaces the categories of the panel database in dataDir
// with those of the snapshot in stageDir, which must have this binary's
// schema, then removes or releases what refers to records that are gone, in
// one transaction. Each table of a category is replaced whole: history from
// a snapshot without raw traffic samples leaves none, because each node's
// next delta starts from the restored checkpoint, and samples the panel took
// after it would count that traffic twice. endSessions signs everyone out,
// as restoring the settings with their credentials requires. The panel must
// be stopped; the caller holds its lease. It returns what the repair changed.
func MergeSnapshot(ctx context.Context, dataDir, stageDir string, categories []Category, endSessions bool) (effects MergeEffects, err error) {
	schema, err := Schema()
	if err != nil {
		return MergeEffects{}, err
	}
	db, err := database.Open(ctx, filepath.Join(dataDir, PanelFile), schema, false)
	if err != nil {
		return MergeEffects{}, err
	}
	defer func() { err = errors.Join(err, db.Close()) }()
	conn, err := db.Conn(ctx)
	if err != nil {
		return MergeEffects{}, err
	}
	defer func() {
		// The connection goes back to a pool that expects its pragmas.
		_, restoreErr := conn.ExecContext(context.WithoutCancel(ctx), `PRAGMA foreign_keys = ON`)
		err = errors.Join(err, restoreErr, conn.Close())
	}()
	// Tables are replaced whole, and each repair below stands in for a cascade.
	if _, err := conn.ExecContext(ctx, `PRAGMA foreign_keys = OFF`); err != nil {
		return MergeEffects{}, err
	}
	if len(categories) > 0 {
		if _, err := conn.ExecContext(ctx, `ATTACH DATABASE ? AS `+stagedSchema, filepath.Join(stageDir, PanelFile)); err != nil {
			return MergeEffects{}, fmt.Errorf("attach the staged database: %w", err)
		}
		defer func() {
			_, detachErr := conn.ExecContext(context.WithoutCancel(ctx), `DETACH DATABASE `+stagedSchema)
			err = errors.Join(err, detachErr)
		}()
		var version int
		if err := conn.QueryRowContext(ctx, `PRAGMA `+stagedSchema+`.user_version`).Scan(&version); err != nil {
			return MergeEffects{}, err
		}
		if version != schema.Version() {
			return MergeEffects{}, fmt.Errorf("%w: the staged database has schema %d, this panel %d", ErrMergeInconsistent, version, schema.Version())
		}
	}
	tx, err := conn.BeginTx(ctx, nil)
	if err != nil {
		return MergeEffects{}, err
	}
	defer func() { _ = tx.Rollback() }()
	for _, category := range categories {
		for _, table := range categoryTables[category] {
			if err := replaceTable(ctx, tx, table); err != nil {
				return MergeEffects{}, fmt.Errorf("restore %s: %w", table, err)
			}
		}
	}
	if effects, err = countEffects(ctx, tx, effectsQuery("main", "main", "main", "main", "main")); err != nil {
		return MergeEffects{}, err
	}
	repairs := []string{
		`CREATE TEMP TABLE restore_gone_pool AS
             SELECT id FROM subscription_nodes WHERE node_id IS NOT NULL AND node_id NOT IN (SELECT id FROM nodes)`,
		`DELETE FROM subscription_channel_members WHERE node_id IN (SELECT id FROM temp.restore_gone_pool)`,
		`DELETE FROM subscription_user_grants
          WHERE node_id IN (SELECT id FROM temp.restore_gone_pool) OR node_id NOT IN (SELECT id FROM subscription_nodes)`,
		// Its trigger releases the exits that named each removed pool node.
		`DELETE FROM subscription_nodes WHERE id IN (SELECT id FROM temp.restore_gone_pool)`,
		`DROP TABLE temp.restore_gone_pool`,
		`DELETE FROM subscription_channel_routes
          WHERE rule_set_id IS NOT NULL AND rule_set_id NOT IN (SELECT id FROM subscription_rule_sets)`,
	}
	for _, table := range historyTables {
		repairs = append(repairs, `DELETE FROM `+table+` WHERE node_id NOT IN (SELECT id FROM nodes)`)
	}
	if endSessions {
		repairs = append(repairs, `DELETE FROM auth_sessions`)
	}
	for _, statement := range repairs {
		if _, err := tx.ExecContext(ctx, statement); err != nil {
			return MergeEffects{}, err
		}
	}
	rows, err := tx.QueryContext(ctx, `PRAGMA main.foreign_key_check`)
	if err != nil {
		return MergeEffects{}, err
	}
	broken := rows.Next()
	if err := errors.Join(rows.Err(), rows.Close()); err != nil {
		return MergeEffects{}, err
	}
	if broken {
		return MergeEffects{}, ErrMergeInconsistent
	}
	return effects, tx.Commit()
}

// replaceTable replaces the rows of a table with the staged database's,
// column by column.
func replaceTable(ctx context.Context, tx *sql.Tx, table string) error {
	rows, err := tx.QueryContext(ctx, `SELECT name FROM pragma_table_info(?)`, table)
	if err != nil {
		return err
	}
	var columns []string
	for rows.Next() {
		var column string
		if err := rows.Scan(&column); err != nil {
			return errors.Join(err, rows.Close())
		}
		columns = append(columns, `"`+column+`"`)
	}
	if err := errors.Join(rows.Err(), rows.Close()); err != nil {
		return err
	}
	if len(columns) == 0 {
		return fmt.Errorf("table %s does not exist", table)
	}
	list := strings.Join(columns, ", ")
	if _, err := tx.ExecContext(ctx, `DELETE FROM main."`+table+`"`); err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO main."`+table+`" (`+list+`) SELECT `+list+` FROM `+stagedSchema+`."`+table+`"`)
	return err
}
