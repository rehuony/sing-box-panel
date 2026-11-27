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

	"github.com/rehuony/sing-box-panel/internal/buildinfo"
	"github.com/rehuony/sing-box-panel/internal/cliui"
	"github.com/rehuony/sing-box-panel/internal/procctl"
	"github.com/rehuony/sing-box-panel/internal/settingsfile"
	"github.com/rehuony/sing-box-panel/internal/sing-box-panel/backup"
	"github.com/rehuony/sing-box-panel/internal/sing-box-panel/settings"
	"github.com/rehuony/sing-box-panel/internal/sing-box-panel/store"
	"github.com/spf13/cobra"
)

// backupInstance is the selected instance's backups as the command line
// reaches them: through read-only snapshots while the panel runs, and under
// the panel's lease, so that it cannot start meanwhile, while it is stopped.
type backupInstance struct {
	service *backup.Service
	lease   *procctl.Lease
	dataDir string
}

func openBackupInstance(ctx context.Context, settingsPath string, build buildinfo.Info, requireStopped bool) (*backupInstance, error) {
	configuration, err := settings.Load(settingsPath)
	if err != nil {
		return nil, cliui.Fail(cliui.ErrorValidation, "settings_invalid", err)
	}
	running, err := procctl.LeaseHeld(configuration.DataDir)
	if err != nil && !errors.Is(err, procctl.ErrLeaseUnsupported) {
		return nil, cliui.Fail(cliui.ErrorUnavailable, "instance_state_unknown", err)
	}
	instance := &backupInstance{dataDir: configuration.DataDir}
	if running && requireStopped {
		return nil, &cliui.Error{Kind: cliui.ErrorConflict, Code: "panel_running",
			Message: "the panel is running; stop it first, or restore from its Web interface"}
	}
	if !running {
		if instance.lease, err = procctl.AcquireLease(configuration.DataDir); err != nil {
			return nil, cliui.Fail(cliui.ErrorConflict, "panel_starting", err)
		}
	}
	instance.service = backup.New(backup.Options{
		DataDir: configuration.DataDir, SettingsPath: configuration.Path(), PanelVersion: build.Version,
		Document: func(ctx context.Context) ([]byte, error) { return savedSettings(ctx, configuration.Path()) },
	})
	return instance, nil
}

func (instance *backupInstance) Close() error {
	if instance.lease == nil {
		return nil
	}
	return instance.lease.Close()
}

// savedSettings reads the settings document under the settings lock; a save
// that needs recovery is the panel's to finish.
func savedSettings(ctx context.Context, path string) ([]byte, error) {
	lock, err := settingsfile.Lock(ctx, path)
	if err != nil {
		return nil, err
	}
	defer lock.Close()
	if err := settings.CheckPending(path); err != nil {
		return nil, err
	}
	raw, err := settings.Read(path)
	if err != nil {
		return nil, err
	}
	if _, err := settings.Parse(path, raw); err != nil {
		return nil, err
	}
	return raw, nil
}

// readPassphrase reads a passphrase from the first line of a file; "-" reads
// standard input.
func readPassphrase(path string, stdin io.Reader) (string, error) {
	if path == "" {
		return "", nil
	}
	var raw []byte
	var err error
	if path == "-" {
		raw, err = io.ReadAll(io.LimitReader(stdin, 4096))
	} else {
		raw, err = os.ReadFile(path)
	}
	if err != nil {
		return "", err
	}
	passphrase, _, _ := strings.Cut(string(raw), "\n")
	passphrase = strings.TrimSuffix(passphrase, "\r")
	if passphrase == "" {
		return "", errors.New("the passphrase file is empty")
	}
	return passphrase, nil
}

func backupFailure(err error) error {
	switch {
	case errors.Is(err, backup.ErrPassphraseRequired), errors.Is(err, backup.ErrPassphraseIncorrect),
		errors.Is(err, backup.ErrInvalid), errors.Is(err, backup.ErrTooNew):
		return cliui.Fail(cliui.ErrorValidation, "backup_invalid", err)
	case errors.Is(err, backup.ErrBusy), errors.Is(err, settings.ErrPending):
		return cliui.Fail(cliui.ErrorConflict, "backup_busy", err)
	}
	return cliui.Fail(cliui.ErrorUnavailable, "backup_failed", err)
}

