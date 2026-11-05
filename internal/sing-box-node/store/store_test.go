package store

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/rehuony/sing-box-panel/internal/database"
)

func TestOpenConfiguresEveryConnectionAndReopens(t *testing.T) {
	ctx := testContext(t)
	path := filepath.Join(t.TempDir(), "panel.db")

	store, err := Open(ctx, path)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}

	info, err := store.SchemaInfo(ctx)
	if err != nil {
		t.Fatalf("SchemaInfo() error = %v", err)
	}
	if info.ApplicationID != ApplicationID || info.Version != CurrentSchemaVersion {
		t.Fatalf("SchemaInfo() = %+v, want application=%#x version=%d", info, ApplicationID, CurrentSchemaVersion)
	}

	connections := make([]*sql.Conn, 0, 2)
	for range 2 {
		conn, err := store.db.Conn(ctx)
		if err != nil {
			t.Fatalf("db.Conn() error = %v", err)
		}
		connections = append(connections, conn)
		defer conn.Close()
	}
	for i, conn := range connections {
		assertPragmaInt(t, ctx, conn, i, "foreign_keys", 1)
		assertPragmaInt(t, ctx, conn, i, "busy_timeout", database.BusyTimeoutMillis)
		assertPragmaInt(t, ctx, conn, i, "synchronous", 2)
		assertPragmaInt(t, ctx, conn, i, "trusted_schema", 0)

		var journalMode string
		if err := conn.QueryRowContext(ctx, `PRAGMA journal_mode`).Scan(&journalMode); err != nil {
			t.Fatalf("connection %d PRAGMA journal_mode error = %v", i, err)
		}
		if journalMode != "wal" {
			t.Fatalf("connection %d journal_mode = %q, want %q", i, journalMode, "wal")
		}
	}

	fileInfo, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat database: %v", err)
	}
	if got := fileInfo.Mode().Perm(); got != 0o600 {
		t.Fatalf("database permissions = %#o, want %#o", got, os.FileMode(0o600))
	}

	for _, conn := range connections {
		if err := conn.Close(); err != nil {
			t.Fatalf("close reserved connection: %v", err)
		}
	}
	if err := store.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	reopened, err := Open(ctx, path)
	if err != nil {
		t.Fatalf("Open() after close error = %v", err)
	}
	t.Cleanup(func() { _ = reopened.Close() })

	bootstrap, err := reopened.Bootstrap(ctx)
	if err != nil {
		t.Fatalf("Bootstrap() error = %v", err)
	}
	if bootstrap.Head != nil || bootstrap.Hub.HeadRevisionID != "" {
		t.Fatalf("Bootstrap() head = %+v / %q, want no head", bootstrap.Head, bootstrap.Hub.HeadRevisionID)
	}
}

func TestOpenRejectsPreviousDatabaseIdentity(t *testing.T) {
	for _, applicationID := range []string{"0x53425032", "0x53425033"} {
		t.Run(applicationID, func(t *testing.T) {
			t.Parallel()
			ctx := testContext(t)
			path := filepath.Join(t.TempDir(), "panel.db")
			legacy, err := sql.Open("sqlite", path)
			if err != nil {
				t.Fatalf("open legacy fixture: %v", err)
			}
			if _, err := legacy.ExecContext(ctx, `PRAGMA application_id = `+applicationID); err != nil {
				_ = legacy.Close()
				t.Fatalf("set legacy application id: %v", err)
			}
			if err := legacy.Close(); err != nil {
				t.Fatalf("close legacy fixture: %v", err)
			}

			opened, err := Open(ctx, path)
			if opened != nil {
				_ = opened.Close()
				t.Fatal("Open returned a store for a previous database identity")
			}
			if !errors.Is(err, database.ErrUnexpectedApplicationID) {
				t.Fatalf("Open error = %v, want database.ErrUnexpectedApplicationID", err)
			}
		})
	}
}

func TestSchemaConstraints(t *testing.T) {
	ctx := testContext(t)
	store := openTestStore(t, ctx)
	now := time.Now().UTC().Format(time.RFC3339Nano)

	for _, test := range []struct {
		name     string
		version  int
		document string
	}{
		{name: "invalid JSON", version: 1, document: `{`},
		{name: "wrong document schema", version: 2, document: `{}`},
		{name: "non-object document", version: 1, document: `[]`},
		{name: "null document", version: 1, document: `null`},
	} {
		_, err := store.db.ExecContext(
			ctx,
			`INSERT INTO canonical_revisions(
            id, sequence, schema_version, document_json, sha256, command_id, created_at
		 ) VALUES (?, 1, ?, ?, ?, ?, ?)`,
			test.name, test.version, test.document, stringsOf('0', 64), test.name+"-command", now,
		)
		if err == nil {
			t.Errorf("%s canonical insert succeeded", test.name)
		}
	}

	if _, err := store.db.ExecContext(
		ctx,
		`UPDATE hub_state SET head_revision_id = 'missing-revision' WHERE singleton = 1`,
	); err == nil {
		t.Fatal("foreign-key violating hub update succeeded")
	}

	if _, err := store.db.ExecContext(
		ctx,
		`UPDATE hub_state SET target_generation = 'not-an-integer' WHERE singleton = 1`,
	); err == nil {
		t.Fatal("STRICT type violating hub update succeeded")
	}
}

func TestWithTxRollsBackCallbackFailure(t *testing.T) {
	ctx := testContext(t)
	store := openTestStore(t, ctx)
	wantErr := errors.New("stop transaction")
	now := time.Now().UTC().Format(time.RFC3339Nano)

	err := store.WithTx(ctx, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(
			ctx,
			`INSERT INTO command_results(request_id, result, completed_at) VALUES ('request-1', x'00', ?)`,
			now,
		); err != nil {
			return err
		}
		return wantErr
	})
	if !errors.Is(err, wantErr) {
		t.Fatalf("WithTx() error = %v, want %v", err, wantErr)
	}

	var count int
	if err := store.db.QueryRowContext(
		ctx,
		`SELECT count(*) FROM command_results WHERE request_id = 'request-1'`,
	).Scan(&count); err != nil {
		t.Fatalf("count rolled-back command result: %v", err)
	}
	if count != 0 {
		t.Fatalf("rolled-back command result count = %d, want 0", count)
	}
}

func openTestStore(t *testing.T, ctx context.Context) *Store {
	t.Helper()
	store, err := Open(ctx, filepath.Join(t.TempDir(), "panel.db"))
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	return store
}

func testContext(t *testing.T) context.Context {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	t.Cleanup(cancel)
	return ctx
}

type pragmaQuerier interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

func assertPragmaInt(
	t *testing.T,
	ctx context.Context,
	query pragmaQuerier,
	connection int,
	name string,
	want int,
) {
	t.Helper()
	var got int
	if err := query.QueryRowContext(ctx, "PRAGMA "+name).Scan(&got); err != nil {
		t.Fatalf("connection %d PRAGMA %s error = %v", connection, name, err)
	}
	if got != want {
		t.Fatalf("connection %d PRAGMA %s = %d, want %d", connection, name, got, want)
	}
}

func stringsOf(value byte, count int) string {
	buffer := make([]byte, count)
	for i := range buffer {
		buffer[i] = value
	}
	return string(buffer)
}
