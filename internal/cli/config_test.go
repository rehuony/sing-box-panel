// SPDX-License-Identifier: GPL-3.0-or-later

package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rehuony/sing-box-panel/internal/settings"
	"github.com/rehuony/sing-box-panel/internal/testutil"
)

func runPanelConfig(t *testing.T, ctx context.Context, path string, input io.Reader, args ...string) (string, error) {
	t.Helper()
	var out, diagnostics bytes.Buffer
	root := NewRootCommand(Dependencies{
		Stdin: input, Stdout: &out, Stderr: &diagnostics,
	})
	root.SetArgs(append([]string{"--config", path, "config"}, args...))
	err := root.ExecuteContext(ctx)
	if diagnostics.Len() != 0 {
		t.Fatalf("unexpected diagnostics: %s", diagnostics.String())
	}
	return out.String(), err
}

func TestPanelConfigShowPreservesBytes(t *testing.T) {
	for _, content := range []string{"{", "{}", "{\n  \"auth\": {\"token\": \"fixture\"}\n}\n"} {
		for _, format := range []string{"text", "json", "jsonl"} {
			t.Run(content+"/"+format, func(t *testing.T) {
				path := filepath.Join(t.TempDir(), "setting.json")
				if err := os.WriteFile(path, []byte(content), 0600); err != nil {
					t.Fatal(err)
				}
				out, err := runPanelConfig(t, t.Context(), path, nil, "show", "-o", format)
				if err != nil {
					t.Fatal(err)
				}
				if format == "text" {
					if out != content {
						t.Fatalf("show changed bytes: %q", out)
					}
				} else {
					var got struct {
						SettingsPath string `json:"settings_path"`
						Content      string `json:"content"`
					}
					if err := json.Unmarshal([]byte(out), &got); err != nil || got.Content != content || got.SettingsPath != path {
						t.Fatalf("output = %q, error = %v", out, err)
					}
				}
			})
		}
	}
}

func TestPanelConfigFileOperationsDoNotOpenStorage(t *testing.T) {
	for _, storage := range []string{"missing", "invalid database", "data path is a file"} {
		for _, format := range []string{"text", "json", "jsonl"} {
			t.Run(storage+"/"+format, func(t *testing.T) {
				dir := t.TempDir()
				dataDir := filepath.Join(dir, "data")
				var sentinel string
				switch storage {
				case "invalid database":
					if err := os.Mkdir(dataDir, 0700); err != nil {
						t.Fatal(err)
					}
					sentinel = filepath.Join(dataDir, "panel.db")
				case "data path is a file":
					sentinel = dataDir
				}
				if sentinel != "" {
					if err := os.WriteFile(sentinel, []byte("untouched"), 0600); err != nil {
						t.Fatal(err)
					}
				}
				value := settings.Defaults()
				value.DataDir = "../data"
				value.Auth.Email = testutil.AdminEmail
				value.Auth.PasswordHash = testutil.PasswordHash
				data, err := json.MarshalIndent(value, "", "  ")
				if err != nil {
					t.Fatal(err)
				}
				path := filepath.Join(dir, "config", "setting.json")
				out, err := runPanelConfig(t, t.Context(), path, bytes.NewReader(data), "set", "--file", "-", "-o", format)
				if err != nil {
					t.Fatal(err)
				}
				if format != "text" && !strings.Contains(out, `"saved":true`) {
					t.Fatal(out)
				}
				out, err = runPanelConfig(t, t.Context(), path, nil, "verify", "-o", format)
				if err != nil {
					t.Fatalf("verify probed storage: %v", err)
				}
				if format == "text" && out != "panel settings are valid\n" || format != "text" && !strings.Contains(out, `"valid":true`) {
					t.Fatal(out)
				}
				shown, err := runPanelConfig(t, t.Context(), path, nil, "show")
				if err != nil || shown != string(data) {
					t.Fatalf("show = %q, %v", shown, err)
				}
				loaded, err := settings.Load(path)
				if err != nil || loaded.DataDir != dataDir {
					t.Fatalf("destination path resolution = %q, %v", loaded.DataDir, err)
				}
				if sentinel == "" {
					if _, err := os.Stat(dataDir); !errors.Is(err, os.ErrNotExist) {
						t.Fatalf("config created data directory: %v", err)
					}
				} else {
					got, err := os.ReadFile(sentinel)
					if err != nil || string(got) != "untouched" {
						t.Fatalf("storage changed: %q, %v", got, err)
					}
				}
			})
		}
	}
}

