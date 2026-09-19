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

	"github.com/rehuony/sing-box-panel/internal/installation"
	"github.com/rehuony/sing-box-panel/internal/panelprocess"
	"github.com/rehuony/sing-box-panel/internal/settings"
	panelSystemd "github.com/rehuony/sing-box-panel/internal/systemd"
)

func TestFileTreeText(t *testing.T) {
	tests := []struct {
		name    string
		entries []fileTreeEntry
		want    string
	}{
		{
			name: "shared parents and compressed paths",
			entries: []fileTreeEntry{
				{path: "/srv/panel/runtime/current.json", label: "file"},
				{path: "/srv/panel/imports/z.zip", label: "file"},
				{path: "/srv/panel/imports/nested/a.zip", label: "file"},
				{path: "/srv/panel/imports", label: "dir", directory: true},
				{path: "/srv/panel/panel.db", label: "file"},
			},
			want: "/srv/panel/\n" +
				"├── imports/ [dir]\n" +
				"│   ├── nested/a.zip [file]\n" +
				"│   └── z.zip [file]\n" +
				"├── panel.db [file]\n" +
				"└── runtime/current.json [file]",
		},
		{
			name: "multiple roots",
			entries: []fileTreeEntry{
				{path: "/var/lib/panel/panel.db", label: "file"},
				{path: "/opt/panel/bin/panel", label: "file"},
				{path: "/etc/panel/setting.json", label: "file"},
			},
			want: "/etc/panel/setting.json [file]\n\n/opt/panel/bin/panel [file]\n\n/var/lib/panel/panel.db [file]",
		},
		{
			name: "explicit root and overlapping reports",
			entries: []fileTreeEntry{
				{path: "/", label: "dir"},
				{path: "/a", label: "service: unmanaged"},
				{path: "/a", label: "file"},
			},
			want: "/ [dir]\n└── a [file] [service: unmanaged]",
		},
		{
			name:    "relative fixture paths",
			entries: []fileTreeEntry{{path: "fixture.service", label: "service: managed"}},
			want:    "fixture.service [service: managed]",
		},
		{name: "empty"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := fileTreeText(test.entries, fileTreeStyle{}); got != test.want {
				t.Fatalf("tree:\n%s\nwant:\n%s", got, test.want)
			}
			reversed := slices.Clone(test.entries)
			slices.Reverse(reversed)
			if got := fileTreeText(reversed, fileTreeStyle{}); got != test.want {
				t.Fatalf("tree depends on input order:\n%s", got)
			}
		})
	}
}

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
				{Path: "/srv/panel/empty", State: "directory", Cleanup: "remove"},
				{Path: "/srv/panel/panel.db", State: "file", Cleanup: "remove"},
				{Path: "/srv/panel/imports", State: "directory", Cleanup: "remove"},
				{Path: "/srv/panel/imports/link", State: "symlink", Cleanup: "remove_link"},
				{Path: "/srv/panel/runtime", State: "missing", Cleanup: "remove"},
				{Path: "/srv/panel/absent-branch/cache/item", State: "missing", Cleanup: "remove"},
				{Path: "/srv/panel/notes", Role: "instance data", State: "file", Cleanup: "remove"},
			},
		},
		Service: panelSystemd.FilesResult{Scope: panelSystemd.ScopeSystem, Files: []panelSystemd.FileStatus{
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
Config:     /srv/config/custom.json
Executable: /opt/bin/panel

/etc/systemd/
├── other.service [service, system, outside scope]
└── panel.service [service, system]

/opt/bin/panel [executable]

/srv/
├── config/custom.json [settings]
└── panel/ [data]
    ├── empty/ [empty]
    ├── imports/
    │   └── link [link]
    ├── notes
    └── panel.db`
	if got := instanceFilesText(report, fileTreeStyle{}); got != want {
		t.Fatalf("output:\n%s\nwant:\n%s", got, want)
	}
	colored := instanceFilesText(report, fileTreeStyle{color: true})
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
	if got := instanceFilesText(report, fileTreeStyle{}); got != want {
		t.Fatalf("report order changed the tree:\n%s", got)
	}
	report.Preview = true
	warning := "Pass --yes to stop this instance and permanently delete its settings and all data."
	wantPreview := strings.Replace(want, "Instance files", "Cleanup preview — no changes", 1) + "\n\n" + warning
	if got := instanceFilesText(report, fileTreeStyle{}); got != wantPreview {
		t.Fatalf("preview warning is not after the complete tree:\n%s", got)
	}
	colored = instanceFilesText(report, fileTreeStyle{color: true})
	if ansi.ReplaceAllString(colored, "") != wantPreview || strings.Count(colored, "\x1b[33m") != 1 {
		t.Fatalf("preview warning color changed content or position: %q", colored)
	}
	report.Preview = false
	report.ServiceMatches = false
	if got := instanceFilesText(report, fileTreeStyle{}); !strings.Contains(got, "panel.service [service, system, outside scope]") {
		t.Fatalf("unrelated service would be uninstalled:\n%s", got)
	}
	report.DatabaseIdentity = "unknown"
	if got := instanceFilesText(report, fileTreeStyle{}); strings.Contains(got, "blocked") || strings.Contains(got, "Policies") {
		t.Fatalf("obsolete identity restriction or footer:\n%s", got)
	}
	report.SettingsPath = "/srv/panel/custom.json"
	for _, preview := range []bool{false, true} {
		report.Preview = preview
		if got := instanceFilesText(report, fileTreeStyle{home: "/srv"}); !strings.Contains(got, "Config:     ~/panel/custom.json\nExecutable: /opt/bin/panel\n") {
			t.Fatalf("overlapping config/data or home abbreviation changed:\n%s", got)
		}
	}
}

func TestCleanupTextReportsOnlyConfirmedResults(t *testing.T) {
	result := installation.CleanupResult{
		Removed:  []string{"/srv/panel/panel.db", "/srv/panel/imports"},
		Retained: []string{"/srv/panel/notes", "/srv/panel"},
	}
	wantTree := "/srv/panel [retained]\n├── imports [removed]\n├── notes [retained]\n└── panel.db [removed]"
	for _, cleanupErr := range []error{nil, errors.New("fixture failure")} {
		heading := "Cleanup results"
		if cleanupErr != nil {
			heading = "Cleanup interrupted; confirmed results only"
		}
		if got := cleanupText(result, cleanupErr, fileTreeStyle{}); got != heading+"\n\n"+wantTree {
			t.Fatalf("cleanup output=%s", got)
		}
		if got := cleanupText(installation.CleanupResult{}, cleanupErr, fileTreeStyle{}); got != heading+": no paths reported." {
			t.Fatalf("empty cleanup output=%s", got)
		}
		colored := cleanupText(result, cleanupErr, fileTreeStyle{color: true})
		if got := regexp.MustCompile(`\x1b\[[0-9;]*m`).ReplaceAllString(colored, ""); got != heading+"\n\n"+wantTree {
			t.Fatalf("color changed cleanup results:\n%s", got)
		}
		if !strings.Contains(colored, "\x1b[32mremoved") || !strings.Contains(colored, "\x1b[36mretained") {
			t.Fatalf("missing cleanup result colors: %q", colored)
		}
		for _, format := range []outputFormat{outputJSON, outputJSONL} {
			var stdout bytes.Buffer
			if err := writeResult(&stdout, format, result, colored); err != nil {
				t.Fatal(err)
			}
			want := "{\"removed\":[\"/srv/panel/panel.db\",\"/srv/panel/imports\"],\"retained\":[\"/srv/panel/notes\",\"/srv/panel\"]}\n"
			if stdout.String() != want {
				t.Fatalf("structured cleanup changed: %s", stdout.String())
			}
		}
	}
}

func TestFileTreeHomeAbbreviation(t *testing.T) {
	for _, test := range []struct{ home, path, want string }{
		{"/home/alice", "/home/alice", "~"},
		{"/home/alice/", "/home/alice/data", "~/data"},
		{"/home/alice", "/home/alice-other/data", "/home/alice-other/data"},
		{"/home/alice", "/home/alice2/data", "/home/alice2/data"},
		{"/home/alice", "/home", "/home"},
		{"/", "/etc/panel", "/etc/panel"},
		{"", "/etc/panel", "/etc/panel"},
		{"relative", "relative/panel", "relative/panel"},
	} {
		if got := (fileTreeStyle{home: test.home}).path(test.path); got != test.want {
			t.Fatalf("home=%q path=%q got=%q want=%q", test.home, test.path, got, test.want)
		}
	}
	entries := []fileTreeEntry{
		{path: "/home/alice/config/setting.json", label: "remove"},
		{path: "/home/alice/data/panel.db", label: "removed"},
	}
	want := "~/\n├── config/setting.json [remove]\n└── data/panel.db [removed]"
	if got := fileTreeText(entries, fileTreeStyle{home: "/home/alice"}); got != want {
		t.Fatalf("compressed root was not abbreviated:\n%s", got)
	}
}

type fileTreeFDWriter struct {
	bytes.Buffer
	fdCalls int
}

func (writer *fileTreeFDWriter) Fd() uintptr {
	writer.fdCalls++
	return ^uintptr(0) // Invalid descriptor: capability checks must fail closed.
}

func TestFileTreeStyleDisablesColor(t *testing.T) {
	for _, test := range []struct {
		name, term, noColor string
		format              outputFormat
		checkTerminal       bool
	}{
		{"terminal check", "xterm-256color", "", outputText, true},
		{"no color", "xterm-256color", "1", outputText, false},
		{"no color any value", "xterm", "0", outputText, false},
		{"dumb", "dumb", "", outputText, false},
		{"unknown terminal", "", "", outputText, false},
		{"json", "xterm", "", outputJSON, false},
		{"jsonl", "xterm", "", outputJSONL, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Setenv("TERM", test.term)
			t.Setenv("NO_COLOR", test.noColor)
			writer := &fileTreeFDWriter{}
			style := newFileTreeStyle(writer, test.format)
			if style.color || (writer.fdCalls > 0) != test.checkTerminal {
				t.Fatalf("color=%v terminal checks=%d", style.color, writer.fdCalls)
			}
			if got := style.label("removed"); got != "[removed]" {
				t.Fatalf("disabled color produced ANSI: %q", got)
			}
		})
	}
	t.Setenv("TERM", "xterm-256color")
	t.Setenv("NO_COLOR", "")
	if style := newFileTreeStyle(&bytes.Buffer{}, outputText); style.color {
		t.Fatal("buffer output enabled color")
	}
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	defer writer.Close()
	if style := newFileTreeStyle(writer, outputText); style.color {
		t.Fatal("pipe output enabled color")
	}
}

type cancelingUninstallService struct {
	fakeSystemdService
	cancel context.CancelFunc
}

func (service *cancelingUninstallService) Uninstall(ctx context.Context, request panelSystemd.UninstallRequest) (panelSystemd.UninstallResult, error) {
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
					filesResult: panelSystemd.FilesResult{SettingsPath: path, Files: []panelSystemd.FileStatus{
						{Path: servicePath, State: "managed", Managed: true},
					}},
					statusResult:    panelSystemd.Status{UnitFileSettingsPath: path},
					uninstallResult: panelSystemd.UninstallResult{RemovedPaths: []string{servicePath}},
				},
				cancel: cancel,
			}
			var stdout, stderr bytes.Buffer
			root := NewRootCommand(Dependencies{Stdout: &stdout, Stderr: &stderr, Systemd: service})
			root.SetArgs([]string{"system", "prune", "-c", path, "--yes", "-o", format})
			err := root.ExecuteContext(ctx)
			var cliErr *Error
			if !errors.Is(err, context.Canceled) || !errors.As(err, &cliErr) || cliErr.Code != "instance_cleanup_failed" || cliErr.Kind != ErrorConflict {
				t.Fatalf("cleanup failure changed: %v", err)
			}
			if format == "text" {
				want := "Cleanup interrupted; confirmed results only\n\n" + servicePath + " [removed]\n"
				if stdout.String() != want {
					t.Fatalf("partial output=%s want=%s", stdout.String(), want)
				}
			} else {
				want, err := json.Marshal(installation.CleanupResult{Removed: []string{servicePath}, Retained: []string{}})
				if err != nil || stdout.String() != string(want)+"\n" {
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
	service := &fakeSystemdService{filesResult: panelSystemd.FilesResult{Files: []panelSystemd.FileStatus{
		{Path: filepath.Join(t.TempDir(), "absent-service-parent", "panel.service"), State: "missing"},
	}}}
	report, err := inspectInstanceFiles(context.Background(), path, panelSystemd.ScopeAuto, service)
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
				style := newFileTreeStyle(&bytes.Buffer{}, outputText)
				summary := "Config:     " + style.path(path) + "\nExecutable: " + style.path(executable) + "\n"
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
				if report.Preview && strings.Index(stdout, "\n\nPass --yes") < strings.LastIndex(stdout, "└──") {
					t.Fatalf("preview warning is not after the complete tree:\n%s", stdout)
				}
			})
		}
	}
}

func TestSystemDFAndPruneShowServiceFiles(t *testing.T) {
	path := commandSettingsFixture(t)
	for _, scope := range []panelSystemd.Scope{panelSystemd.ScopeUser, panelSystemd.ScopeSystem} {
		files := []panelSystemd.FileStatus{
			{Path: "/home/test/.config/systemd/user/sing-box-panel.service", State: "managed", Managed: true},
		}
		wantTree := "/home/test/.config/systemd/user/sing-box-panel.service [service, user]"
		if scope == panelSystemd.ScopeSystem {
			files = []panelSystemd.FileStatus{
				{Path: "/etc/systemd/system/sing-box-panel.service", State: "managed", Managed: true},
				{Path: "/etc/sysusers.d/sing-box-panel.conf", State: "managed", Managed: true},
				{Path: "/etc/tmpfiles.d/sing-box-panel.conf", State: "managed", Managed: true},
			}
			wantTree = "/etc/\n├── systemd/system/sing-box-panel.service [service, system]\n├── sysusers.d/sing-box-panel.conf [service, system]\n└── tmpfiles.d/sing-box-panel.conf [service, system]"
		}
		files = append(files, panelSystemd.FileStatus{Path: "/absent-services/missing.service", State: "missing"})
		for _, matches := range []bool{true, false} {
			selectedPath := path
			want := wantTree
			if !matches {
				selectedPath = filepath.Join(t.TempDir(), "other.json")
				want = strings.ReplaceAll(want, "]", ", outside scope]")
			}
			service := &fakeSystemdService{filesResult: panelSystemd.FilesResult{Scope: scope, SettingsPath: selectedPath, Files: files}}
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
			root := NewRootCommand(Dependencies{Stdout: &stdout, Stderr: &stderr, Systemd: &fakeSystemdService{}})
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
			stdout, _, err := executeSystemCommand(t, &fakeSystemdService{err: panelSystemd.ErrUnsupportedOS}, "-c", path, "system", command)
			if err != nil {
				t.Fatal(err)
			}
			if strings.Contains(stdout, "data/ [empty, data]") != exists || strings.Contains(stdout, "panel.db") || strings.Contains(stdout, "[missing") {
				t.Fatalf("empty data not clearly represented:\n%s", stdout)
			}
			if strings.Contains(stdout, "Data:") || strings.Contains(stdout, "No data directory.") {
				t.Fatalf("obsolete data summary is still displayed:\n%s", stdout)
			}
			if !strings.Contains(stdout, "Systemd:    unsupported on this platform") || strings.Contains(stdout, "[service") || strings.Contains(stdout, "not installed") {
				t.Fatalf("unsupported systemd state was misrepresented:\n%s", stdout)
			}
			if command == "prune" {
				warning := strings.Index(stdout, "\n\nPass --yes")
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
			service := &fakeSystemdService{err: panelSystemd.ErrUnsupportedOS}
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
						style := newFileTreeStyle(&bytes.Buffer{}, outputText)
						for _, want := range []string{"Config:     " + style.path(path) + " (missing)\n", "[executable]"} {
							if !strings.Contains(stdout, want) {
								t.Fatalf("missing %q from output:\n%s", want, stdout)
							}
						}
						if strings.Contains(stdout, "Settings:") || strings.Count(stdout, "Config:") != 1 {
							t.Fatalf("configuration summary duplicated:\n%s", stdout)
						}
						if strings.Contains(stdout, "Data:") || strings.Contains(stdout, "unknown (settings missing)") || strings.Contains(stdout, "No data directory.") {
							t.Fatalf("obsolete data summary is still displayed:\n%s", stdout)
						}
						if strings.Contains(stdout, "Cleanup unavailable:") != (command == "prune") || strings.Contains(stdout, "Pass --yes") {
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
	service := &fakeSystemdService{filesResult: panelSystemd.FilesResult{
		Scope: panelSystemd.ScopeUser, SettingsPath: path,
		Files: []panelSystemd.FileStatus{{Path: servicePath, State: "managed", Managed: true}},
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
	if err != nil || !strings.Contains(stdout, "Cleanup unavailable:") || strings.Contains(stdout, "Pass --yes") {
		t.Fatalf("preview suggested cleanup without settings: err=%v output=%s", err, stdout)
	}
	style := newFileTreeStyle(&bytes.Buffer{}, outputText)
	if !strings.Contains(stdout, "Config:     "+style.path(path)+" (missing)\n") || strings.Contains(stdout, "Settings:") {
		t.Fatalf("preview did not consolidate the configuration summary:\n%s", stdout)
	}
	_, _, err = executeSystemCommand(t, service, "system", "prune", "-c", path, "--yes")
	var cliErr *Error
	if !errors.As(err, &cliErr) || cliErr.Code != "instance_files_unavailable" || ExitCode(err) != 3 || !strings.Contains(err.Error(), "settings file is missing") {
		t.Fatalf("unexpected cleanup error: %v", err)
	}
	if service.uninstallRequest.Scope != "" {
		t.Fatal("missing settings allowed service removal")
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
			for _, name := range []string{"panel.db", "unknown.txt", "panel-control.sock"} {
				if err := os.WriteFile(filepath.Join(dataDir, name), []byte("unrecognized contents"), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			service := &fakeSystemdService{err: panelSystemd.ErrUnsupportedOS}
			lease, err := panelprocess.AcquireLease(dataDir)
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
				if !strings.Contains(stdout, "unknown.txt [removed]") || !strings.Contains(stdout, "panel-control.sock [removed]") {
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
	root := NewRootCommand(Dependencies{Stdout: &stdout, Stderr: &stderr, Systemd: &fakeSystemdService{}})
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
	root = NewRootCommand(Dependencies{Stdout: &stdout, Stderr: &stderr, Systemd: &fakeSystemdService{}})
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
				filesResult:  panelSystemd.FilesResult{Scope: panelSystemd.ScopeUser, SettingsPath: path, Files: []panelSystemd.FileStatus{{Path: "fixture.service", State: "managed", Managed: true}}},
				statusResult: panelSystemd.Status{Scope: panelSystemd.ScopeUser, UnitFileSettingsPath: path},
			}
			report, err := inspectInstanceFiles(context.Background(), path, panelSystemd.ScopeUser, service)
			if err != nil {
				t.Fatal(err)
			}
			if kind != "matching" {
				report.ServiceMatches = false
				report.Service.SettingsPath = "/different/setting.json"
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
			if kind == "matching" && service.uninstallRequest.Scope != panelSystemd.ScopeUser {
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
			service := &fakeSystemdService{filesResult: panelSystemd.FilesResult{
				Scope: panelSystemd.ScopeUser, SettingsPath: serviceSettings,
				Files: []panelSystemd.FileStatus{{Path: filepath.Join(outside, "panel.service"), State: "managed", Managed: true}},
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
	saveConfigurationFixture(t, path, "{}")
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
	service := &fakeSystemdService{err: panelSystemd.ErrUnsupportedOS}
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