func newSystemBackupCommand(state *options, build buildinfo.Info) *cobra.Command {
	var categories []string
	var passphraseFile, output string
	command := &cobra.Command{Use: "backup", Short: "Back up the settings and databases into the instance's backups directory", Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			passphrase, err := readPassphrase(passphraseFile, cmd.InOrStdin())
			if err != nil {
				return cliui.Fail(cliui.ErrorValidation, "passphrase_unreadable", err)
			}
			if passphrase != "" && len(passphrase) < 8 {
				return &cliui.Error{Kind: cliui.ErrorValidation, Code: "passphrase_too_short", Message: "use a passphrase of at least 8 bytes"}
			}
			instance, err := openBackupInstance(cmd.Context(), state.settingsPath, build, false)
			if err != nil {
				return err
			}
			chosen, err := parseCategories(categories, backup.Categories)
			if err != nil {
				return errors.Join(err, instance.Close())
			}
			info, err := instance.service.Create(cmd.Context(), backup.CreateOptions{Kind: backup.KindManual, Categories: chosen, Passphrase: passphrase})
			err = errors.Join(err, instance.Close())
			if err != nil {
				return backupFailure(err)
			}
			path := filepath.Join(instance.service.Directory(), info.Name)
			if output != "" {
				if err := copyBackup(path, output); err != nil {
					return cliui.Fail(cliui.ErrorUnavailable, "backup_copy_failed", err)
				}
				path = output
			}
			result := struct {
				backup.Info
				Path string `json:"path"`
			}{info, path}
			style := cliui.NewStyle(cmd.OutOrStdout(), state.format)
			return cliui.Write(cmd.OutOrStdout(), state.format, result,
				fmt.Sprintf("%s %s (%s)\n", style.Success("Backed up to"), path, byteSize(info.Size)))
		},
	}
	command.Flags().StringSliceVar(&categories, "categories", categoryNames(backup.DefaultCategories),
		"what to back up: "+strings.Join(categoryNames(backup.Categories), ", ")+", or all")
	command.Flags().StringVar(&passphraseFile, "passphrase-file", "", "encrypt with the passphrase on the first line of this file (- reads standard input)")
	command.Flags().StringVarP(&output, "output", "o", "", "also copy the backup to this file")
	return command
}

func copyBackup(source, target string) (err error) {
	input, err := os.Open(source)
	if err != nil {
		return err
	}
	defer input.Close()
	output, err := os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, output.Close()) }()
	if _, err := io.Copy(output, input); err != nil {
		return err
	}
	return output.Sync()
}

func newSystemRestoreCommand(state *options, build buildinfo.Info) *cobra.Command {
	var passphraseFile string
	var categories []string
	var takeService, yes bool
	command := &cobra.Command{Use: "restore FILE", Short: "Check a backup; --yes restores it into the stopped instance", Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, arguments []string) error {
			passphrase, err := readPassphrase(passphraseFile, cmd.InOrStdin())
			if err != nil {
				return cliui.Fail(cliui.ErrorValidation, "passphrase_unreadable", err)
			}
			archive, err := os.Open(arguments[0])
			if err != nil {
				return cliui.Fail(cliui.ErrorValidation, "backup_unreadable", err)
			}
			defer archive.Close()
			instance, err := openBackupInstance(cmd.Context(), state.settingsPath, build, true)
			if err != nil {
				return err
			}
			defer instance.Close()
			staged, err := instance.service.Stage(cmd.Context(), archive, passphrase)
			if err != nil {
				return backupFailure(err)
			}
			result := struct {
				Staged  backup.Staged       `json:"staged"`
				Applied bool                `json:"applied"`
				Before  *backup.Info        `json:"before,omitempty"`
				Effects *store.MergeEffects `json:"effects,omitempty"`
			}{Staged: staged}
			style := cliui.NewStyle(cmd.OutOrStdout(), state.format)
			if !yes {
				err := instance.service.Discard(staged.ID)
				return errors.Join(err, cliui.Write(cmd.OutOrStdout(), state.format, result, restorePreviewText(staged, style)+
					style.Muted("Run again with --yes to restore it; the current state is backed up first.")+"\n"))
			}
			chosen := staged.Manifest.Categories
			if len(categories) > 0 {
				if chosen, err = parseCategories(categories, staged.Manifest.Categories); err != nil {
					return err
				}
			}
			before, err := instance.service.RequestRestore(cmd.Context(), staged.ID, takeService, chosen)
			if err != nil {
				return backupFailure(err)
			}
			applied, err := backup.ApplyPending(cmd.Context(), state.settingsPath, instance.dataDir)
			if err != nil {
				return backupFailure(err)
			}
			if applied == nil || applied.Skipped != "" {
				reason := "the restore was not applied"
				if applied != nil {
					reason = applied.Skipped
				}
				return &cliui.Error{Kind: cliui.ErrorConflict, Code: "restore_skipped", Message: reason}
			}
			result.Applied, result.Before, result.Effects = true, &before, &applied.Effects
			return cliui.Write(cmd.OutOrStdout(), state.format, result, restorePreviewText(staged, style)+effectsText(applied.Effects)+
				fmt.Sprintf("%s The state it replaced is in %s.\n", style.Success("Restored."), before.Name))
		},
	}
	command.Flags().StringVar(&passphraseFile, "passphrase-file", "", "decrypt with the passphrase on the first line of this file (- reads standard input)")
	command.Flags().StringSliceVar(&categories, "categories", nil, "what to restore of what the backup holds (default all of it)")
	command.Flags().BoolVar(&takeService, "take-service", false, "also take the backup's listener, base path and origin with its settings")
	command.Flags().BoolVar(&yes, "yes", false, "restore the backup, replacing the instance's settings and databases")
	return command
}

