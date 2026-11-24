// SPDX-License-Identifier: GPL-3.0-or-later

package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/rehuony/sing-box-panel/internal/cliui"
	"github.com/rehuony/sing-box-panel/internal/procctl"
	"github.com/rehuony/sing-box-panel/internal/sing-box-panel/installation"
	"github.com/rehuony/sing-box-panel/internal/sing-box-panel/settings"
	"github.com/rehuony/sing-box-panel/internal/systemd"
)

func TestInstanceFilesTextShowsExistingContentsWithoutCleanupLabels(t *testing.T) {
	report := instanceFilesReport{
		Report: installation.Report{
			DatabaseIdentity: "unrecognized",
			SettingsPath:     "/srv/config/custom.json",
			DataDir:          "/srv/panel",
			Entries: []installation.Entry{
				{Path: "/opt/bin/panel", Role: "panel executable", State: "file", Cleanup: "retain"},
				{Path: "/srv/config/custom.json", Role: "panel settings", State: "file", Cleanup: "remove"},
				{Path: "/srv/panel", Role: "instance data directory", State: "directory", Cleanup: "remove"},
				{Path: "/srv/panel/empty", Empty: new(true), State: "directory", Cleanup: "remove"},
				{Path: "/srv/panel/panel.db", State: "file", Cleanup: "remove"},
				{Path: "/srv/panel/imports", State: "directory", Cleanup: "remove"},
				{Path: "/srv/panel/imports/link", State: "symlink", Cleanup: "remove_link"},
				{Path: "/srv/panel/runtime", State: "missing", Cleanup: "remove"},
				{Path: "/srv/panel/absent-branch/cache/item", State: "missing", Cleanup: "remove"},
				{Path: "/srv/panel/notes", Role: "instance data", State: "file", Cleanup: "remove"},
			},
		},
		Service: systemd.FilesResult{Scope: systemd.ScopeSystem, Files: []systemd.FileStatus{
			{Path: "/etc/systemd/panel.service", State: "managed", Managed: true},
			{Path: "/etc/systemd/other.service", State: "unmanaged"},
			{Path: "/etc/systemd/missing.service", State: "missing"},
			{Path: "/missing-services/panel.service", State: "missing"},
		}},
		ServiceMatches: true,
	}
	before, err := json.Marshal(report)
	if err != nil {
		t.Fatal(err)
	}
	want := `Instance files

  Config      /srv/config/custom.json
  Executable  /opt/bin/panel
  Service     system, uses these settings

  /
  ├── etc/systemd/
  │   ├── other.service [service, system, outside scope]
  │   └── panel.service [service, system]
  ├── opt/bin/panel [executable, retained, 0 B]
  └── srv/
      ├── config/custom.json [settings, 0 B]
      └── panel/ [data, 0 B]
          ├── empty/ [empty]
          ├── imports/
          │   └── link [link]
          ├── notes [0 B]
          └── panel.db [0 B]`
	if got := instanceFilesText(report, cliui.Style{}); got != want {
		t.Fatalf("output:\n%s\nwant:\n%s", got, want)
	}
	colored := instanceFilesText(report, cliui.Style{Color: true})
	ansi := regexp.MustCompile(`\x1b\[[0-9;]*m`)
	if got := ansi.ReplaceAllString(colored, ""); got != want {
		t.Fatalf("color changed the report content:\n%s", got)
	}
	for _, want := range []string{"\x1b[36mlink", "\x1b[34mpanel/", "\x1b[2m├── "} {
		if !strings.Contains(colored, want) {
			t.Fatalf("missing semantic color %q in %q", want, colored)
		}
	}
	after, err := json.Marshal(report)
	if err != nil || !bytes.Equal(before, after) {
		t.Fatal("rendering changed the source report")
	}
	slices.Reverse(report.Entries)
	slices.Reverse(report.Service.Files)
	if got := instanceFilesText(report, cliui.Style{}); got != want {
		t.Fatalf("report order changed the tree:\n%s", got)
	}
	report.Preview = true
	warning := "Pass --yes to stop this instance and permanently delete its settings and all data."
	wantPreview := strings.Replace(want, "Instance files", "Cleanup preview — no changes", 1) + "\n\n  " + warning
	if got := instanceFilesText(report, cliui.Style{}); got != wantPreview {
		t.Fatalf("preview warning is not after the complete tree:\n%s", got)
	}
	colored = instanceFilesText(report, cliui.Style{Color: true})
	if ansi.ReplaceAllString(colored, "") != wantPreview || strings.Count(colored, "\x1b[1;33m") != 2 {
		t.Fatalf("preview warning color changed content or position: %q", colored)
	}
	report.Preview = false
	report.ServiceMatches = false
	if got := instanceFilesText(report, cliui.Style{}); !strings.Contains(got, "panel.service [service, system, outside scope]") {
		t.Fatalf("unrelated service would be uninstalled:\n%s", got)
	}
	report.DatabaseIdentity = "unknown"
	if got := instanceFilesText(report, cliui.Style{}); strings.Contains(got, "blocked") || strings.Contains(got, "Policies") {
		t.Fatalf("obsolete identity restriction or footer:\n%s", got)
	}
	report.SettingsPath = "/srv/panel/custom.json"
	for _, preview := range []bool{false, true} {
		report.Preview = preview
		if got := instanceFilesText(report, cliui.Style{Home: "/srv"}); !strings.Contains(got, "  Config      ~/panel/custom.json\n  Executable  /opt/bin/panel\n") {
			t.Fatalf("overlapping config/data or home abbreviation changed:\n%s", got)
		}
	}
}

