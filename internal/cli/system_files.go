// SPDX-License-Identifier: GPL-3.0-or-later

package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/mattn/go-isatty"
	"github.com/rehuony/sing-box-panel/internal/installation"
	"github.com/rehuony/sing-box-panel/internal/panelprocess"
	"github.com/rehuony/sing-box-panel/internal/settings"
	panelSystemd "github.com/rehuony/sing-box-panel/internal/systemd"
	"github.com/spf13/cobra"
)

func newSystemCommand(state *options, service panelSystemd.Service) *cobra.Command {
	root := group("system", "Inspect or prune the selected instance's files and data")
	root.AddCommand(newSystemDFCommand(state, service), newSystemPruneCommand(state, service))
	return root
}

type instanceFilesReport struct {
	installation.Report
	Service          panelSystemd.FilesResult `json:"service"`
	ServiceState     string                   `json:"service_state"`
	ServiceMatches   bool                     `json:"service_matches_instance"`
	ServiceDataDir   string                   `json:"service_data_dir,omitempty"`
	ServiceDataKnown bool                     `json:"service_data_known"`
	Preview          bool                     `json:"preview"`
}

func inspectInstanceFiles(ctx context.Context, path string, scope panelSystemd.Scope, service panelSystemd.Service) (instanceFilesReport, error) {
	files, err := installation.Inspect(ctx, path)
	result := instanceFilesReport{Report: files, ServiceState: "unavailable"}
	if err != nil {
		return result, err
	}
	if service == nil {
		return result, nil
	}
	result.Service, err = service.Files(ctx, scope)
	if errors.Is(err, panelSystemd.ErrUnsupportedOS) {
		result.ServiceState = "unsupported"
		return result, nil
	}
	if err != nil {
		return result, err
	}
	result.ServiceState = "inspected"
	result.ServiceMatches = result.Service.SettingsPath != "" && filepath.Clean(result.Service.SettingsPath) == files.SettingsPath
	if result.Service.SettingsPath != "" {
		dataDir, err := settings.LoadDataDir(result.Service.SettingsPath)
		if err == nil {
			result.ServiceDataDir = dataDir
			result.ServiceDataKnown = true
		}
	}
	return result, nil
}

func newSystemDFCommand(state *options, service panelSystemd.Service) *cobra.Command {
	var rawScope string
	command := &cobra.Command{Use: "df", Short: "List selected instance files, storage paths and cleanup scope", Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			scope, err := parseSystemScope(rawScope)
			if err != nil {
				return err
			}
			report, err := inspectInstanceFiles(cmd.Context(), state.settingsPath, scope, service)
			if err != nil {
				return &Error{Kind: ErrorValidation, Code: "instance_files_unavailable", Message: err.Error(), Cause: err}
			}
			return writeResult(cmd.OutOrStdout(), state.format, report, instanceFilesText(report, newFileTreeStyle(cmd.OutOrStdout(), state.format)))
		},
	}
	addSystemScopeFlag(command, &rawScope)
	return command
}

