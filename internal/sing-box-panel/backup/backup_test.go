// SPDX-License-Identifier: GPL-3.0-or-later

package backup

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/rehuony/sing-box-panel/internal/sing-box-panel/settings"
	"github.com/rehuony/sing-box-panel/internal/sing-box-panel/store"
	"github.com/rehuony/sing-box-panel/internal/sing-box-panel/testutil"
)

// panel is one panel installation in a test: its settings file and data
// directory.
type panel struct {
	settingsPath, dataDir string
}

func newPanel(t *testing.T, port int, prepare func(*store.Store)) panel {
	t.Helper()
	root := t.TempDir()
	installation := panel{settingsPath: filepath.Join(root, "setting.json"), dataDir: filepath.Join(root, "data")}
	if err := os.Mkdir(installation.dataDir, 0o700); err != nil {
		t.Fatal(err)
	}
	value := settings.Defaults()
	value.DataDir, value.Server.Port = installation.dataDir, port
	value.Auth = settings.Auth{Email: testutil.AdminEmail, PasswordHash: testutil.ChangedPasswordHash}
	raw, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(installation.settingsPath, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	database, err := store.Open(t.Context(), filepath.Join(installation.dataDir, store.PanelFile))
	if err != nil {
		t.Fatal(err)
	}
	if prepare != nil {
		prepare(database)
	}
	if err := database.Close(); err != nil {
		t.Fatal(err)
	}
	return installation
}

func (installation panel) service(now time.Time) *Service {
	return New(Options{
		DataDir: installation.dataDir, SettingsPath: installation.settingsPath, PanelVersion: "v1.2.3",
		Document: func(context.Context) ([]byte, error) { return settings.Read(installation.settingsPath) },
		Now:      func() time.Time { return now },
	})
}

// TestBackupRestoresAnotherPanelAtStartup moves a panel to another host:
// an encrypted backup of its database and settings is verified, staged and
// applied before the target opens its databases, keeping the target's
// listener and data directory and backing the target up first.
func TestBackupRestoresAnotherPanelAtStartup(t *testing.T) {
	ctx := t.Context()
	now := time.Date(2026, time.November, 27, 10, 0, 0, 0, time.UTC)
	source := newPanel(t, 3100, func(database *store.Store) {
		testutil.CreateNode(t, database, "edge")
		if _, err := database.PanelID(ctx, "panel-a"); err != nil {
			t.Fatal(err)
		}
	})
	info, err := source.service(now).Create(ctx, CreateOptions{Kind: KindManual, Categories: Categories, Passphrase: "correct horse"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	// What an encrypted backup holds reads without its passphrase.
	if !info.Encrypted || info.Kind != KindManual || !strings.HasSuffix(info.Name, "-manual.tar.gz") ||
		len(info.Categories) != len(Categories) || info.Counts.Nodes != 1 {
		t.Fatalf("backup = %+v", info)
	}

	target := newPanel(t, 3200, nil)
	restore := target.service(now.Add(time.Hour))
	archive := filepath.Join(source.dataDir, DirectoryName, info.Name)
	for passphrase, want := range map[string]error{"": ErrPassphraseRequired, "wrong": ErrPassphraseIncorrect} {
		file, err := os.Open(archive)
		if err != nil {
			t.Fatal(err)
		}
		_, err = restore.Stage(ctx, file, passphrase)
		_ = file.Close()
		if !errors.Is(err, want) {
			t.Fatalf("Stage(%q) error = %v, want %v", passphrase, err, want)
		}
	}
	file, err := os.Open(archive)
	if err != nil {
		t.Fatal(err)
	}
	staged, err := restore.Stage(ctx, file, "correct horse")
	_ = file.Close()
	if err != nil {
		t.Fatalf("Stage: %v", err)
	}
	if staged.Manifest.Snapshot.Counts.Nodes != 1 || staged.Manifest.Snapshot.PanelID != "panel-a" || !staged.Manifest.Holds(CategoryLogs) ||
		staged.Server == nil || staged.Server.Port != 3100 || staged.SamePanel {
		t.Fatalf("staged = %+v", staged)
	}
	if plan, err := restore.Plan(ctx, staged.ID, Categories); err != nil || !plan.SignsOut || plan.Effects != (store.MergeEffects{}) {
		t.Fatalf("Plan = %+v, %v", plan, err)
	}
	before, err := restore.RequestRestore(ctx, staged.ID, false, Categories)
	if err != nil || before.Kind != KindPreRestore {
		t.Fatalf("RequestRestore = %+v, %v", before, err)
	}

	applied, err := ApplyPending(ctx, target.settingsPath, target.dataDir)
	if err != nil || applied == nil || applied.Skipped != "" || applied.Manifest.Snapshot.PanelID != "panel-a" {
		t.Fatalf("ApplyPending = %+v, %v", applied, err)
	}
	database, err := store.Open(ctx, filepath.Join(target.dataDir, store.PanelFile))
	if err != nil {
		t.Fatalf("open restored database: %v", err)
	}
	nodes, err := database.Nodes(ctx)
	id, idErr := database.CurrentPanelID(ctx)
	_ = database.Close()
	if err != nil || idErr != nil || len(nodes) != 1 || nodes[0].Name != "edge" || id != "panel-a" {
		t.Fatalf("restored nodes = %+v (%v), panel id = %q (%v)", nodes, err, id, idErr)
	}
	restored, err := settings.Load(target.settingsPath)
	if err != nil || restored.Server.Port != 3200 || restored.DataDir != target.dataDir {
		t.Fatalf("restored settings = %+v, %v", restored.Server, err)
	}
	if again, err := ApplyPending(ctx, target.settingsPath, target.dataDir); again != nil || err != nil {
		t.Fatalf("second ApplyPending = %+v, %v", again, err)
	}
	backups, err := restore.List(ctx)
	if err != nil || len(backups) != 1 || backups[0].Name != before.Name {
		t.Fatalf("target backups = %+v, %v", backups, err)
	}
}

// TestBackupStagingRejectsAChangedArchive refuses an archive whose file does
// not match its manifest and leaves nothing staged.
func TestBackupStagingRejectsAChangedArchive(t *testing.T) {
	ctx := t.Context()
	installation := newPanel(t, 3100, nil)
	service := installation.service(time.Now())
	info, err := service.Create(ctx, CreateOptions{Kind: KindManual, Categories: DefaultCategories})
	if err != nil {
		t.Fatal(err)
	}
	manifest, err := readManifest(filepath.Join(service.Directory(), info.Name))
	if err != nil {
		t.Fatal(err)
	}
	manifest.Files[1].SHA256 = strings.Repeat("0", 64)
	var changed strings.Builder
	stage := t.TempDir()
	for _, file := range manifest.Files {
		if err := os.WriteFile(filepath.Join(stage, file.Name), []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	manifest.Files[0].Size, manifest.Files[1].Size = 1, 1
	if err := writeArchive(&changed, manifest, stage, ""); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Stage(ctx, strings.NewReader(changed.String()), ""); !errors.Is(err, ErrInvalid) {
		t.Fatalf("Stage error = %v, want ErrInvalid", err)
	}
	entries, err := os.ReadDir(service.Directory())
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), stagePrefix) {
			t.Fatalf("a rejected archive left %s", entry.Name())
		}
	}
}

// TestPruneKeepsTheNewestScheduledBackups removes only scheduled backups,
// oldest first.
func TestPruneKeepsTheNewestScheduledBackups(t *testing.T) {
	ctx := t.Context()
	installation := newPanel(t, 3100, nil)
	start := time.Date(2026, time.November, 27, 10, 0, 0, 0, time.UTC)
	for index, kind := range []Kind{KindScheduled, KindManual, KindScheduled, KindScheduled} {
		if _, err := installation.service(start.Add(time.Duration(index)*time.Hour)).Create(ctx, CreateOptions{Kind: kind, Categories: DefaultCategories}); err != nil {
			t.Fatal(err)
		}
	}
	service := installation.service(start)
	if removed, err := service.PruneScheduled(ctx, 2); err != nil || removed != 1 {
		t.Fatalf("PruneScheduled = %d, %v", removed, err)
	}
	backups, err := service.List(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var kinds []string
	for _, backup := range backups {
		kinds = append(kinds, backup.CreatedAt.Format("15")+":"+string(backup.Kind))
	}
	if strings.Join(kinds, ",") != "13:scheduled,12:scheduled,11:manual" {
		t.Fatalf("backups = %v", kinds)
	}
}

// TestPartialRestoreRepairsReferencesToRecordsThatAreGone restores the nodes
// and rule sets of another panel into one whose subscriptions refer to its
// own: what referred to the records that are gone goes with them or falls
// back as the schema's rules say, the plan counts exactly what the restore
// changes, and the settings, users and sessions stay.
func TestPartialRestoreRepairsReferencesToRecordsThatAreGone(t *testing.T) {
	ctx := t.Context()
	now := time.Date(2026, time.November, 27, 10, 0, 0, 0, time.UTC)
	source := newPanel(t, 3100, func(database *store.Store) {
		testutil.CreateNode(t, database, "edge")
		execute(t, database, `INSERT INTO subscription_rule_sets (id, client, name, rules, created_at, updated_at)
            VALUES ('rs-a', 'mihomo', 'Streaming', '[]', 'now', 'now')`)
	})
	target := newPanel(t, 3200, func(database *store.Store) {
		node := testutil.CreateNode(t, database, "home")
		execute(t, database,
			`INSERT INTO subscription_rule_sets (id, client, name, rules, created_at, updated_at)
             VALUES ('rs-b', 'mihomo', 'Office', '[]', 'now', 'now')`,
			`INSERT INTO subscription_nodes (id, node_id, origin_key, origin_name, display_name, protocol, outbound_json, position, created_at, updated_at)
             VALUES ('p1', '`+node+`', 'in', 'in', 'Home', 'vless', '{}', 0, 'now', 'now')`,
			`INSERT INTO subscription_channels (id, name, format, default_exit_kind, default_exit_node_id, created_at, updated_at)
             VALUES ('c1', 'Phone', 'mihomo', 'node', 'p1', 'now', 'now')`,
			`INSERT INTO subscription_channel_routes (channel_id, id, position, enabled, rule_set_id, exit_kind, exit_node_id)
             VALUES ('c1', 'r1', 0, 1, 'rs-b', 'node', 'p1')`,
			`INSERT INTO subscription_channel_routes (channel_id, id, position, enabled, kind, value, exit_kind, exit_node_id)
             VALUES ('c1', 'r2', 1, 1, 'domain', 'example.com', 'node', 'p1')`,
			`INSERT INTO subscription_users (id, name, created_at, updated_at) VALUES ('u1', 'Ada', 'now', 'now')`,
			`INSERT INTO subscription_user_grants (user_id, node_id, created_at) VALUES ('u1', 'p1', 'now')`,
			`INSERT INTO traffic_days (node_id, day_start, inbound_bytes, outbound_bytes, first_observed_at, last_sample_at)
             VALUES ('`+node+`', '2026-11-26', 1, 1, 'a', 'b')`,
		)
	})
	partial := []Category{CategoryRuleSets, CategoryNodes}
	info, err := source.service(now).Create(ctx, CreateOptions{Kind: KindManual, Categories: partial})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if !slices.Equal(info.Categories, []Category{CategoryNodes, CategoryRuleSets}) || info.Counts.RuleSets != 1 {
		t.Fatalf("backup = %+v", info)
	}
	restore := target.service(now.Add(time.Hour))
	file, err := os.Open(filepath.Join(source.dataDir, DirectoryName, info.Name))
	if err != nil {
		t.Fatal(err)
	}
	staged, err := restore.Stage(ctx, file, "")
	_ = file.Close()
	if err != nil || staged.Server != nil {
		t.Fatalf("Stage = %+v, %v", staged, err)
	}
	if _, err := restore.Plan(ctx, staged.ID, []Category{CategorySettings}); !errors.Is(err, ErrCategories) {
		t.Fatalf("Plan of a category the backup lacks: %v", err)
	}
	if plan, err := restore.Plan(ctx, staged.ID, []Category{CategoryRuleSets}); err != nil || plan.Effects != (store.MergeEffects{ExitRules: 1}) {
		t.Fatalf("Plan of the rule sets = %+v, %v", plan, err)
	}
	plan, err := restore.Plan(ctx, staged.ID, partial)
	want := store.MergeEffects{PoolNodes: 1, ExitRules: 1, RejectedExits: 1, DirectExits: 1, Grants: 1, HistoryNodes: 1}
	if err != nil || plan.Effects != want || plan.SignsOut {
		t.Fatalf("Plan = %+v, %v", plan, err)
	}
	if _, err := restore.RequestRestore(ctx, staged.ID, false, partial); err != nil {
		t.Fatalf("RequestRestore: %v", err)
	}
	applied, err := ApplyPending(ctx, target.settingsPath, target.dataDir)
	if err != nil || applied == nil || applied.Skipped != "" || applied.Effects != want {
		t.Fatalf("ApplyPending = %+v, %v", applied, err)
	}
	database, err := store.Open(ctx, filepath.Join(target.dataDir, store.PanelFile))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	var got string
	err = database.WithTx(ctx, func(tx *sql.Tx) error {
		return tx.QueryRowContext(ctx, `SELECT
            (SELECT group_concat(name) FROM nodes) || '|' ||
            (SELECT group_concat(name) FROM subscription_rule_sets) || '|' ||
            (SELECT count(*) FROM subscription_nodes) || '|' ||
            (SELECT group_concat(id || ':' || exit_kind) FROM subscription_channel_routes) || '|' ||
            (SELECT default_exit_kind FROM subscription_channels) || '|' ||
            (SELECT count(*) FROM subscription_user_grants) || '|' ||
            (SELECT count(*) FROM subscription_users) || '|' ||
            (SELECT count(*) FROM traffic_days)`).Scan(&got)
	})
	if err != nil || got != "edge|Streaming|0|r2:reject|direct|0|1|0" {
		t.Fatalf("restored records = %q, %v", got, err)
	}
	if kept, err := settings.Load(target.settingsPath); err != nil || kept.Server.Port != 3200 {
		t.Fatalf("settings = %+v, %v", kept.Server, err)
	}
}

// TestEncryptedBackupRefusesAnAlteredManifest refuses an encrypted backup
// whose readable manifest no longer matches the one inside its payload.
func TestEncryptedBackupRefusesAnAlteredManifest(t *testing.T) {
	ctx := t.Context()
	installation := newPanel(t, 3100, nil)
	service := installation.service(time.Now())
	info, err := service.Create(ctx, CreateOptions{Kind: KindManual, Categories: DefaultCategories, Passphrase: "correct horse"})
	if err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(filepath.Join(service.Directory(), info.Name))
	if err != nil {
		t.Fatal(err)
	}
	compressed, err := gzip.NewReader(bytes.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	reader := tar.NewReader(compressed)
	var entries []tarEntry
	for {
		header, err := reader.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		content, err := io.ReadAll(reader)
		if err != nil {
			t.Fatal(err)
		}
		if header.Name == manifestName {
			content = bytes.Replace(content, []byte(`"keys": 0`), []byte(`"keys": 9`), 1)
		}
		entries = append(entries, tarEntry{header.Name, int64(len(content)), bytes.NewReader(content)})
	}
	var altered bytes.Buffer
	manifest, _ := io.ReadAll(entries[0].reader)
	if err := writeTar(&altered, gzip.NoCompression, time.Now(), manifest, "", nil, entries[1]); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Stage(ctx, &altered, "correct horse"); !errors.Is(err, ErrInvalid) {
		t.Fatalf("Stage error = %v, want ErrInvalid", err)
	}
}

func execute(t *testing.T, database *store.Store, statements ...string) {
	t.Helper()
	err := database.WithTx(t.Context(), func(tx *sql.Tx) error {
		for _, statement := range statements {
			if _, err := tx.ExecContext(t.Context(), statement); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
