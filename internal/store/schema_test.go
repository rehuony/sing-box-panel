package store

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/rehuony/sing-box-panel/internal/subscription"
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
	for version := 0; version < 11; version++ {
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

func TestRemoveExportBindingsMigration(t *testing.T) {
	for _, version := range []int{11, 12} {
		for _, fail := range []bool{false, true} {
			t.Run(fmt.Sprintf("version-%d/fail-%t", version, fail), func(t *testing.T) {
				ctx := t.Context()
				db := openTestStore(t, ctx)
				var before []SubscriptionChannel
				for _, id := range []string{"channel-first", "channel-second", "channel-unbound"} {
					channel, err := db.CreateSubscriptionChannel(ctx, SubscriptionChannel{
						ID: id, Name: id, Format: SubscriptionFormatMihomo, Enabled: id != "channel-second",
						Config: json.RawMessage(`{"exclude_tags":["private"],"exclude_types":["http"]}`),
					})
					if err != nil {
						t.Fatal(err)
					}
					before = append(before, channel)
					if id != "channel-unbound" {
						if _, err := db.db.ExecContext(ctx, `UPDATE subscription_channels SET config_json = json_set(config_json, '$.export_token_ids', json('["token-old"]')) WHERE id = ?`, id); err != nil {
							t.Fatal(err)
						}
					}
				}
				if version == 11 {
					if _, err := db.db.ExecContext(ctx, "DROP TABLE traffic_months"); err != nil {
						t.Fatal(err)
					}
				}
				if fail {
					if _, err := db.db.ExecContext(ctx, `CREATE TRIGGER reject_cleanup BEFORE UPDATE OF config_json ON subscription_channels
						WHEN OLD.id = 'channel-second' BEGIN SELECT RAISE(ABORT, 'reject cleanup'); END`); err != nil {
						t.Fatal(err)
					}
				}
				if _, err := db.db.ExecContext(ctx, fmt.Sprintf("PRAGMA user_version = %d", version)); err != nil {
					t.Fatal(err)
				}
				path := db.Path()
				if err := db.Close(); err != nil {
					t.Fatal(err)
				}
				upgraded, err := Open(ctx, path)
				if fail {
					if err == nil {
						upgraded.Close()
						t.Fatal("failed cleanup was accepted")
					}
					raw, err := sql.Open("sqlite", path)
					if err != nil {
						t.Fatal(err)
					}
					defer raw.Close()
					assertPragmaInt(t, ctx, raw, 0, "user_version", version)
					var count int
					if err := raw.QueryRowContext(ctx, `SELECT count(*) FROM subscription_channels WHERE json_type(config_json, '$.export_token_ids') IS NOT NULL`).Scan(&count); err != nil || count != 2 {
						t.Fatalf("partial cleanup persisted: %d %v", count, err)
					}
					if version == 11 {
						if err := raw.QueryRowContext(ctx, `SELECT count(*) FROM sqlite_schema WHERE name = 'traffic_months'`).Scan(&count); err != nil || count != 0 {
							t.Fatal("earlier migration was not rolled back", count, err)
						}
					}
					return
				}
				if err != nil {
					t.Fatal(err)
				}
				defer upgraded.Close()
				// A second initialization must not change the upgraded data.
				for range 2 {
					for _, want := range before {
						got, err := upgraded.GetSubscriptionChannel(ctx, want.ID)
						if err != nil || !reflect.DeepEqual(got, want) {
							t.Fatalf("channel changed beyond bindings: got %+v want %+v error %v", got, want, err)
						}
					}
					if info, err := upgraded.SchemaInfo(ctx); err != nil || info.Version != CurrentSchemaVersion {
						t.Fatal(info, err)
					}
					if err := upgraded.initializeSchema(ctx); err != nil {
						t.Fatal(err)
					}
				}
			})
		}
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

func TestFailedVersion11UpgradeRollsBackSchemaAndVersion(t *testing.T) {
	path := filepath.Join(t.TempDir(), "panel.db")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	// An incomplete version-11 schema cannot supply the migration's source data.
	if _, err := db.ExecContext(t.Context(), fmt.Sprintf("PRAGMA application_id=%d; PRAGMA user_version=11; CREATE TABLE preserved(value TEXT); INSERT INTO preserved VALUES('untouched')", ApplicationID)); err != nil {
		t.Fatal(err)
	}
	opened, err := Open(t.Context(), path)
	if err == nil {
		opened.Close()
		t.Fatal("incomplete migration succeeded")
	}
	assertPragmaInt(t, t.Context(), db, 0, "user_version", 11)
	var count int
	if err := db.QueryRowContext(t.Context(), "SELECT count(*) FROM sqlite_schema WHERE name='traffic_months'").Scan(&count); err != nil || count != 0 {
		t.Fatalf("partial schema persisted: %d %v", count, err)
	}
	var value string
	if err := db.QueryRowContext(t.Context(), "SELECT value FROM preserved").Scan(&value); err != nil || value != "untouched" {
		t.Fatal("existing data changed", err)
	}
}

func TestSubscriptionOrderingMigrationIsAtomic(t *testing.T) {
	for _, fail := range []bool{false, true} {
		t.Run(fmt.Sprint(fail), func(t *testing.T) {
			ctx := t.Context()
			db := openTestStore(t, ctx)
			legacy := `{"policy":{"selection":{"ids":["one"],"excluded_ids":[],"new_node_policy":"exclude"},"organizer":{"prefix":"Old ","exclude_names":["hidden"],"sort":"name","deduplicate":true,"incompatible":"error"},"default_exit":{"kind":"group","id":"main"},"groups":[{"id":"main","name":"Main","enabled":true,"type":"select","node_ids":["one"],"builtin_nodes":["direct","reject"],"candidate_order":["builtin:direct","node:one","builtin:reject"],"default_exit":{"kind":"reject"},"rules":[{"id":"first","kind":"domain","enabled":false,"value":"z.example","exit":{"kind":"group-default"}},{"id":"second","kind":"domain","enabled":true,"value":"a.example","exit":{"kind":"direct"}}]}],"template":{"format":"mihomo","content":"future: 900719925474099312345\n"}}}`
			for _, id := range []string{"a", "b"} {
				_, err := db.CreateSubscriptionChannel(ctx, SubscriptionChannel{ID: id, Name: id, Format: SubscriptionFormatMihomo, Enabled: true, Config: json.RawMessage(`{}`)})
				if err != nil {
					t.Fatal(err)
				}
				if _, err := db.db.ExecContext(ctx, `UPDATE subscription_channels SET config_json=? WHERE id=?`, legacy, id); err != nil {
					t.Fatal(err)
				}
			}
			if fail {
				if _, err := db.db.ExecContext(ctx, `CREATE TRIGGER reject_policy BEFORE UPDATE OF config_json ON subscription_channels WHEN OLD.id='b' BEGIN SELECT RAISE(ABORT,'test failure'); END`); err != nil {
					t.Fatal(err)
				}
			}
			if _, err := db.db.ExecContext(ctx, `PRAGMA user_version=13`); err != nil {
				t.Fatal(err)
			}
			path := db.Path()
			if err := db.Close(); err != nil {
				t.Fatal(err)
			}
			upgraded, err := Open(ctx, path)
			if fail {
				if err == nil {
					upgraded.Close()
					t.Fatal("migration failure ignored")
				}
				raw, err := sql.Open("sqlite", path)
				if err != nil {
					t.Fatal(err)
				}
				defer raw.Close()
				assertPragmaInt(t, ctx, raw, 0, "user_version", 13)
				var unchanged string
				if err := raw.QueryRowContext(ctx, `SELECT config_json FROM subscription_channels WHERE id='a'`).Scan(&unchanged); err != nil || unchanged != legacy {
					t.Fatal("partially persisted migration", unchanged, err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			defer upgraded.Close()
			channel, err := upgraded.GetSubscriptionChannel(ctx, "a")
			if err != nil {
				t.Fatal(err)
			}
			var config SubscriptionChannelConfig
			if err := json.Unmarshal(channel.Config, &config); err != nil {
				t.Fatal(err)
			}
			p := config.Policy
			g := p.Groups[0]
			if p.IncompatibleNodes != "error" || p.DefaultExit.ID != "main" || g.Rules[0].SortIndex != 10 || g.Rules[1].SortIndex != 20 || !reflect.DeepEqual(g.CandidateOrder, []string{"builtin:direct", "node:one", "builtin:reject"}) || p.Template.Content != "future: 900719925474099312345\n" {
				t.Fatalf("migration lost semantics: %+v", config)
			}
			var raw map[string]any
			json.Unmarshal(channel.Config, &raw)
			policy := raw["policy"].(map[string]any)
			if _, exists := policy["organizer"]; exists {
				t.Fatal("organizer survived")
			}
			group := policy["groups"].([]any)[0].(map[string]any)
			if _, exists := group["default_exit"]; exists {
				t.Fatal("default override survived")
			}
			if err := upgraded.initializeSchema(ctx); err != nil {
				t.Fatal(err)
			}
			again, err := upgraded.GetSubscriptionChannel(ctx, "a")
			if err != nil || !reflect.DeepEqual(channel, again) {
				t.Fatal("migration repeated", err)
			}
		})
	}
}

func TestSubscriptionPolicyMigrationRemovesLegacyFilters(t *testing.T) {
	db := openTestStore(t, t.Context())
	nodes, _, err := subscription.ParseSource(subscription.SourceFormatSingBoxJSON, []byte(`{"outbounds":[{"type":"socks","tag":"Tokyo","server":"tokyo.example.com","server_port":1080}]}`), "test-source")
	if err != nil {
		t.Fatal(err)
	}
	id := subscription.PublicationID(nodes[0])
	policy := &subscription.ChannelPolicy{Selection: subscription.NodeSelection{IDs: []string{id}, NewNodePolicy: "exclude"}, DefaultExit: subscription.RouteExit{Kind: "group", ID: "main"}, Groups: []subscription.RuleGroup{{ID: "main", Name: "Main", Enabled: true, Type: "select", NodeIDs: []string{id}, BuiltinNodes: []string{}, Rules: []subscription.ChannelRule{{ID: "rule", Enabled: true, Kind: "domain", Value: "example.com", Exit: subscription.RouteExit{Kind: "group-default"}}}}}}
	raw, _ := json.Marshal(SubscriptionChannelConfig{Policy: policy, ExcludeTypes: []string{"socks"}})
	legacy := strings.Replace(string(raw), `"groups":`, `"organizer":{"prefix":"","exclude_names":[],"sort":"none","deduplicate":false,"incompatible":"skip"},"groups":`, 1)
	legacy = strings.Replace(legacy, `"rules":`, `"default_exit":{"kind":"node","id":"`+id+`"},"rules":`, 1)
	_, err = db.CreateSubscriptionChannel(t.Context(), SubscriptionChannel{ID: "test", Name: "Test", Format: SubscriptionFormatMihomo, Enabled: true, Config: json.RawMessage(`{}`)})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = db.db.ExecContext(t.Context(), `UPDATE subscription_channels SET config_json=? WHERE id='test'`, legacy); err != nil {
		t.Fatal(err)
	}
	if _, err = db.db.ExecContext(t.Context(), `PRAGMA user_version=13`); err != nil {
		t.Fatal(err)
	}
	if err = db.initializeSchema(t.Context()); err != nil {
		t.Fatal(err)
	}
	channel, err := db.GetSubscriptionChannel(t.Context(), "test")
	if err != nil {
		t.Fatal(err)
	}
	config, err := DecodeSubscriptionChannelConfig(channel.Config)
	if err != nil {
		t.Fatal(err)
	}
	result, err := subscription.RenderPolicyNodes(nodes, subscription.RenderChannel{Format: subscription.RenderFormatMihomo, ExcludeTags: config.ExcludeTags, ExcludeTypes: config.ExcludeTypes}, config.Policy)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("selected=%d exported=%d diagnostics=%d contains_reject=%t", len(config.Policy.Selection.IDs), result.NodeCount, len(result.Diagnostics), strings.Contains(string(result.Content), "DOMAIN,example.com,REJECT"))
	if result.NodeCount != 1 {
		t.Errorf("selected visible node was silently filtered after migration")
	}
}

func TestSubscriptionPolicyMigrationPreservesLargeConfig(t *testing.T) {
	db := openTestStore(t, t.Context())
	policy := &subscription.ChannelPolicy{Selection: subscription.NodeSelection{NewNodePolicy: "exclude"}, DefaultExit: subscription.RouteExit{Kind: "direct"}, Groups: []subscription.RuleGroup{{ID: "main", Name: "Main", Enabled: true, Type: "select", NodeIDs: []string{}, BuiltinNodes: []string{"direct"}, Rules: []subscription.ChannelRule{}}}}
	var legacy []byte
	var validCurrent []byte
	for i := 0; i < 5000; i++ {
		rule := subscription.ChannelRule{ID: fmt.Sprint("r", i), Enabled: true, Kind: "domain", Value: strings.Repeat("a", 60) + "." + strings.Repeat("b", 50) + ".example", Exit: subscription.RouteExit{Kind: "group-default"}}
		policy.Groups[0].Rules = append(policy.Groups[0].Rules, rule)
		raw, _ := json.Marshal(SubscriptionChannelConfig{Policy: policy})
		old := strings.Replace(string(raw), `"groups":`, `"organizer":{"prefix":"","exclude_names":[],"sort":"none","deduplicate":false,"incompatible":"skip"},"groups":`, 1)
		old = strings.Replace(old, `"rules":`, `"default_exit":{"kind":"direct"},"rules":`, 1)
		if len(old) > 512<<10 {
			policy.Groups[0].Rules = policy.Groups[0].Rules[:len(policy.Groups[0].Rules)-1]
			break
		}
		legacy = []byte(old)
		validCurrent = raw
	}
	if _, err := canonicalChannelConfig(validCurrent, SubscriptionFormatMihomo); err != nil {
		t.Fatal("fixture invalid", err)
	}
	_, err := db.CreateSubscriptionChannel(t.Context(), SubscriptionChannel{ID: "large", Name: "Large", Format: SubscriptionFormatMihomo, Enabled: true, Config: json.RawMessage(`{}`)})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = db.db.ExecContext(t.Context(), `UPDATE subscription_channels SET config_json=? WHERE id='large'`, string(legacy)); err != nil {
		t.Fatal(err)
	}
	if _, err = db.db.ExecContext(t.Context(), `PRAGMA user_version=13`); err != nil {
		t.Fatal(err)
	}
	if err = db.initializeSchema(t.Context()); err != nil {
		t.Fatal(err)
	}
	var after string
	if err = db.db.QueryRowContext(t.Context(), `SELECT config_json FROM subscription_channels WHERE id='large'`).Scan(&after); err != nil {
		t.Fatal(err)
	}
	_, err = DecodeSubscriptionChannelConfig(json.RawMessage(after))
	t.Logf("rules=%d before=%d after=%d limit=%d decode_error=%v", len(policy.Groups[0].Rules), len(legacy), len(after), maximumChannelConfigBytes, err)
	if err != nil {
		t.Fatal("migration committed an unreadable channel", err)
	}
	if len(after) <= 512<<10 {
		t.Fatal("fixture did not exercise migration growth")
	}
	channel, err := db.GetSubscriptionChannel(t.Context(), "large")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = db.UpdateSubscriptionChannel(t.Context(), UpdateSubscriptionChannelInput{ID: channel.ID, Name: channel.Name, Format: channel.Format, Enabled: channel.Enabled, Config: channel.Config, ExpectedUpdatedAt: channel.UpdatedAt, UpdatedAt: channel.UpdatedAt.Add(time.Second)}); err != nil {
		t.Fatal("migrated config cannot be saved", err)
	}
}
