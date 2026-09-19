package store

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"sync"
	"testing"
)

func TestPanelSettingsMigrationPreservesExistingData(t *testing.T) {
	ctx := testContext(t)
	path := filepath.Join(t.TempDir(), "panel.db")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	initial, err := migrationFiles.ReadFile("migrations/0001_initial.sql")
	if err != nil {
		t.Fatal(err)
	}
	for _, statement := range []string{string(initial), fmt.Sprintf("PRAGMA application_id = %d", ApplicationID),
		"PRAGMA user_version = 1", "INSERT INTO schema_migrations VALUES(1, '0001_initial', '2026-09-01T00:00:00Z')",
		`INSERT INTO subscription_channels(id,name,format,config_json,public_host,created_at,updated_at)
		 VALUES('existing','Existing channel','sing-box','{}','example.com','2026-09-01T00:00:00Z','2026-09-01T00:00:00Z')`,
	} {
		if _, err := db.ExecContext(ctx, statement); err != nil {
			t.Fatal(err)
		}
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	upgraded, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer upgraded.Close()
	var name string
	if err := upgraded.db.QueryRowContext(ctx, "SELECT name FROM subscription_channels WHERE id='existing'").Scan(&name); err != nil || name != "Existing channel" {
		t.Fatalf("existing channel changed: %q %v", name, err)
	}
	channel, err := upgraded.GetSubscriptionChannel(ctx, "existing")
	if err != nil || channel.PublicHost != "example.com" || string(channel.Config) != "{}" || channel.Format != SubscriptionFormatSingBox || !channel.Enabled || channel.CreatedAt.Format("2006-01-02") != "2026-09-01" || !channel.UpdatedAt.Equal(channel.CreatedAt) {
		t.Fatalf("channel migration changed fields: %+v %v", channel, err)
	}
	if _, err := upgraded.CreateSubscriptionChannel(ctx, SubscriptionChannel{ID: "automatic", Name: "Automatic", Format: SubscriptionFormatSingBox, Config: []byte(`{}`), Enabled: true}); err != nil {
		t.Fatalf("empty host unavailable after migration: %v", err)
	}
	info, err := upgraded.SchemaInfo(ctx)
	if err != nil || info.Version != CurrentSchemaVersion {
		t.Fatalf("migration: %+v %v", info, err)
	}
	if document, revision, err := upgraded.PanelSettings(ctx); err != nil || document != nil || revision != 0 {
		t.Fatalf("settings defaults: %s %d %v", document, revision, err)
	}
}

func TestConcurrentPanelSettingsSaveRejectsLostUpdate(t *testing.T) {
	ctx := testContext(t)
	db := openTestStore(t, ctx)
	var writers sync.WaitGroup
	var results [2]error
	start := make(chan struct{})
	for i := range results {
		writers.Add(1)
		go func() {
			defer writers.Done()
			<-start
			_, results[i] = db.SavePanelSettings(ctx, json.RawMessage(fmt.Sprintf(`{"writer":%d}`, i)), 0, nil)
		}()
	}
	close(start)
	writers.Wait()
	succeeded, conflicted := 0, 0
	for _, err := range results {
		switch {
		case err == nil:
			succeeded++
		case errors.Is(err, ErrPanelSettingsConflict):
			conflicted++
		default:
			t.Fatal(err)
		}
	}
	if succeeded != 1 || conflicted != 1 {
		t.Fatalf("writers: succeeded=%d conflicted=%d", succeeded, conflicted)
	}
}