func newSystemPruneCommand(state *options, service panelSystemd.Service) *cobra.Command {
	var rawScope string
	var yes bool
	command := &cobra.Command{Use: "prune", Short: "Preview cleanup; --yes stops the instance and removes its settings and entire data directory", Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			scope, err := parseSystemScope(rawScope)
			if err != nil {
				return err
			}
			report, err := inspectInstanceFiles(cmd.Context(), state.settingsPath, scope, service)
			if err != nil {
				return &Error{Kind: ErrorValidation, Code: "instance_files_unavailable", Message: err.Error(), Cause: err}
			}
			report.Preview = !yes
			style := newFileTreeStyle(cmd.OutOrStdout(), state.format)
			if !yes {
				return writeResult(cmd.OutOrStdout(), state.format, report, instanceFilesText(report, style))
			}
			if err := installation.ValidateCleanup(report.Report); err != nil {
				if report.DataDir == "" {
					return &Error{Kind: ErrorValidation, Code: "instance_files_unavailable", Message: err.Error(), Cause: err}
				}
				return classifyCleanupError(err)
			}
			serviceRemoved, err := stopInstanceForCleanup(cmd.Context(), report, service)
			if err != nil {
				if len(serviceRemoved) > 0 {
					result := installation.CleanupResult{Removed: serviceRemoved, Retained: []string{}}
					_ = writeResult(cmd.OutOrStdout(), state.format, result, cleanupText(result, err, style))
				}
				return classifyCleanupError(err)
			}
			result, err := installation.Clean(cmd.Context(), report.Report)
			result.Removed = append(serviceRemoved, result.Removed...)
			if err != nil {
				if len(result.Removed) > 0 {
					_ = writeResult(cmd.OutOrStdout(), state.format, result, cleanupText(result, err, style))
				}
				return classifyCleanupError(err)
			}
			return writeResult(cmd.OutOrStdout(), state.format, result, cleanupText(result, nil, style))
		},
	}
	addSystemScopeFlag(command, &rawScope)
	command.Flags().BoolVar(&yes, "yes", false, "permanently remove the selected settings, all data-directory contents and matching managed service")
	return command
}

func stopInstanceForCleanup(ctx context.Context, report instanceFilesReport, service panelSystemd.Service) ([]string, error) {
	var removed []string
	if report.ServiceState == "unavailable" {
		return nil, errors.New("system service ownership could not be inspected")
	}
	if !report.ServiceMatches && len(report.Service.Files) > 0 && report.Service.Files[0].State != "missing" {
		if !report.ServiceDataKnown {
			return nil, errors.New("installed service settings are unknown; resolve its ownership before cleanup")
		}
		overlap, err := installation.DataDirectoriesOverlap(report.DataDir, report.ServiceDataDir)
		if err != nil {
			return nil, err
		}
		if overlap {
			return nil, errors.New("another service configuration shares or overlaps this data directory; select that service's settings before cleanup")
		}
		for _, path := range []string{report.Service.SettingsPath, report.ServiceDataDir} {
			contained, err := installation.DataDirectoryContainsPath(report.DataDir, path)
			if err != nil {
				return nil, err
			}
			if contained {
				return nil, errors.New("another service depends on a settings or data path inside this data directory; relocate it before cleanup")
			}
		}
	}
	if report.ServiceMatches {
		for _, file := range report.Service.Files {
			if file.State != "missing" && !file.Managed {
				return nil, fmt.Errorf("refusing cleanup of unmanaged service file %s", file.Path)
			}
		}
		status, err := service.Status(ctx, report.Service.Scope)
		if err != nil && !errors.Is(err, panelSystemd.ErrNotInstalled) {
			return nil, err
		}
		if err == nil && (status.NeedDaemonReload || filepath.Clean(status.UnitFileSettingsPath) != report.SettingsPath) {
			return nil, errors.New("service settings are ambiguous or changed on disk; resolve the service configuration before cleanup")
		}
		result, err := service.Uninstall(ctx, panelSystemd.UninstallRequest{Scope: report.Service.Scope})
		if err != nil && !errors.Is(err, panelSystemd.ErrNotInstalled) {
			return nil, err
		}
		removed = result.RemovedPaths
	}
	for _, entry := range report.Entries {
		if entry.Path == filepath.Join(report.DataDir, "panel-control.sock") && entry.State != "socket" && entry.State != "missing" {
			// A leftover non-socket cannot answer stop requests. Clean still must
			// acquire the runtime lease, so a live owner cannot be bypassed.
			return removed, nil
		}
	}
	stopCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	_, err := panelprocess.Stop(stopCtx, report.DataDir)
	return removed, err
}

func classifyCleanupError(err error) error {
	return &Error{Kind: ErrorConflict, Code: "instance_cleanup_failed", Message: err.Error(), Cause: err}
}