func TestCleanupTextReportsOnlyConfirmedResults(t *testing.T) {
	result := installation.CleanupResult{
		Removed:  []string{"/srv/panel/panel.db", "/srv/panel/imports"},
		Retained: []string{"/srv/panel/notes", "/srv/panel"},
	}
	wantTree := "  /\n  └── srv/panel [retained]\n      ├── imports [removed]\n      ├── notes [retained]\n      └── panel.db [removed]"
	for _, cleanupErr := range []error{nil, errors.New("fixture failure")} {
		heading := "Cleanup results"
		if cleanupErr != nil {
			heading = "Cleanup interrupted; confirmed results only"
		}
		want := heading + "\n\n  Files  2 removed, 0 remaining, 2 retained\n\n" + wantTree
		if got := cleanupText(result, cleanupErr, cliui.Style{}); got != want {
			t.Fatalf("cleanup output=%s", got)
		}
		if got := cleanupText(installation.CleanupResult{}, cleanupErr, cliui.Style{}); !strings.HasPrefix(got, heading) {
			t.Fatalf("empty cleanup output=%s", got)
		}
		colored := cleanupText(result, cleanupErr, cliui.Style{Color: true})
		if got := regexp.MustCompile(`\x1b\[[0-9;]*m`).ReplaceAllString(colored, ""); got != want {
			t.Fatalf("color changed cleanup results:\n%s", got)
		}
		if !strings.Contains(colored, "\x1b[32mremoved") || !strings.Contains(colored, "\x1b[36mretained") {
			t.Fatalf("missing cleanup result colors: %q", colored)
		}
		for _, format := range []cliui.Format{cliui.JSON, cliui.JSONL} {
			var stdout bytes.Buffer
			if err := cliui.Write(&stdout, format, result, colored); err != nil {
				t.Fatal(err)
			}
			var decoded installation.CleanupResult
			if err := json.Unmarshal(stdout.Bytes(), &decoded); err != nil || !slices.Equal(decoded.Removed, result.Removed) || !slices.Equal(decoded.Retained, result.Retained) {
				t.Fatalf("structured cleanup changed: %s", stdout.String())
			}

		}
	}
}

type cancelingUninstallService struct {
	fakeSystemdService
	cancel context.CancelFunc
}

func (service *cancelingUninstallService) Uninstall(ctx context.Context, request systemd.UninstallRequest) (systemd.UninstallResult, error) {
	result, err := service.fakeSystemdService.Uninstall(ctx, request)
	service.cancel()
	return result, err
}