func TestPanelConfigSetValidatesBeforeReplacing(t *testing.T) {
	path := commandSettingsFixture(t)
	original, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, invalid := range []string{"{", "{}", "null", string(original) + "{}", strings.Replace(string(original), `"port":3000`, `"port":0`, 1), strings.Replace(string(original), `"port":3000`, `"port":3000,"port":3001`, 1), strings.Replace(string(original), `"port":3000`, `"port":3000,"unknown":true`, 1), strings.Repeat(" ", settings.MaximumBytes+1)} {
		out, err := runPanelConfig(t, t.Context(), path, strings.NewReader(invalid), "set", "--file", "-")
		if ExitCode(err) != 3 || out != "" {
			t.Fatalf("invalid set: output=%q, error=%v", out, err)
		}
		after, err := os.ReadFile(path)
		if err != nil || !bytes.Equal(after, original) {
			t.Fatal("invalid input changed existing settings")
		}
	}
	// Replacing an invalid destination is supported; input comes from a separate file.
	if err := os.WriteFile(path, []byte("{"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0644); err != nil {
		t.Fatal(err)
	}
	input := filepath.Join(t.TempDir(), "replacement.json")
	if err := os.WriteFile(input, original, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := runPanelConfig(t, t.Context(), path, nil, "set", "--file", input); err != nil {
		t.Fatal(err)
	}
	after, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(after, original) {
		t.Fatal("valid replacement changed input bytes")
	}
	info, err := os.Stat(path)
	if err != nil || info.Mode().Perm() != 0600 {
		t.Fatalf("private mode not enforced: %v, %v", info, err)
	}
}

func TestPanelConfigReadErrorsAndSetUsage(t *testing.T) {
	for name, path := range unavailableSettingsFixtures(t) {
		if name != "malformed" && name != "invalid" {
			if out, err := runPanelConfig(t, t.Context(), path, nil, "show"); ExitCode(err) != 3 || out != "" {
				t.Fatalf("%s show = %q, %v", name, out, err)
			}
		}
		if out, err := runPanelConfig(t, t.Context(), path, nil, "verify"); ExitCode(err) != 3 || out != "" {
			t.Fatalf("%s verify = %q, %v", name, out, err)
		}
	}
	path := filepath.Join(t.TempDir(), "setting.json")
	for _, args := range [][]string{
		{"set"}, {"set", "--file="}, {"set", "server.port"},
		{"set", "server.port", "9090", "extra"}, {"set", "--stdin"},
		{"set", "server.port", "9090", "--stdin"}, {"set", "--file=-", "--stdin"},
		{"set", "server.port", "--file=-"}, {"set", "server.port", "9090", "--file=-"},
		{"verify", "--core=x"}, {"set", "--revision=0"},
	} {
		if out, err := runPanelConfig(t, t.Context(), path, nil, args...); ExitCode(err) != 2 || out != "" {
			t.Fatalf("usage %v = %q, %v", args, out, err)
		}
	}
	if out, err := runPanelConfig(t, t.Context(), path, nil, "set", "--file", path+".missing"); ExitCode(err) != 3 || out != "" {
		t.Fatalf("missing input = %q, %v", out, err)
	}
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("failed operation created settings")
	}
}

func TestPanelConfigCancellationDoesNotWrite(t *testing.T) {
	path := commandSettingsFixture(t)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		args  []string
		input string
	}{
		{[]string{"set", "--file=-"}, string(data)},
		{[]string{"set", "server.port", "--stdin"}, "9090"},
		{[]string{"set", "auth.password", "--stdin"}, testutil.ChangedPassword},
	} {
		ctx, cancel := context.WithCancel(t.Context())
		reader := &cancelSettingsReader{Reader: strings.NewReader(tc.input), cancel: cancel}
		out, err := runPanelConfig(t, ctx, path, reader, tc.args...)
		cancel()
		if !errors.Is(err, context.Canceled) || out != "" {
			t.Fatalf("cancelled set = %q, %v", out, err)
		}
		after, _ := os.ReadFile(path)
		if !bytes.Equal(data, after) {
			t.Fatal("cancelled set changed settings")
		}
	}
}

