// SPDX-License-Identifier: GPL-3.0-or-later

package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rehuony/sing-box-panel/internal/settings"
)

func TestServerStartInitializesMissingSettings(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root defaults use system directories")
	}
	for _, test := range []struct {
		name, format string
		explicit     bool
	}{
		{"default", "text", false},
		{"explicit relative", "text", true},
		{"json", "json", true},
		{"jsonl", "jsonl", true},
	} {
		t.Run(test.name, func(t *testing.T) {
			directory := t.TempDir()
			t.Chdir(directory)
			t.Setenv("HOME", directory)
			t.Setenv("XDG_CONFIG_HOME", filepath.Join(directory, "config"))
			t.Setenv("XDG_DATA_HOME", filepath.Join(directory, "data"))
			path := settings.DefaultPath()
			args := []string{"server", "start", "--output", test.format}
			if test.explicit {
				path = filepath.Join("custom", "setting.json")
				args = append(args, "-c", path)
			}
			var stdout, stderr bytes.Buffer
			started := 0
			var configuration settings.Settings
			run := func(_ context.Context, selectedPath string) error {
				started++
				if selectedPath != path {
					t.Fatalf("selected path = %q, want %q", selectedPath, path)
				}
				var err error
				configuration, err = settings.Load(selectedPath)
				return err
			}
			command := NewRootCommand(Dependencies{Stdout: &stdout, Stderr: &stderr, RunServer: run})
			command.SetArgs(args)
			if err := command.ExecuteContext(t.Context()); err != nil {
				t.Fatal(err)
			}
			if started != 1 || stdout.Len() != 0 || configuration.Auth.Token == "" {
				t.Fatalf("startup = %d, stdout = %q", started, stdout.String())
			}
			if strings.Count(stderr.String(), configuration.Auth.Token) != 1 {
				t.Fatal("first-run guidance must show the generated token exactly once")
			}
			if strings.Contains(stderr.String(), "\x1b[") {
				t.Fatal("redirected guidance contains terminal escapes")
			}
			if test.format == "text" {
				settingsPath := "~/config/sing-box-panel/setting.json"
				if test.explicit {
					settingsPath = "~/custom/setting.json"
				}
				want := "\nsing-box-panel settings is created\n\n" +
					"  Default URL       http://127.0.0.1:3000/\n" +
					"  Default Token     " + configuration.Auth.Token + "\n" +
					"  Default Settings  " + settingsPath + "\n" +
					"  Default Data Dir  ~/data/sing-box-panel\n\n"
				if stderr.String() != want {
					t.Errorf("initialization banner mismatch:\nwant %q\n got %q", want, stderr.String())
				}
			} else {
				var event map[string]string
				if err := json.Unmarshal(stderr.Bytes(), &event); err != nil {
					t.Fatal(err)
				}
				absolute, err := filepath.Abs(path)
				if err != nil {
					t.Fatal(err)
				}
				if event["event"] != "settings_initialized" || event["settings_path"] != absolute ||
					event["data_dir"] != configuration.DataDir || event["default_panel_url"] != "http://127.0.0.1:3000/" ||
					event["login_token"] != configuration.Auth.Token || event["login_hint"] != "" {
					t.Fatalf("initialization event = %v", event)
				}
			}
			before, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			stderr.Reset()
			command = NewRootCommand(Dependencies{Stdout: &stdout, Stderr: &stderr, RunServer: run})
			command.SetArgs(args)
			if err := command.ExecuteContext(t.Context()); err != nil {
				t.Fatal(err)
			}
			after, err := os.ReadFile(path)
			if err != nil || !bytes.Equal(before, after) || started != 2 || stderr.Len() != 0 {
				t.Fatalf("repeat startup changed settings or repeated first-run guidance: %v", err)
			}
		})
	}
}

func TestServerStartCanceledBeforeInitialization(t *testing.T) {
	path := filepath.Join(t.TempDir(), "setting.json")
	var stdout, stderr bytes.Buffer
	command := NewRootCommand(Dependencies{
		Stdout: &stdout, Stderr: &stderr,
		RunServer: func(context.Context, string) error {
			t.Fatal("canceled command started the server")
			return nil
		},
	})
	command.SetArgs([]string{"server", "start", "--config", path})
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if err := command.ExecuteContext(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v, want canceled", err)
	}
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) || stdout.Len() != 0 || stderr.Len() != 0 {
		t.Fatalf("canceled command initialized settings: %v", err)
	}
}
