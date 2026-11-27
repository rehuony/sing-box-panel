// SPDX-License-Identifier: GPL-3.0-or-later

package store

import (
	"slices"
	"testing"
)

// TestEveryPanelTableHasOneBackupCategory keeps backups complete as the
// schema grows: each table of the panel database is in exactly one category
// or belongs to the installation.
func TestEveryPanelTableHasOneBackupCategory(t *testing.T) {
	ctx := testContext(t)
	database := openTestStore(t, ctx)
	rows, err := database.db.QueryContext(ctx, `SELECT name FROM sqlite_schema
        WHERE type = 'table' AND name NOT LIKE 'sqlite_%' AND name <> 'migration_history'`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	homes := make(map[string]int)
	for _, table := range installationTables {
		homes[table]++
	}
	for _, category := range Categories {
		for _, table := range categoryTables[category] {
			homes[table]++
		}
	}
	var tables []string
	for rows.Next() {
		var table string
		if err := rows.Scan(&table); err != nil {
			t.Fatal(err)
		}
		tables = append(tables, table)
		if homes[table] != 1 {
			t.Errorf("table %s is in %d backup categories, want 1", table, homes[table])
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	for table := range homes {
		if !slices.Contains(tables, table) {
			t.Errorf("backup category names %s, which the schema does not have", table)
		}
	}
}
