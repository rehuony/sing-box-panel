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

	"github.com/rehuony/sing-box-panel/internal/application"
	"github.com/rehuony/sing-box-panel/internal/settings"
)

// This runner deliberately fails if a file command acquires application services.
func runPanelConfig(t *testing.T, ctx context.Context, path string, input io.Reader, args ...string) (string, error) {
	t.Helper()
	var out, diagnostics bytes.Buffer
	root := NewRootCommand(Dependencies{
		Stdin: input, Stdout: &out, Stderr: &diagnostics,
		OpenApplication: func(context.Context, string) (*application.Application, error) {
			t.Fatal("panel config opened application services")
			return nil, nil
		},
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
				value.Auth.Token = "fixture-token"
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
	for _, args := range [][]string{{"set"}, {"set", "--file="}, {"set", "/log/level", "--file=-"}, {"verify", "--core=x"}, {"set", "--revision=0"}} {
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
	ctx, cancel := context.WithCancel(t.Context())
	reader := &cancelSettingsReader{Reader: bytes.NewReader(data), cancel: cancel}
	out, err := runPanelConfig(t, ctx, path, reader, "set", "--file=-")
	if !errors.Is(err, context.Canceled) || out != "" {
		t.Fatalf("cancelled set = %q, %v", out, err)
	}
	after, _ := os.ReadFile(path)
	if !bytes.Equal(data, after) {
		t.Fatal("cancelled set changed settings")
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
			if err != nil || value.Subscription.Provider != "default" || value.Auth.Token == "" {
				t.Fatal("init did not generate valid defaults", err)
			}
			if !strings.Contains(out, value.Auth.Token) || !strings.Contains(out, path) {
				t.Fatal("initialization summary omitted the path or token")
			}
			if format != "text" {
				var result struct {
					Initialized  bool   `json:"initialized"`
					SettingsPath string `json:"settings_path"`
					LoginToken   string `json:"login_token"`
				}
				if err := json.Unmarshal([]byte(out), &result); err != nil || !result.Initialized || result.SettingsPath != path || result.LoginToken != value.Auth.Token {
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
			if err != nil || replacement.Auth.Token == value.Auth.Token {
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

func TestPanelConfigUnsetRestoresDefaultsWithoutOpeningStorage(t *testing.T) {
	for _, format := range []string{"text", "json", "jsonl"} {
		path := filepath.Join(t.TempDir(), "setting.json")
		value := settings.Defaults()
		value.Auth.Token = "keep-token"
		value.DataDir = "./absent-data"
		value.Server.Port = 8181
		value.Subscription.Provider = "custom"
		raw, _ := json.Marshal(value)
		if err := os.WriteFile(path, raw, 0600); err != nil {
			t.Fatal(err)
		}
		out, err := runPanelConfig(t, t.Context(), path, nil, "unset", "server.port", "/subscription/provider", "-o", format)
		if err != nil {
			t.Fatal(err)
		}
		if format != "text" && (!strings.Contains(out, `"saved":true`) || !strings.Contains(out, `"reset_fields":["server.port","/subscription/provider"]`)) {
			t.Fatal("invalid structured reset result", out)
		}
		if strings.Contains(out, value.Auth.Token) {
			t.Fatal("reset result exposed a credential")
		}
		loaded, err := settings.Load(path)
		if err != nil || loaded.Server.Port != 3000 || loaded.Subscription.Provider != "default" || loaded.Auth.Token != value.Auth.Token {
			t.Fatal("unset did not restore selected defaults", err)
		}
		if _, err := os.Stat(loaded.DataDir); !errors.Is(err, os.ErrNotExist) {
			t.Fatal("unset opened or created storage", err)
		}
		for _, args := range [][]string{{"unset"}, {"unset", "auth.token"}, {"unset", "unknown"}} {
			out, err := runPanelConfig(t, t.Context(), path, nil, args...)
			want := 3
			if len(args) == 1 {
				want = 2
			}
			if ExitCode(err) != want || out != "" {
				t.Fatalf("invalid reset %v: %q %v", args, out, err)
			}
		}
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

func TestCoreSelectionRejectsExplicitBlankArtifactBeforeOpeningApplication(t *testing.T) {
	for _, args := range [][]string{
		{"core", "enable", ""}, {"core", "enable", " \t "},
	} {
		opened := false
		command := NewRootCommand(Dependencies{
			Stdout: &bytes.Buffer{}, Stderr: &bytes.Buffer{},
			OpenApplication: func(context.Context, string) (*application.Application, error) {
				opened = true
				return nil, errors.New("unexpected application open")
			},
		})
		command.SetArgs(args)
		err := command.ExecuteContext(context.Background())
		if ExitCode(err) != 2 || opened {
			t.Fatalf("blank artifact %q: opened=%t error=%v exit=%d", args, opened, err, ExitCode(err))
		}
	}
}

func TestCoreEnableReportsMissingArtifact(t *testing.T) {
	path := commandSettingsFixture(t)
	saveConfigurationFixture(t, path, "{}")
	command := NewRootCommand(Dependencies{Stdout: &bytes.Buffer{}, Stderr: &bytes.Buffer{}, OpenApplication: application.Open})
	command.SetArgs([]string{"--config", path, "core", "enable", "1.13.21"})
	err := command.ExecuteContext(t.Context())
	var classified *Error
	if ExitCode(err) != 1 || !errors.As(err, &classified) || classified.Code != "core_version_not_installed" {
		t.Fatalf("core enable error = %v", err)
	}
}