func TestPanelConfigSetField(t *testing.T) {
	for _, format := range []string{"text", "json", "jsonl"} {
		t.Run(format, func(t *testing.T) {
			path := commandSettingsFixture(t)
			before, err := settings.Load(path)
			if err != nil {
				t.Fatal(err)
			}
			for _, tc := range []struct {
				args  []string
				input string
			}{
				{[]string{"set", "server.port", "9090"}, ""},
				{[]string{"set", "github.token", "--stdin"}, "  12345  \r\n"},
				{[]string{"set", "subscription.private_source_cidrs", `["10.0.0.0/8"]`}, ""},
			} {
				out, err := runPanelConfig(t, t.Context(), path, strings.NewReader(tc.input), append(tc.args, "-o", format)...)
				if err != nil || out == "" || (format != "text" && !strings.Contains(out, `"saved":true`)) {
					t.Fatalf("set field %v: %q, %v", tc.args, out, err)
				}
				if strings.Contains(out, "12345") {
					t.Fatal("set exposed the field value")
				}
			}
			after, err := settings.Load(path)
			if err != nil || after.Server.Port != 9090 || after.GitHub.Token != "  12345  " ||
				len(after.Subscription.PrivateSourceCIDRs) != 1 || after.Subscription.PrivateSourceCIDRs[0] != "10.0.0.0/8" ||
				after.Auth != before.Auth || after.DataDir != before.DataDir {
				t.Fatal("targeted edit changed the wrong values", err)
			}
			if files, err := os.ReadDir(before.DataDir); err != nil || len(files) != 0 {
				t.Fatal("field edit created storage files", err)
			}
			original, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			for _, args := range [][]string{
				{"set", "server.port", "zero"}, {"set", "server.port", "0"},
				{"set", "unknown", "secret-value"}, {"set", "auth.password", "secret"},
				{"set", "/server/port", "9090"}, {"set", "/auth/password", testutil.ChangedPassword},
				{"set", "server.port", ""}, {"set", "auth.password", ""},
			} {
				out, err := runPanelConfig(t, t.Context(), path, nil, append(args, "-o", format)...)
				if ExitCode(err) != 3 || out != "" || strings.Contains(err.Error(), "secret") {
					t.Fatalf("invalid field edit leaked input or succeeded: %q, %v", out, err)
				}
				data, readErr := os.ReadFile(path)
				if readErr != nil || !bytes.Equal(original, data) {
					t.Fatal("invalid field edit changed settings", readErr)
				}
			}
		})
	}
}

func TestPanelConfigGetField(t *testing.T) {
	path := commandSettingsFixture(t)
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	hash, _ := json.Marshal(testutil.PasswordHash)
	for _, tc := range []struct{ field, text, json string }{
		{"server.port", "3000", "3000"},
		{"server.host", "127.0.0.1", `"127.0.0.1"`},
		{"github.token", "", `""`},
		{"traffic.quota_gib", "null", "null"},
		{"subscription.private_source_cidrs", "[]", "[]"},
		{"server", `{"host":"127.0.0.1","port":3000,"base_path":""}`, `{"host":"127.0.0.1","port":3000,"base_path":""}`},
		{"auth.password_hash", testutil.PasswordHash, string(hash)},
	} {
		for _, format := range []string{"text", "json", "jsonl"} {
			t.Run(tc.field+"/"+format, func(t *testing.T) {
				out, err := runPanelConfig(t, t.Context(), path, nil, "get", tc.field, "-o", format)
				if err != nil {
					t.Fatal(err)
				}
				if format == "text" {
					if out != tc.text+"\n" {
						t.Fatalf("get text = %q, want %q", out, tc.text+"\n")
					}
					return
				}
				var result struct {
					Path  string          `json:"settings_path"`
					Field string          `json:"field"`
					Value json.RawMessage `json:"value"`
				}
				if err := json.Unmarshal([]byte(out), &result); err != nil || result.Path != path || result.Field != tc.field || string(result.Value) != tc.json {
					t.Fatalf("get structured output = %q, error = %v", out, err)
				}
			})
		}
	}
	for _, tc := range []struct {
		args []string
		code int
	}{
		{[]string{"get"}, 2}, {[]string{"get", "server.port", "extra"}, 2},
		{[]string{"get", "/server/port"}, 3}, {[]string{"get", "server/port"}, 3},
		{[]string{"get", "unknown"}, 3}, {[]string{"get", "panel.language"}, 3},
		{[]string{"get", "auth.password"}, 3},
	} {
		if out, err := runPanelConfig(t, t.Context(), path, nil, tc.args...); ExitCode(err) != tc.code || out != "" {
			t.Fatalf("invalid get %v: %q, %v", tc.args, out, err)
		}
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if out, err := runPanelConfig(t, ctx, path, nil, "get", "server.port"); !errors.Is(err, context.Canceled) || out != "" {
		t.Fatal("get ignored cancellation", err)
	}
	after, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(before, after) {
		t.Fatal("get changed settings", err)
	}
	if _, err := os.Stat(path + ".lock"); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("get created a writer lock", err)
	}
	loaded, err := settings.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if files, err := os.ReadDir(loaded.DataDir); err != nil || len(files) != 0 {
		t.Fatal("get opened storage", err)
	}
}