func TestSystemPruneReportsPartialResultsAndPreservesFailure(t *testing.T) {
	for _, format := range []string{"text", "json", "jsonl"} {
		t.Run(format, func(t *testing.T) {
			path := commandSettingsFixture(t)
			servicePath := filepath.Join(filepath.Dir(path), "fixture.service")
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			service := &cancelingUninstallService{
				fakeSystemdService: fakeSystemdService{
					filesResult: systemd.FilesResult{StatePath: path, Files: []systemd.FileStatus{
						{Path: servicePath, State: "managed", Managed: true},
					}},
					statusResult:    systemd.Status{UnitFileStatePath: path},
					uninstallResult: systemd.UninstallResult{RemovedPaths: []string{servicePath}},
				},
				cancel: cancel,
			}
			var stdout, stderr bytes.Buffer
			root := NewRootCommand(Dependencies{CleanupHistoryPath: testHistoryPath(t), Stdout: &stdout, Stderr: &stderr, Systemd: service})
			root.SetArgs([]string{"system", "prune", "-c", path, "--yes", "-o", format})
			err := root.ExecuteContext(ctx)
			var cliErr *cliui.Error
			if !errors.Is(err, context.Canceled) || !errors.As(err, &cliErr) || cliErr.Code != "instance_cleanup_failed" || cliErr.Kind != cliui.ErrorConflict {
				t.Fatalf("cleanup failure changed: %v", err)
			}
			if format == "text" {
				if !strings.Contains(stdout.String(), "[removed]") || !strings.Contains(stdout.String(), "[remaining]") {
					t.Fatalf("partial output=%s", stdout.String())
				}
			} else {
				var result installation.CleanupResult
				if err := json.Unmarshal(stdout.Bytes(), &result); err != nil || !slices.Contains(result.Removed, servicePath) || len(result.Remaining) == 0 || len(result.Warnings) == 0 {
					t.Fatalf("partial structured output changed: %s", stdout.String())
				}
			}
			if _, err := os.Stat(path); err != nil {
				t.Fatalf("settings removed after interruption: %v", err)
			}
		})
	}
}

func TestSystemDFAndPruneOutputFormats(t *testing.T) {
	path := commandSettingsFixture(t)
	dataDir, err := settings.LoadDataDir(path)
	if err != nil {
		t.Fatal(err)
	}
	imports := filepath.Join(dataDir, "imports")
	if err := os.Mkdir(imports, 0o700); err != nil {
		t.Fatal(err)
	}
	outside := t.TempDir()
	if err := os.WriteFile(filepath.Join(outside, "not-enumerated.txt"), []byte("keep"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(imports, "link")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(outside, "absent-target"), filepath.Join(imports, "dangling-link")); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(imports, "empty"), 0o700); err != nil {
		t.Fatal(err)
	}
	service := &fakeSystemdService{filesResult: systemd.FilesResult{Files: []systemd.FileStatus{
		{Path: filepath.Join(t.TempDir(), "absent-service-parent", "panel.service"), State: "missing"},
	}}}
	report, err := inspectInstanceFiles(context.Background(), path, systemd.ScopeAuto, service, testHistoryPath(t))
	if err != nil {
		t.Fatal(err)
	}
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	for _, command := range []string{"df", "prune"} {
		for _, format := range []string{"", "text", "json", "jsonl"} {
			t.Run(command+"/"+format, func(t *testing.T) {
				args := []string{"system", command, "-c", path}
				if format != "" {
					args = append(args, "-o", format)
				}
				stdout, stderr, err := executeSystemCommand(t, service, args...)
				if err != nil || stderr != "" {
					t.Fatalf("command error=%v stderr=%s", err, stderr)
				}
				report.HistoryPath = testHistoryPath(t)
				report.Preview = command == "prune"
				if format == "json" || format == "jsonl" {
					var want bytes.Buffer
					encoder := json.NewEncoder(&want)
					encoder.SetEscapeHTML(false)
					if err := encoder.Encode(report); err != nil {
						t.Fatal(err)
					}
					if stdout != want.String() {
						t.Fatalf("structured report changed:\n%s\nwant:\n%s", stdout, want.String())
					}
					return
				}
				style := cliui.NewStyle(&bytes.Buffer{}, cliui.Text)
				summary := "  Config      " + style.Path(path) + "\n  Executable  " + style.Path(executable) + "\n"
				if !strings.Contains(stdout, summary) {
					t.Fatalf("selected paths missing:\n%s", stdout)
				}
				if strings.Contains(stdout, "Data:") {
					t.Fatalf("obsolete data summary is still displayed:\n%s", stdout)
				}
				for _, want := range []string{"├── ", "└── ", "link [link]", "dangling-link [link]", "empty/ [empty]"} {
					if !strings.Contains(stdout, want) {
						t.Fatalf("missing %q in output:\n%s", want, stdout)
					}
				}
				if strings.Contains(stdout, "not-enumerated.txt") || strings.Contains(stdout, "[removed]") {
					t.Fatalf("inspection expanded a link or claimed deletion:\n%s", stdout)
				}
				for _, absent := range []string{"[missing", "absent-service-parent", "runtime", "logs", "panel.db", "absent-target"} {
					if strings.Contains(stdout, absent) {
						t.Fatalf("absent path or branch %q leaked into the tree:\n%s", absent, stdout)
					}
				}
				if strings.Contains(stdout, "Pass --yes") != report.Preview {
					t.Fatalf("incorrect preview warning:\n%s", stdout)
				}
				if report.Preview && strings.Index(stdout, "\n\n  Pass --yes") < strings.LastIndex(stdout, "└──") {
					t.Fatalf("preview warning is not after the complete tree:\n%s", stdout)
				}
			})
		}
	}
}