func instanceFilesText(report instanceFilesReport, style fileTreeStyle) string {
	var text strings.Builder
	heading := "Instance files"
	if report.Preview {
		heading = "Cleanup preview — no changes"
	}
	text.WriteString(style.paint("1", heading) + "\n")
	configPath := style.path(report.SettingsPath)
	for _, entry := range report.Entries {
		if entry.Role == "panel bootstrap settings" && entry.State == "missing" {
			configPath += " (missing)"
			break
		}
	}
	fmt.Fprintf(&text, "Config:     %s\n", configPath)
	for _, entry := range report.Entries {
		if entry.Role == "panel executable" {
			fmt.Fprintf(&text, "Executable: %s\n", style.path(entry.Path))
			break
		}
	}
	switch report.ServiceState {
	case "unsupported":
		text.WriteString(style.paint("2", "Systemd:    unsupported on this platform") + "\n")
	case "unavailable":
		text.WriteString(style.paint("2", "Systemd:    inspection unavailable") + "\n")
	}
	parents := make(map[string]bool)
	for _, entry := range report.Entries {
		if entry.State != "missing" {
			parents[filepath.Dir(entry.Path)] = true
		}
	}
	entries := make([]fileTreeEntry, 0, len(report.Entries)+len(report.Service.Files))
	for _, entry := range report.Entries {
		if entry.State == "missing" {
			continue
		}
		var labels []string
		switch entry.State {
		case "file":
		case "directory":
			if !parents[entry.Path] {
				labels = append(labels, "empty")
			}
		case "symlink":
			labels = append(labels, "link")
		default:
			labels = append(labels, entry.State)
		}
		switch entry.Role {
		case "panel executable":
			labels = append(labels, "executable")
		case "panel bootstrap settings":
			labels = append(labels, "settings")
		case "instance data directory":
			labels = append(labels, "data")
		}
		entries = append(entries, fileTreeEntry{path: entry.Path, label: strings.Join(labels, ", "), directory: entry.State == "directory"})
	}
	for _, entry := range report.Service.Files {
		if entry.State == "missing" {
			continue
		}
		label := "service"
		if report.Service.Scope != "" {
			label += ", " + string(report.Service.Scope)
		}
		if !report.ServiceMatches || !entry.Managed {
			label += ", outside scope"
		}
		entries = append(entries, fileTreeEntry{path: entry.Path, label: label})
	}
	text.WriteByte('\n')
	text.WriteString(fileTreeText(entries, style))
	if report.Preview {
		if report.DataDir == "" {
			text.WriteString("\n\n" + style.paint("33", "Cleanup unavailable: restore the settings file to identify the data directory."))
		} else {
			text.WriteString("\n\n" + style.paint("33", "Pass --yes to stop this instance and permanently delete its settings and all data."))
		}
	}
	return text.String()
}

func cleanupText(result installation.CleanupResult, cleanupErr error, style fileTreeStyle) string {
	heading := "Cleanup results"
	if cleanupErr != nil {
		heading = "Cleanup interrupted; confirmed results only"
		heading = style.paint("31", heading)
	} else {
		heading = style.paint("1", heading)
	}
	entries := make([]fileTreeEntry, 0, len(result.Removed)+len(result.Retained))
	for _, path := range result.Removed {
		entries = append(entries, fileTreeEntry{path: path, label: "removed"})
	}
	for _, path := range result.Retained {
		entries = append(entries, fileTreeEntry{path: path, label: "retained"})
	}
	if len(entries) == 0 {
		return heading + ": no paths reported."
	}
	return heading + "\n\n" + fileTreeText(entries, style)
}

type fileTreeEntry struct {
	path      string
	label     string
	directory bool
}

type fileTreeNode struct {
	path      string
	labels    []string
	children  []*fileTreeNode
	directory bool
}

