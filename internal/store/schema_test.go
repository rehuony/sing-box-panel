package store

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"reflect"
	"testing"
)

func TestOpenRejectsUnsupportedFormatsWithoutConvertingData(t *testing.T) {
	cases := []struct {
		name        string
		id, version int
		want        error
	}{
		{"unidentified", 0, 0, ErrUnexpectedApplicationID},
		{"foreign", 123, CurrentSchemaVersion, ErrUnexpectedApplicationID},
		{"future", ApplicationID, CurrentSchemaVersion + 1, ErrSchemaTooNew},
	}
	for version := 0; version < CurrentSchemaVersion; version++ {
		cases = append(cases, struct {
			name        string
			id, version int
			want        error
		}{fmt.Sprintf("old-%d", version), ApplicationID, version, ErrSchemaUnsupported})
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ctx := t.Context()
			path := filepath.Join(t.TempDir(), "panel.db")
			db, err := sql.Open("sqlite", path)
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			_, err = db.ExecContext(ctx, fmt.Sprintf("PRAGMA application_id=%d; PRAGMA user_version=%d; CREATE TABLE preserved(value TEXT); INSERT INTO preserved VALUES('untouched');", tc.id, tc.version))
			if err != nil {
				t.Fatal(err)
			}
			opened, err := Open(ctx, path)
			if opened != nil {
				opened.Close()
				t.Fatal("unsupported database opened")
			}
			if !errors.Is(err, tc.want) {
				t.Fatalf("Open: %v, want %v", err, tc.want)
			}
			var content string
			if err := db.QueryRowContext(ctx, "SELECT value FROM preserved").Scan(&content); err != nil || content != "untouched" {
				t.Fatalf("data changed: %q %v", content, err)
			}
			var count int
			if err := db.QueryRowContext(ctx, "SELECT count(*) FROM sqlite_schema WHERE name NOT LIKE 'sqlite_%'").Scan(&count); err != nil || count != 1 {
				t.Fatalf("schema changed: %d %v", count, err)
			}
			assertPragmaInt(t, ctx, db, 0, "user_version", tc.version)
			assertPragmaInt(t, ctx, db, 0, "application_id", tc.id)
		})
	}
}

func TestNodeOrderPersistence(t *testing.T) {
	ctx := t.Context()
	db := openTestStore(t, ctx)
	source, err := db.CreateSubscriptionSource(ctx, SubscriptionSource{ID: "source-order", Name: "Order", SourceKind: SubscriptionSourceRemote, Config: json.RawMessage(`{}`), Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	state, err := db.LoadSubscriptionNodeCatalogState(ctx)
	if err != nil || len(state.NodeOrders) != 0 {
		t.Fatalf("initial orders: %+v, %v", state.NodeOrders, err)
	}
	order, err := db.SetSubscriptionNodeOrder(ctx, source.ID, []string{"node_b", "node_a"}, 0)
	if err != nil || order.Revision != 1 {
		t.Fatalf("save order: %+v, %v", order, err)
	}
	if _, err := db.SetSubscriptionNodeOrder(ctx, source.ID, []string{"node_a"}, 0); !errors.Is(err, ErrSubscriptionConflict) {
		t.Fatalf("stale save: %v", err)
	}
	path := db.Path()
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	state, err = reopened.LoadSubscriptionNodeCatalogState(ctx)
	if err != nil || !reflect.DeepEqual(state.NodeOrders[source.ID], order) {
		t.Fatalf("reopened order: %+v, %v", state.NodeOrders, err)
	}
	unchanged, err := reopened.GetSubscriptionSource(ctx, source.ID)
	if err != nil || !reflect.DeepEqual(unchanged, source) {
		t.Fatalf("source changed: %+v, %v", unchanged, err)
	}
	if err := reopened.DeleteSubscriptionSource(ctx, source.ID, source.UpdatedAt); err != nil {
		t.Fatal(err)
	}
	state, err = reopened.LoadSubscriptionNodeCatalogState(ctx)
	if err != nil || len(state.NodeOrders) != 0 {
		t.Fatalf("deleted source retained order: %+v, %v", state.NodeOrders, err)
	}
}

func TestFreshSchemaContainsOnlyCurrentStorage(t *testing.T) {
	db := openTestStore(t, t.Context())
	var obsolete int
	if err := db.db.QueryRowContext(t.Context(), `SELECT count(*) FROM sqlite_schema WHERE name IN ('schema_migrations', 'panel_settings', 'tasks')`).Scan(&obsolete); err != nil || obsolete != 0 {
		t.Fatalf("obsolete storage objects: %d %v", obsolete, err)
	}
	var violations int
	rows, err := db.db.QueryContext(t.Context(), "PRAGMA foreign_key_check")
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	for rows.Next() {
		violations++
	}
	if err := rows.Err(); err != nil || violations != 0 {
		t.Fatalf("foreign key violations: %d %v", violations, err)
	}
}