func restorePreviewText(staged backup.Staged, style cliui.Style) string {
	manifest := staged.Manifest
	counts := manifest.Snapshot.Counts
	var text strings.Builder
	fmt.Fprintf(&text, "%s\n", style.Title("Backup"))
	fmt.Fprintf(&text, "  made      %s by %s (%s)\n", manifest.CreatedAt.Local().Format("2006-01-02 15:04:05"), manifest.PanelVersion, manifest.Kind)
	fmt.Fprintf(&text, "  holds     %s\n", strings.Join(categoryNames(manifest.Categories), ", "))
	fmt.Fprintf(&text, "  records   %d nodes, %d sources, %d channels, %d rule sets, %d keys, %d days of history, %d log entries\n",
		counts.Nodes, counts.Sources, counts.Channels, counts.RuleSets, counts.Keys, counts.HistoryDays, counts.LogEntries)
	if staged.Server != nil {
		fmt.Fprintf(&text, "  listener  %s:%d%s\n", staged.Server.Host, staged.Server.Port, staged.Server.BasePath)
	}
	return text.String()
}

// effectsText lists what a partial restore changed beyond its categories.
func effectsText(effects store.MergeEffects) string {
	var changes []string
	for _, change := range []struct {
		count int
		text  string
	}{
		{effects.PoolNodes, "pool nodes removed"},
		{effects.ExitRules, "exit rules removed"},
		{effects.RejectedExits, "exit rules now reject"},
		{effects.DirectExits, "final exits now direct"},
		{effects.Grants, "grants removed"},
		{effects.HistoryNodes, "nodes' records removed"},
	} {
		if change.count > 0 {
			changes = append(changes, fmt.Sprintf("%d %s", change.count, change.text))
		}
	}
	if len(changes) == 0 {
		return ""
	}
	return "  repaired  " + strings.Join(changes, ", ") + "\n"
}

func categoryNames(categories []backup.Category) []string {
	names := make([]string, len(categories))
	for index, category := range categories {
		names[index] = string(category)
	}
	return names
}

// parseCategories reads category names, or all of allowed, into categories
// that allowed holds.
func parseCategories(names []string, allowed []backup.Category) ([]backup.Category, error) {
	var categories []backup.Category
	for _, name := range names {
		if name == "all" {
			categories = append(categories, allowed...)
			continue
		}
		categories = append(categories, backup.Category(strings.TrimSpace(name)))
	}
	categories, err := backup.NormalizeCategories(categories)
	if err == nil && slices.ContainsFunc(categories, func(category backup.Category) bool { return !slices.Contains(allowed, category) }) {
		err = backup.ErrCategories
	}
	if err != nil {
		return nil, &cliui.Error{Kind: cliui.ErrorValidation, Code: "backup_categories_invalid",
			Message: "choose from " + strings.Join(categoryNames(allowed), ", ") + ", or all"}
	}
	return categories, nil
}

func byteSize(size int64) string {
	units := []string{"B", "KiB", "MiB", "GiB", "TiB"}
	value, index := float64(size), 0
	for value >= 1024 && index < len(units)-1 {
		value /= 1024
		index++
	}
	if index == 0 {
		return fmt.Sprintf("%d B", size)
	}
	return fmt.Sprintf("%.1f %s", value, units[index])
}