func TestSystemDFAndPruneShowServiceFiles(t *testing.T) {
	path := commandSettingsFixture(t)
	for _, scope := range []systemd.Scope{systemd.ScopeUser, systemd.ScopeSystem} {
		files := []systemd.FileStatus{
			{Path: "/home/test/.config/systemd/user/sing-box-panel.service", State: "managed", Managed: true},
		}
		wantTree := "  ├── home/test/.config/systemd/user/sing-box-panel.service [service, user]"
		if scope == systemd.ScopeSystem {
			files = []systemd.FileStatus{
				{Path: "/etc/systemd/system/sing-box-panel.service", State: "managed", Managed: true},
				{Path: "/etc/sysusers.d/sing-box-panel.conf", State: "managed", Managed: true},
				{Path: "/etc/tmpfiles.d/sing-box-panel.conf", State: "managed", Managed: true},
			}
			wantTree = "  ├── etc/\n  │   ├── systemd/system/sing-box-panel.service [service, system]\n  │   ├── sysusers.d/sing-box-panel.conf [service, system]\n  │   └── tmpfiles.d/sing-box-panel.conf [service, system]"
		}
		files = append(files, systemd.FileStatus{Path: "/absent-services/missing.service", State: "missing"})
		for _, matches := range []bool{true, false} {
			selectedPath := path
			want := wantTree
			if !matches {
				selectedPath = filepath.Join(t.TempDir(), "other.json")
				want = strings.ReplaceAll(want, "]", ", outside scope]")
			}
			service := &fakeSystemdService{filesResult: systemd.FilesResult{Scope: scope, StatePath: selectedPath, Files: files}}
			for _, command := range []string{"df", "prune"} {
				stdout, stderr, err := executeSystemCommand(t, service, "system", command, "-c", path, "--scope", string(scope))
				if err != nil || stderr != "" {
					t.Fatalf("command error=%v stderr=%s", err, stderr)
				}
				if strings.Count(stdout, want) != 1 || strings.Contains(stdout, "absent-services") {
					t.Fatalf("scope=%s matches=%v service tree missing, duplicated or fabricated:\n%s", scope, matches, stdout)
				}
			}
		}
	}
}

func TestSystemDFUsesDefaultAndExplicitConfigPaths(t *testing.T) {
	defaultPath := commandSettingsFixture(t)
	if os.Geteuid() != 0 {
		configHome := t.TempDir()
		t.Setenv("XDG_CONFIG_HOME", configHome)
		defaultPath = settings.DefaultPath()
		if err := os.MkdirAll(filepath.Dir(defaultPath), 0o700); err != nil {
			t.Fatal(err)
		}
		data, err := os.ReadFile(commandSettingsFixture(t))
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(defaultPath, data, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	explicitPath := commandSettingsFixture(t)
	tests := []struct {
		name string
		args []string
		path string
	}{
		{"short", []string{"system", "df", "-c", explicitPath}, explicitPath},
		{"long", []string{"--config", explicitPath, "system", "df"}, explicitPath},
		{"last flag", []string{"-c", defaultPath, "system", "df", "--config", explicitPath}, explicitPath},
	}
	if os.Geteuid() != 0 {
		tests = append(tests, struct {
			name string
			args []string
			path string
		}{"default", []string{"system", "df"}, defaultPath})
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			root := NewRootCommand(Dependencies{CleanupHistoryPath: testHistoryPath(t), Stdout: &stdout, Stderr: &stderr, Systemd: &fakeSystemdService{}})
			root.SetArgs(append(test.args, "--output", "json"))
			if err := root.ExecuteContext(context.Background()); err != nil {
				t.Fatal(err)
			}
			var result instanceFilesReport
			if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
				t.Fatal(err)
			}
			if result.SettingsPath != test.path {
				t.Fatalf("loaded %q want %q", result.SettingsPath, test.path)
			}
			value, err := settings.Load(test.path)
			if err != nil || result.DataDir != value.DataDir {
				t.Fatalf("default/explicit file not loaded: %+v %v", result, err)
			}
		})
	}
}

