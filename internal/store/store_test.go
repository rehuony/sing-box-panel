package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/rehuony/sing-box-panel/internal/configuration"
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
		assertPragmaInt(t, ctx, conn, i, "busy_timeout", defaultBusyTimeoutMillis)
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
			if !errors.Is(err, ErrUnexpectedApplicationID) {
				t.Fatalf("Open error = %v, want ErrUnexpectedApplicationID", err)
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
			`INSERT INTO subscription_channels(
		        id, name, format, config_json, public_host, created_at, updated_at
		     ) VALUES ('channel-1', 'default', 'sing-box', '{}', 'example.test', ?, ?)`,
			now,
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
		`SELECT count(*) FROM subscription_channels WHERE id = 'channel-1'`,
	).Scan(&count); err != nil {
		t.Fatalf("count rolled-back channel: %v", err)
	}
	if count != 0 {
		t.Fatalf("rolled-back channel count = %d, want 0", count)
	}
}

func TestConfigurationSaveAndSnapshotAreAtomic(t *testing.T) {
	ctx := testContext(t)
	database := openTestStore(t, ctx)
	revision := NewCanonicalRevision{ID: "snapshot-1", SchemaVersion: configuration.SchemaVersion, CommandID: "save-1"}
	first, err := database.SaveConfigurationFile(ctx, 0, `{"value":9007199254740993}`, revision)
	if err != nil {
		t.Fatal(err)
	}
	assertHeadAndCounts(t, ctx, database, first.CanonicalRevisionID, 1)
	// Duplicate snapshot identity must roll back both the text and runtime head.
	if _, err := database.SaveConfigurationFile(ctx, first.Revision, `{"value":2}`, revision); err == nil {
		t.Fatal("duplicate identity accepted")
	}
	unchanged, err := database.ConfigurationFile(ctx)
	if err != nil || unchanged != first {
		t.Fatalf("partial save: %+v %v", unchanged, err)
	}
	revision.ID, revision.CommandID = "snapshot-2", "save-2"
	second, err := database.SaveConfigurationFile(ctx, first.Revision, `{"value":2}`, revision)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := database.SaveConfigurationFile(ctx, first.Revision, `{}`, revision); !errors.Is(err, ErrConfigurationFileConflict) {
		t.Fatalf("stale save: %v", err)
	}
	assertHeadAndCounts(t, ctx, database, second.CanonicalRevisionID, 2)
	path := database.Path()
	if err := database.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	persisted, err := reopened.ConfigurationFile(ctx)
	if err != nil || persisted != second {
		t.Fatalf("reopen: %+v %v", persisted, err)
	}
	assertHeadAndCounts(t, ctx, reopened, second.CanonicalRevisionID, 2)
}

func TestConfigurationCASAcrossStores(t *testing.T) {
	ctx := testContext(t)
	path := filepath.Join(t.TempDir(), "panel.db")
	first, err := Open(ctx, path)
	if err != nil {
		t.Fatalf("first Open() error = %v", err)
	}
	t.Cleanup(func() { _ = first.Close() })
	second, err := Open(ctx, path)
	if err != nil {
		t.Fatalf("second Open() error = %v", err)
	}
	t.Cleanup(func() { _ = second.Close() })

	start := make(chan struct{})
	errorsByWriter := make([]error, 2)
	var writers sync.WaitGroup
	for i, store := range []*Store{first, second} {
		writers.Add(1)
		go func(i int, store *Store) {
			defer writers.Done()
			<-start
			_, errorsByWriter[i] = saveTestConfiguration(ctx, store, 0,
				NewCanonicalRevision{
					ID:            fmt.Sprintf("revision-%d", i),
					SchemaVersion: configuration.SchemaVersion,
					Document:      json.RawMessage(fmt.Sprintf(`{"experimental":{"writer":%d}}`, i)),
					CommandID:     fmt.Sprintf("command-%d", i),
				},
			)
		}(i, store)
	}
	close(start)
	writers.Wait()

	successes := 0
	conflicts := 0
	for i, err := range errorsByWriter {
		switch {
		case err == nil:
			successes++
		case errors.Is(err, ErrConfigurationFileConflict):
			conflicts++
		default:
			t.Fatalf("writer %d error = %v, want success or conflict", i, err)
		}
	}
	if successes != 1 || conflicts != 1 {
		t.Fatalf("writers: successes=%d conflicts=%d, want 1 and 1", successes, conflicts)
	}

	var revisionCount int
	if err := first.db.QueryRowContext(ctx, `SELECT count(*) FROM canonical_revisions`).Scan(&revisionCount); err != nil {
		t.Fatalf("count revisions: %v", err)
	}

	if revisionCount != 1 {
		t.Fatalf("revision count = %d, want 1", revisionCount)
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

func assertHeadAndCounts(
	t *testing.T,
	ctx context.Context,
	store *Store,
	wantHead string,
	wantRevisions int,
) {
	t.Helper()
	head, err := store.Head(ctx)
	if err != nil {
		t.Fatalf("Head() error = %v", err)
	}
	if head == nil || head.ID != wantHead {
		t.Fatalf("Head() = %+v, want %q", head, wantHead)
	}

	var revisions int
	if err := store.db.QueryRowContext(ctx, `SELECT count(*) FROM canonical_revisions`).Scan(&revisions); err != nil {
		t.Fatalf("count revisions: %v", err)
	}
	if revisions != wantRevisions {
		t.Fatalf("revision count = %d, want %d", revisions, wantRevisions)
	}

}

func stringsOf(value byte, count int) string {
	buffer := make([]byte, count)
	for i := range buffer {
		buffer[i] = value
	}
	return string(buffer)
}