func TestPanelConfigSetClearsOptionalValues(t *testing.T) {
	path := commandSettingsFixture(t)
	for _, tc := range []struct{ field, populated, cleared, output string }{
		{"github.token", "secret-token", "", "\n"},
		{"server.external_origin", "https://panel.example.com", "", "\n"},
		{"traffic.quota_gib", "42", "null", "null\n"},
		{"subscription.private_source_cidrs", `["10.0.0.0/8"]`, "[]", "[]\n"},
	} {
		for _, value := range []string{tc.populated, tc.cleared} {
			if _, err := runPanelConfig(t, t.Context(), path, nil, "set", tc.field, value); err != nil {
				t.Fatalf("set %s failed: %v", tc.field, err)
			}
		}
		out, err := runPanelConfig(t, t.Context(), path, nil, "get", tc.field)
		if err != nil || out != tc.output {
			t.Fatalf("cleared %s = %q, error = %v", tc.field, out, err)
		}
	}
	if _, err := settings.Load(path); err != nil {
		t.Fatal("clearing optional fields invalidated settings", err)
	}
}

func TestPanelConfigInitOnlyGeneratesSettings(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("requires isolated user-scoped XDG defaults")
	}
	for _, format := range []string{"text", "json", "jsonl"} {
		t.Run(format, func(t *testing.T) {
			root := t.TempDir()
			t.Setenv("XDG_DATA_HOME", filepath.Join(root, "data-home"))
			path := filepath.Join(root, "config", "setting.json")
			out, err := runPanelConfig(t, t.Context(), path, nil, "init", "-o", format)
			if err != nil {
				t.Fatal(err)
			}
			value, err := settings.Load(path)
			if err != nil || len(value.Subscription.PrivateSourceCIDRs) != 0 || value.Auth.PasswordHash == "" {
				t.Fatal("init did not generate valid defaults", err)
			}
			shown, err := runPanelConfig(t, t.Context(), path, nil, "show")
			if err != nil {
				t.Fatal(err)
			}
			for _, field := range []string{`"author"`, `"provider"`, `"retention_days"`} {
				if strings.Contains(shown, field) {
					t.Fatalf("initialization generated removed field %s", field)
				}
			}
			password := initialPasswordFromOutput(t, out, value.Auth.PasswordHash)
			if !strings.Contains(out, path) {
				t.Fatal("initialization summary omitted the path or token")
			}
			if format != "text" {
				var result struct {
					Initialized   bool   `json:"initialized"`
					SettingsPath  string `json:"settings_path"`
					LoginPassword string `json:"login_password"`
				}
				if err := json.Unmarshal([]byte(out), &result); err != nil || !result.Initialized || result.SettingsPath != path || result.LoginPassword != password {
					t.Fatal("invalid initialization result", err)
				}
			}
			if _, err := os.Stat(value.DataDir); !errors.Is(err, os.ErrNotExist) {
				t.Fatal("config init created storage", err)
			}
			before, _ := os.ReadFile(path)
			if out, err := runPanelConfig(t, t.Context(), path, nil, "init"); ExitCode(err) != 3 || out != "" {
				t.Fatal("init did not preserve the existing file", err)
			}
			after, _ := os.ReadFile(path)
			if !bytes.Equal(before, after) {
				t.Fatal("existing settings changed")
			}
			if _, err := runPanelConfig(t, t.Context(), path, nil, "init", "--force"); err != nil {
				t.Fatal(err)
			}
			replacement, err := settings.Load(path)
			if err != nil || replacement.Auth.PasswordHash == value.Auth.PasswordHash {
				t.Fatal("forced initialization did not generate a new token", err)
			}
			for target, mode := range map[string]os.FileMode{path: 0600, filepath.Dir(path): 0700} {
				info, err := os.Stat(target)
				if err != nil || info.Mode().Perm() != mode {
					t.Fatal("initialization permissions are not private", err)
				}
			}
		})
	}
}