func TestSystemDFAndPreviewShowEmptyData(t *testing.T) {
	for _, exists := range []bool{true, false} {
		path := commandSettingsFixture(t)
		dataDir, err := settings.LoadDataDir(path)
		if err != nil {
			t.Fatal(err)
		}
		if !exists {
			if err := os.Remove(dataDir); err != nil {
				t.Fatal(err)
			}
		}
		for _, command := range []string{"df", "prune"} {
			stdout, _, err := executeSystemCommand(t, &fakeSystemdService{err: systemd.ErrUnsupportedOS}, "-c", path, "system", command)
			if err != nil {
				t.Fatal(err)
			}
			if strings.Contains(stdout, "data/ [empty, data]") != exists || strings.Contains(stdout, "panel.db") || strings.Contains(stdout, "[missing") {
				t.Fatalf("empty data not clearly represented:\n%s", stdout)
			}
			if strings.Contains(stdout, "Data:") || strings.Contains(stdout, "No data directory.") {
				t.Fatalf("obsolete data summary is still displayed:\n%s", stdout)
			}
			if !strings.Contains(stdout, "  Service     unsupported on this platform") || strings.Contains(stdout, "[service") || strings.Contains(stdout, "not installed") {
				t.Fatalf("unsupported systemd state was misrepresented:\n%s", stdout)
			}
			if command == "prune" {
				warning := strings.Index(stdout, "\n\n  Pass --yes")
				if warning < strings.LastIndex(stdout, "└──") {
					t.Fatalf("cleanup warning is not after the tree:\n%s", stdout)
				}
			}
		}
	}
}

func TestSystemDFAndPreviewAfterPruneWithoutSettings(t *testing.T) {
	for _, selection := range []string{"default", "explicit"} {
		t.Run(selection, func(t *testing.T) {
			if selection == "default" && os.Geteuid() == 0 {
				t.Skip("root settings path cannot be redirected to a fixture")
			}
			path := commandSettingsFixture(t)
			dataDir, err := settings.LoadDataDir(path)
			if err != nil {
				t.Fatal(err)
			}
			var configArgs []string
			if selection == "default" {
				t.Setenv("XDG_CONFIG_HOME", t.TempDir())
				selected := settings.DefaultPath()
				if err := os.MkdirAll(filepath.Dir(selected), 0o700); err != nil {
					t.Fatal(err)
				}
				if err := os.Rename(path, selected); err != nil {
					t.Fatal(err)
				}
				path = selected
			} else {
				configArgs = []string{"-c", path}
			}
			service := &fakeSystemdService{err: systemd.ErrUnsupportedOS}
			args := append([]string{"system", "prune", "--yes"}, configArgs...)
			if _, _, err := executeSystemCommand(t, service, args...); err != nil {
				t.Fatal(err)
			}
			for _, command := range []string{"df", "prune"} {
				for _, format := range []string{"text", "json", "jsonl"} {
					args := append([]string{"system", command, "-o", format}, configArgs...)
					stdout, stderr, err := executeSystemCommand(t, service, args...)
					if err != nil || stderr != "" {
						t.Fatalf("%s after prune: err=%v stderr=%s", command, err, stderr)
					}
					if format == "text" {
						style := cliui.NewStyle(&bytes.Buffer{}, cliui.Text)
						for _, want := range []string{"  Config      " + style.Path(path) + " (missing)\n", "[executable, retained, "} {
							if !strings.Contains(stdout, want) {
								t.Fatalf("missing %q from output:\n%s", want, stdout)
							}
						}
						if strings.Contains(stdout, "Settings:") || strings.Count(stdout, "  Config  ") != 1 {
							t.Fatalf("configuration summary duplicated:\n%s", stdout)
						}
						if strings.Contains(stdout, "Data:") || strings.Contains(stdout, "unknown (settings missing)") || strings.Contains(stdout, "No data directory.") {
							t.Fatalf("obsolete data summary is still displayed:\n%s", stdout)
						}
						if !strings.Contains(stdout, "No removable resources remain") || strings.Contains(stdout, "Pass --yes") {
							t.Fatalf("missing settings led to incorrect cleanup guidance:\n%s", stdout)
						}
					} else {
						var report instanceFilesReport
						if err := json.Unmarshal([]byte(stdout), &report); err != nil {
							t.Fatal(err)
						}
						if report.SettingsPath != path || report.DataDir != "" || report.DatabaseIdentity != "unknown" || report.Preview != (command == "prune") {
							t.Fatalf("incorrect post-cleanup report: %+v", report)
						}
					}
				}
			}
			for _, removed := range []string{path, dataDir} {
				if _, err := os.Lstat(removed); !os.IsNotExist(err) {
					t.Fatalf("inspection recreated %s: %v", removed, err)
				}
			}
		})
	}
}

