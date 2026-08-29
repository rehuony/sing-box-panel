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
	t.Parallel()

	ctx := testContext(t)
	path := filepath.Join(t.TempDir(), "panel.db")
	legacy, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatalf("open legacy fixture: %v", err)
	}
	if _, err := legacy.ExecContext(ctx, `PRAGMA application_id = 0x53425032`); err != nil {
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
		{name: "invalid JSON", version: 2, document: `{`},
		{name: "schema v1", version: 1, document: `{"schema_version":1,"configuration":{}}`},
		{name: "missing document version", version: 2, document: `{"configuration":{}}`},
		{name: "mismatched document version", version: 2, document: `{"schema_version":1,"configuration":{}}`},
		{name: "non-object document", version: 2, document: `[{"schema_version":2}]`},
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
		`INSERT INTO tasks(id, lane, kind, created_at, updated_at)
		 VALUES ('invalid-lane-kind', 'runtime', 'catalog-refresh', ?, ?)`,
		now, now,
	); err == nil {
		t.Fatal("task with an invalid lane/kind combination succeeded")
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

func TestCanonicalSaveAndTaskAreAtomic(t *testing.T) {
	ctx := testContext(t)
	store := openTestStore(t, ctx)
	createdAt := time.Date(2026, time.August, 26, 12, 0, 0, 0, time.UTC)

	_, err := store.SaveCanonicalRevisionAndTask(
		ctx,
		"",
		NewCanonicalRevision{
			ID: "revision-invalid", SchemaVersion: configuration.SchemaVersion,
			Document:  json.RawMessage(`{"schema_version":2,"configuration":{},"unexpected":true}`),
			CommandID: "command-invalid", CreatedAt: createdAt,
		},
		NewTask{ID: "task-invalid", Lane: TaskLaneMaintenance, Kind: TaskKindCanonicalSaved},
	)
	if !errors.Is(err, configuration.ErrInvalidDocument) {
		t.Fatalf("invalid canonical save error = %v, want ErrInvalidDocument", err)
	}
	if head, headErr := store.Head(ctx); headErr != nil || head != nil {
		t.Fatalf("invalid canonical save changed head: head=%+v err=%v", head, headErr)
	}
	if _, taskErr := store.GetTask(ctx, "task-invalid"); !errors.Is(taskErr, ErrTaskNotFound) {
		t.Fatalf("invalid canonical save created task: %v", taskErr)
	}

	first, err := store.SaveCanonicalRevisionAndTask(
		ctx,
		"",
		NewCanonicalRevision{
			ID:            "revision-1",
			SchemaVersion: 2,
			Document:      json.RawMessage(`{"schema_version":2,"configuration":{"experimental":{"port":8080}}}`),
			CommandID:     "command-1",
			CreatedAt:     createdAt,
		},
		NewTask{
			ID:             "task-1",
			IdempotencyKey: "project-revision-1",
			Lane:           TaskLaneMaintenance,
			Kind:           TaskKindCanonicalSaved,
			Payload:        json.RawMessage(`{"revision":"revision-1"}`),
		},
	)
	if err != nil {
		t.Fatalf("first SaveCanonicalRevisionAndTask() error = %v", err)
	}
	if first.Sequence != 1 || first.ParentID != "" || first.SHA256 == "" {
		t.Fatalf("first revision = %+v, want sequence 1, no parent, and digest", first)
	}

	bootstrap, err := store.Bootstrap(ctx)
	if err != nil {
		t.Fatalf("Bootstrap() after first save error = %v", err)
	}
	if bootstrap.Head == nil || bootstrap.Head.ID != first.ID {
		t.Fatalf("head after first save = %+v, want %q", bootstrap.Head, first.ID)
	}

	_, err = store.SaveCanonicalRevisionAndTask(
		ctx,
		first.ID,
		NewCanonicalRevision{
			ID:            "revision-rolled-back",
			SchemaVersion: 2,
			Document:      json.RawMessage(`{"schema_version":2,"configuration":{"experimental":{"port":9090}}}`),
			CommandID:     "command-rolled-back",
			CreatedAt:     createdAt.Add(time.Minute),
		},
		NewTask{
			ID:      "task-1",
			Lane:    TaskLaneMaintenance,
			Kind:    TaskKindCanonicalSaved,
			Payload: json.RawMessage(`{}`),
		},
	)
	if err == nil {
		t.Fatal("save with duplicate task id succeeded")
	}

	assertHeadAndCounts(t, ctx, store, first.ID, 1, 1)

	second, err := store.SaveCanonicalRevisionAndTask(
		ctx,
		first.ID,
		NewCanonicalRevision{
			ID:            "revision-2",
			SchemaVersion: 2,
			Document:      json.RawMessage(`{"schema_version":2,"configuration":{"experimental":{"port":9090}}}`),
			CommandID:     "command-2",
			CreatedAt:     createdAt.Add(2 * time.Minute),
		},
		NewTask{
			ID:         "task-2",
			Lane:       TaskLaneMaintenance,
			Kind:       TaskKindCanonicalSaved,
			Generation: 1,
			Payload:    json.RawMessage(`{"revision":"revision-2"}`),
		},
	)
	if err != nil {
		t.Fatalf("second SaveCanonicalRevisionAndTask() error = %v", err)
	}
	if second.Sequence != 2 || second.ParentID != first.ID {
		t.Fatalf("second revision = %+v, want sequence 2 parent %q", second, first.ID)
	}

	_, err = store.SaveCanonicalRevisionAndTask(
		ctx,
		first.ID,
		NewCanonicalRevision{
			ID:            "revision-conflict",
			SchemaVersion: 2,
			Document:      json.RawMessage(`{"schema_version":2,"configuration":{"experimental":{"port":10000}}}`),
			CommandID:     "command-conflict",
		},
		NewTask{ID: "task-conflict", Lane: TaskLaneMaintenance, Kind: TaskKindCanonicalSaved},
	)
	if !errors.Is(err, ErrRevisionConflict) {
		t.Fatalf("stale save error = %v, want ErrRevisionConflict", err)
	}
	var conflict *RevisionConflictError
	if !errors.As(err, &conflict) || conflict.ActualHead != second.ID {
		t.Fatalf("stale save conflict = %#v, want actual head %q", conflict, second.ID)
	}

	assertHeadAndCounts(t, ctx, store, second.ID, 2, 2)

	path := store.Path()
	if err := store.Close(); err != nil {
		t.Fatalf("Close() before reopen error = %v", err)
	}
	reopened, err := Open(ctx, path)
	if err != nil {
		t.Fatalf("Open() after canonical saves error = %v", err)
	}
	t.Cleanup(func() { _ = reopened.Close() })
	assertHeadAndCounts(t, ctx, reopened, second.ID, 2, 2)
}

func TestCanonicalCASAcrossStores(t *testing.T) {
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
			_, errorsByWriter[i] = store.SaveCanonicalRevisionAndTask(
				ctx,
				"",
				NewCanonicalRevision{
					ID:            fmt.Sprintf("revision-%d", i),
					SchemaVersion: 2,
					Document:      json.RawMessage(fmt.Sprintf(`{"schema_version":2,"configuration":{"experimental":{"writer":%d}}}`, i)),
					CommandID:     fmt.Sprintf("command-%d", i),
				},
				NewTask{
					ID:      fmt.Sprintf("task-%d", i),
					Lane:    TaskLaneMaintenance,
					Kind:    TaskKindCanonicalSaved,
					Payload: json.RawMessage(`{}`),
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
		case errors.Is(err, ErrRevisionConflict):
			conflicts++
		default:
			t.Fatalf("writer %d error = %v, want success or conflict", i, err)
		}
	}
	if successes != 1 || conflicts != 1 {
		t.Fatalf("writers: successes=%d conflicts=%d, want 1 and 1", successes, conflicts)
	}

	var revisionCount, taskCount int
	if err := first.db.QueryRowContext(ctx, `SELECT count(*) FROM canonical_revisions`).Scan(&revisionCount); err != nil {
		t.Fatalf("count revisions: %v", err)
	}
	if err := first.db.QueryRowContext(ctx, `SELECT count(*) FROM tasks`).Scan(&taskCount); err != nil {
		t.Fatalf("count tasks: %v", err)
	}
	if revisionCount != 1 || taskCount != 1 {
		t.Fatalf("revision/task counts = %d/%d, want 1/1", revisionCount, taskCount)
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
	wantTasks int,
) {
	t.Helper()
	head, err := store.Head(ctx)
	if err != nil {
		t.Fatalf("Head() error = %v", err)
	}
	if head == nil || head.ID != wantHead {
		t.Fatalf("Head() = %+v, want %q", head, wantHead)
	}

	var revisions, tasks int
	if err := store.db.QueryRowContext(ctx, `SELECT count(*) FROM canonical_revisions`).Scan(&revisions); err != nil {
		t.Fatalf("count revisions: %v", err)
	}
	if err := store.db.QueryRowContext(ctx, `SELECT count(*) FROM tasks`).Scan(&tasks); err != nil {
		t.Fatalf("count tasks: %v", err)
	}
	if revisions != wantRevisions || tasks != wantTasks {
		t.Fatalf(
			"revision/task counts = %d/%d, want %d/%d",
			revisions,
			tasks,
			wantRevisions,
			wantTasks,
		)
	}
}

func stringsOf(value byte, count int) string {
	buffer := make([]byte, count)
	for i := range buffer {
		buffer[i] = value
	}
	return string(buffer)
}