func TestPanelConfigRejectsRemovedFieldsAndForceInitDoesNotRegenerateThem(t *testing.T) {
	for _, tc := range []struct{ section, field, value string }{
		{"subscription", "author", `"old-author"`},
		{"subscription", "provider", `"old-provider"`},
		{"logs", "retention_days", `7`},
	} {
		t.Run(tc.section+"."+tc.field, func(t *testing.T) {
			path := commandSettingsFixture(t)
			before, err := settings.Read(path)
			if err != nil {
				t.Fatal(err)
			}
			var root map[string]json.RawMessage
			if err := json.Unmarshal(before, &root); err != nil {
				t.Fatal(err)
			}
			var section map[string]json.RawMessage
			if err := json.Unmarshal(root[tc.section], &section); err != nil {
				t.Fatal(err)
			}
			section[tc.field] = json.RawMessage(tc.value)
			root[tc.section], _ = json.Marshal(section)
			legacy, _ := json.Marshal(root)
			if _, err := runPanelConfig(t, t.Context(), path, bytes.NewReader(legacy), "set", "--file=-"); ExitCode(err) != 3 || !strings.Contains(err.Error(), "unknown field") {
				t.Fatalf("removed field accepted by set: %v", err)
			}
			if _, err := runPanelConfig(t, t.Context(), path, nil, "set", tc.section+"."+tc.field, tc.value); ExitCode(err) != 3 {
				t.Fatalf("removed field accepted by field set: %v", err)
			}
			after, _ := settings.Read(path)
			if !bytes.Equal(before, after) {
				t.Fatal("invalid edit changed settings")
			}
			if err := os.WriteFile(path, legacy, 0600); err != nil {
				t.Fatal(err)
			}
			if _, err := runPanelConfig(t, t.Context(), path, nil, "verify"); ExitCode(err) != 3 || !strings.Contains(err.Error(), "unknown field") {
				t.Fatalf("removed field accepted by verify: %v", err)
			}
			if _, err := runPanelConfig(t, t.Context(), path, nil, "init", "--force"); err != nil {
				t.Fatal(err)
			}
			shown, err := runPanelConfig(t, t.Context(), path, nil, "show")
			if err != nil || strings.Contains(shown, `"`+tc.field+`"`) {
				t.Fatal("force initialization regenerated removed field", err)
			}
		})
	}
}

func TestPanelConfigInitCancellationDoesNotCreateFiles(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config", "setting.json")
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if out, err := runPanelConfig(t, ctx, path, nil, "init"); !errors.Is(err, context.Canceled) || out != "" {
		t.Fatal("init ignored cancellation", err)
	}
	if _, err := os.Stat(filepath.Dir(path)); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("cancelled initialization created files", err)
	}
}

type cancelSettingsReader struct {
	io.Reader
	cancel context.CancelFunc
}

func (r *cancelSettingsReader) Read(p []byte) (int, error) {
	n, err := r.Reader.Read(p)
	r.cancel()
	return n, err
}

func TestPanelConfigRejectsRemovedIdentityFields(t *testing.T) {
	path := filepath.Join(t.TempDir(), "setting.json")
	if _, err := runPanelConfig(t, t.Context(), path, nil, "init"); err != nil {
		t.Fatal(err)
	}
	before, err := settings.Read(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{"identity_name", "identity_key"} {
		t.Run(field, func(t *testing.T) {
			var fields map[string]any
			if err := json.Unmarshal(before, &fields); err != nil {
				t.Fatal(err)
			}
			panel := fields["panel"].(map[string]any)
			if _, exists := panel[field]; exists {
				t.Fatalf("config init emitted %s", field)
			}
			panel[field] = "removed"
			raw, err := json.Marshal(fields)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := runPanelConfig(t, t.Context(), path, bytes.NewReader(raw), "set", "--file", "-"); err == nil {
				t.Fatal("removed field accepted")
			}
			if _, err := runPanelConfig(t, t.Context(), path, nil, "set", "panel."+field, "removed"); err == nil {
				t.Fatal("removed field accepted by field set")
			}
			after, err := settings.Read(path)
			if err != nil || !bytes.Equal(before, after) {
				t.Fatal("rejected input changed settings", err)
			}
			legacyPath := filepath.Join(t.TempDir(), "setting.json")
			if err := os.WriteFile(legacyPath, raw, 0600); err != nil {
				t.Fatal(err)
			}
			if _, err := runPanelConfig(t, t.Context(), legacyPath, nil, "verify"); err == nil {
				t.Fatal("verification accepted removed field")
			}
			if _, err := runPanelConfig(t, t.Context(), legacyPath, nil, "init", "--force"); err != nil {
				t.Fatal(err)
			}
			initialized, err := settings.Read(legacyPath)
			if err != nil || bytes.Contains(initialized, []byte(field)) {
				t.Fatal("reinitialization retained removed field", err)
			}
		})
	}
}