func TestSystemDFWithoutSettingsStillInspectsServices(t *testing.T) {
	path := filepath.Join(t.TempDir(), "setting.json")
	servicePath := filepath.Join(t.TempDir(), "panel.service")
	service := &fakeSystemdService{filesResult: systemd.FilesResult{
		Scope: systemd.ScopeUser, StatePath: path,
		Files: []systemd.FileStatus{{Path: servicePath, State: "managed", Managed: true}},
	}}
	stdout, stderr, err := executeSystemCommand(t, service, "system", "df", "-c", path, "-o", "json")
	if err != nil || stderr != "" {
		t.Fatalf("files error=%v stderr=%s", err, stderr)
	}
	var report instanceFilesReport
	if err := json.Unmarshal([]byte(stdout), &report); err != nil {
		t.Fatal(err)
	}
	if report.ServiceState != "inspected" || len(report.Service.Files) != 1 || report.Service.Files[0].Path != servicePath || report.ServiceDataKnown {
		t.Fatalf("service inspection lost without settings: %+v", report)
	}
	stdout, _, err = executeSystemCommand(t, service, "system", "prune", "-c", path)
	if err != nil || !strings.Contains(stdout, "Pass --yes") {
		t.Fatalf("preview suggested cleanup without settings: err=%v output=%s", err, stdout)
	}
	style := cliui.NewStyle(&bytes.Buffer{}, cliui.Text)
	if !strings.Contains(stdout, "  Config      "+style.Path(path)+" (missing)\n") || strings.Contains(stdout, "Settings:") {
		t.Fatalf("preview did not consolidate the configuration summary:\n%s", stdout)
	}
	_, _, err = executeSystemCommand(t, service, "system", "prune", "-c", path, "--yes")
	if err != nil {
		t.Fatalf("service-only cleanup: %v", err)
	}
	if service.uninstallRequest.Scope != systemd.ScopeUser {
		t.Fatal("matching orphan service was not removed")
	}
}