// fileTreeText groups only reported paths. Unlabeled ancestors are structural;
// rendering never inspects the filesystem or expands a symlink's target.
func fileTreeText(entries []fileTreeEntry, style fileTreeStyle) string {
	nodes := make(map[string]*fileTreeNode)
	var roots []*fileTreeNode
	var ensureNode func(string) *fileTreeNode
	ensureNode = func(path string) *fileTreeNode {
		if node, ok := nodes[path]; ok {
			return node
		}
		node := &fileTreeNode{path: path}
		nodes[path] = node
		if parent := filepath.Dir(path); parent != path {
			parentNode := ensureNode(parent)
			parentNode.children = append(parentNode.children, node)
		} else {
			roots = append(roots, node)
		}
		return node
	}
	for _, entry := range entries {
		node := ensureNode(filepath.Clean(entry.path))
		node.labels = append(node.labels, entry.label)
		node.directory = node.directory || entry.directory
	}
	// Avoid a redundant filesystem-root level while preserving explicit roots.
	var visibleRoots []*fileTreeNode
	for _, root := range roots {
		if len(root.labels) == 0 {
			visibleRoots = append(visibleRoots, root.children...)
		} else {
			visibleRoots = append(visibleRoots, root)
		}
	}
	for _, node := range nodes {
		slices.Sort(node.labels)
		slices.SortFunc(node.children, func(a, b *fileTreeNode) int { return strings.Compare(a.path, b.path) })
	}
	slices.SortFunc(visibleRoots, func(a, b *fileTreeNode) int { return strings.Compare(a.path, b.path) })
	var text strings.Builder
	var writeNode func(*fileTreeNode, string, string, string)
	writeNode = func(node *fileTreeNode, name, prefix, branch string) {
		for len(node.labels) == 0 && len(node.children) == 1 {
			node = node.children[0]
			name = filepath.Join(name, filepath.Base(node.path))
		}
		if branch == "" {
			name = style.path(name)
		}
		if node.directory || len(node.labels) == 0 {
			if !strings.HasSuffix(name, string(filepath.Separator)) {
				name += string(filepath.Separator)
			}
			name = style.paint("34", name)
		}
		fmt.Fprintf(&text, "%s%s", style.paint("2", prefix+branch), name)
		for _, label := range node.labels {
			if label == "" {
				continue
			}
			fmt.Fprintf(&text, " %s", style.label(label))
		}
		text.WriteByte('\n')
		switch branch {
		case "├── ":
			prefix += "│   "
		case "└── ":
			prefix += "    "
		}
		for i, child := range node.children {
			childBranch := "├── "
			if i == len(node.children)-1 {
				childBranch = "└── "
			}
			writeNode(child, filepath.Base(child.path), prefix, childBranch)
		}
	}
	for i, root := range visibleRoots {
		if i > 0 {
			text.WriteByte('\n')
		}
		writeNode(root, root.path, "", "")
	}
	return strings.TrimSuffix(text.String(), "\n")
}

type fileTreeStyle struct {
	color bool
	home  string
}

func newFileTreeStyle(writer io.Writer, format outputFormat) fileTreeStyle {
	home, _ := os.UserHomeDir()
	style := fileTreeStyle{home: home}
	term := os.Getenv("TERM")
	if format != outputText || os.Getenv("NO_COLOR") != "" || term == "" || term == "dumb" {
		return style
	}
	if terminal, ok := writer.(interface{ Fd() uintptr }); ok {
		style.color = isatty.IsTerminal(terminal.Fd())
	}
	return style
}

func (style fileTreeStyle) paint(code, text string) string {
	if !style.color || text == "" {
		return text
	}
	return "\x1b[" + code + "m" + text + "\x1b[0m"
}

func (style fileTreeStyle) label(label string) string {
	parts := strings.Split(label, ", ")
	for i, part := range parts {
		code := "2"
		switch part {
		case "link", "outside scope", "retained":
			code = "36"
		case "removed":
			code = "32"
		}
		parts[i] = style.paint(code, part)
	}
	return style.paint("2", "[") + strings.Join(parts, style.paint("2", ", ")) + style.paint("2", "]")
}

func (style fileTreeStyle) path(path string) string {
	home := filepath.Clean(style.home)
	if filepath.IsAbs(home) && home != string(filepath.Separator) &&
		(path == home || strings.HasPrefix(path, home+string(filepath.Separator))) {
		return "~" + strings.TrimPrefix(path, home)
	}
	return path
}