func TestSystemPruneRemovesUnknownDataAndStaleControlFile(t *testing.T) {
	for _, format := range []string{"text", "json", "jsonl"} {
		t.Run(format, func(t *testing.T) {
			path := commandSettingsFixture(t)
			dataDir, err := settings.LoadDataDir(path)
			if err != nil {
				t.Fatal(err)
			}
			for _, name := range []string{"panel.db", "unknown.txt", procctl.SocketName} {
				if err := os.WriteFile(filepath.Join(dataDir, name), []byte("unrecognized contents"), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			service := &fakeSystemdService{err: systemd.ErrUnsupportedOS}
			lease, err := procctl.AcquireLease(dataDir)
			if err != nil {
				t.Fatal(err)
			}
			_, _, busyErr := executeSystemCommand(t, service, "-c", path, "system", "prune", "--yes")
			if err := lease.Close(); err != nil {
				t.Fatal(err)
			}
			if busyErr == nil {
				t.Fatal("stale control file bypassed a live runtime owner")
			}
			if _, err := os.Stat(path); err != nil {
				t.Fatal("failed cleanup removed settings")
			}
			stdout, stderr, err := executeSystemCommand(t, service, "-c", path, "system", "prune", "--yes", "-o", format)
			if err != nil || stderr != "" {
				t.Fatalf("full cleanup error=%v stderr=%s", err, stderr)
			}
			if format == "text" {
				if !strings.Contains(stdout, "unknown.txt [removed]") || !strings.Contains(stdout, procctl.SocketName+" [removed]") {
					t.Fatalf("missing completed cleanup results:\n%s", stdout)
				}
			} else {
				var result installation.CleanupResult
				if err := json.Unmarshal([]byte(stdout), &result); err != nil {
					t.Fatal(err)
				}
				if !slices.Contains(result.Removed, dataDir) || !slices.Contains(result.Removed, filepath.Join(dataDir, "unknown.txt")) {
					t.Fatalf("incomplete structured results: %+v", result)
				}
			}
			if _, err := os.Lstat(dataDir); !os.IsNotExist(err) {
				t.Fatalf("unknown data survived cleanup: %v", err)
			}
		})
	}
}

func TestSystemPruneDefaultsToPreviewAndRejectsEmptyConfig(t *testing.T) {
	path := commandSettingsFixture(t)
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	root := NewRootCommand(Dependencies{CleanupHistoryPath: testHistoryPath(t), Stdout: &stdout, Stderr: &stderr, Systemd: &fakeSystemdService{}})
	root.SetArgs([]string{"system", "prune", "--config", path})
	if err := root.ExecuteContext(context.Background()); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stdout.String(), "Cleanup preview") {
		t.Fatalf("output=%s", stdout.String())
	}
	after, err := os.ReadFile(path)
	if err != nil || string(before) != string(after) {
		t.Fatal("preview removed or changed settings")
	}
	root = NewRootCommand(Dependencies{CleanupHistoryPath: testHistoryPath(t), Stdout: &stdout, Stderr: &stderr, Systemd: &fakeSystemdService{}})
	root.SetArgs([]string{"system", "df", "--config="})
	if err := root.ExecuteContext(context.Background()); err == nil {
		t.Fatal("explicit empty config silently used default")
	}
}

func TestCleanupDistinguishesMatchingSharedAndUnrelatedServices(t *testing.T) {
	path := commandSettingsFixture(t)
	value, err := settings.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, kind := range []string{"matching", "shared", "nested", "parent", "unrelated", "unknown"} {
		t.Run(kind, func(t *testing.T) {
			service := &fakeSystemdService{
				filesResult:  systemd.FilesResult{Scope: systemd.ScopeUser, StatePath: path, Files: []systemd.FileStatus{{Path: "fixture.service", State: "managed", Managed: true}}},
				statusResult: systemd.Status{Scope: systemd.ScopeUser, UnitFileStatePath: path},
			}
			report, err := inspectInstanceFiles(context.Background(), path, systemd.ScopeUser, service, testHistoryPath(t))
			if err != nil {
				t.Fatal(err)
			}
			if kind != "matching" {
				report.ServiceMatches = false
				report.Service.StatePath = "/different/setting.json"
			}
			if kind == "unrelated" {
				report.ServiceDataDir = t.TempDir()
			}
			if kind == "nested" {
				report.ServiceDataDir = filepath.Join(value.DataDir, "other")
			}
			if kind == "parent" {
				report.ServiceDataDir = filepath.Dir(value.DataDir)
			}
			if kind == "unknown" {
				report.ServiceDataKnown = false
			}
			_, err = stopInstanceForCleanup(context.Background(), report, service)
			if (kind == "shared" || kind == "nested" || kind == "parent" || kind == "unknown") && err == nil {
				t.Fatal("ambiguous shared service accepted")
			}
			if (kind == "matching" || kind == "unrelated") && err != nil {
				t.Fatal(err)
			}
			if kind == "matching" && service.uninstallRequest.Scope != systemd.ScopeUser {
				t.Fatal("matching service not uninstalled")
			}
			if kind != "matching" && service.uninstallRequest.Scope != "" {
				t.Fatal("unrelated service was changed")
			}
		})
	}
}

func TestCleanupProtectsOtherServiceSettingsAndAliases(t *testing.T) {
	for _, kind := range []string{"inside", "parent alias outside", "file alias inside", "parent alias inside", "data alias inside", "unrelated"} {
		t.Run(kind, func(t *testing.T) {
			path := commandSettingsFixture(t)
			dataDir, err := settings.LoadDataDir(path)
			if err != nil {
				t.Fatal(err)
			}
			outside := t.TempDir()
			serviceSettings := filepath.Join(outside, "setting.json")
			serviceData := filepath.Join(outside, "data")
			if err := os.Mkdir(serviceData, 0o700); err != nil {
				t.Fatal(err)
			}
			switch kind {
			case "inside", "file alias inside", "parent alias inside":
				serviceSettings = filepath.Join(dataDir, "other.json")
			case "parent alias outside":
				alias := filepath.Join(dataDir, "external")
				if err := os.Symlink(outside, alias); err != nil {
					t.Fatal(err)
				}
				serviceSettings = filepath.Join(alias, "setting.json")
			case "data alias inside":
				alias := filepath.Join(dataDir, "external-data")
				if err := os.Symlink(serviceData, alias); err != nil {
					t.Fatal(err)
				}
				serviceData = alias
			}
			encoded, err := json.Marshal(map[string]string{"data_dir": serviceData})
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(serviceSettings, encoded, 0o600); err != nil {
				t.Fatal(err)
			}
			if kind == "file alias inside" {
				alias := filepath.Join(outside, "linked.json")
				if err := os.Symlink(serviceSettings, alias); err != nil {
					t.Fatal(err)
				}
				serviceSettings = alias
			}
			if kind == "parent alias inside" {
				alias := filepath.Join(outside, "linked")
				if err := os.Symlink(dataDir, alias); err != nil {
					t.Fatal(err)
				}
				serviceSettings = filepath.Join(alias, "other.json")
			}
			service := &fakeSystemdService{filesResult: systemd.FilesResult{
				Scope: systemd.ScopeUser, StatePath: serviceSettings,
				Files: []systemd.FileStatus{{Path: filepath.Join(outside, "panel.service"), State: "managed", Managed: true}},
			}}
			_, _, err = executeSystemCommand(t, service, "system", "prune", "-c", path, "--yes")
			if (err != nil) != (kind != "unrelated") {
				t.Fatalf("incorrect service path ownership decision: %v", err)
			}
			if kind != "unrelated" {
				if _, err := os.Stat(path); err != nil {
					t.Fatalf("removed selected settings before rejecting conflict: %v", err)
				}
			}
			if contents, err := os.ReadFile(serviceSettings); err != nil || string(contents) != string(encoded) {
				t.Fatalf("changed unrelated service settings: %v", err)
			}
			if service.uninstallRequest.Scope != "" || service.statusScope != "" || service.controlScope != "" {
				t.Fatal("acted on unrelated service")
			}
		})
	}
}

func TestSystemPruneRemovesAllDataWithInvalidRuntimeSettings(t *testing.T) {
	path := commandSettingsFixture(t)
	createPanelDatabaseFixture(t, path)
	dataDir, err := settings.LoadDataDir(path)
	if err != nil {
		t.Fatal(err)
	}
	unrelated := filepath.Join(dataDir, "keep.txt")
	if err := os.WriteFile(unrelated, []byte("retain"), 0o600); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	invalid := strings.Replace(string(before), `"sample_retention_days":90`, `"sample_retention_days":0`, 1)
	if err := os.WriteFile(path, []byte(invalid), 0o600); err != nil {
		t.Fatal(err)
	}
	// The injected service cannot invoke the host's systemd manager.
	service := &fakeSystemdService{err: systemd.ErrUnsupportedOS}
	stdout, stderr, err := executeSystemCommand(t, service, "--config", path, "system", "prune", "--yes")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"Cleanup results", "panel.db [removed]", "setting.json [removed]", "data [removed]", "keep.txt [removed]"} {
		if !strings.Contains(stdout, want) {
			t.Fatalf("missing %q in cleanup result:\n%s", want, stdout)
		}
	}
	if stderr != "" || strings.Contains(stdout, "prune:") || strings.Contains(stdout, "Cleanup preview") {
		t.Fatalf("cleanup result mixed with preview: stdout=%s stderr=%s", stdout, stderr)
	}
	for _, removed := range []string{path, filepath.Join(dataDir, "panel.db")} {
		if _, err := os.Stat(removed); !os.IsNotExist(err) {
			t.Fatalf("managed file remains: %s: %v", removed, err)
		}
	}
	if _, err := os.Lstat(unrelated); !os.IsNotExist(err) {
		t.Fatal("prune retained a file inside the selected data directory")
	}
}
