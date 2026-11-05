// SPDX-License-Identifier: GPL-3.0-or-later

// Package systemd defines the panel's systemd service: its audited templates
// and how its settings file identifies an installation, selects its data
// directory and moves it. The shared systemd package runs the lifecycle.
package systemd

import (
	_ "embed"

	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/rehuony/sing-box-panel/internal/sing-box-panel/installation"
	"github.com/rehuony/sing-box-panel/internal/sing-box-panel/settings"
	"github.com/rehuony/sing-box-panel/internal/systemd"
)

const (
	// Name names the panel's unit, system account, executable and data
	// directory.
	Name     = "sing-box-panel"
	UnitName = Name + ".service"
	// SystemSettingsPath is the settings file of the system service.
	SystemSettingsPath = "/etc/sing-box-panel/setting.json"
)

// The audited packaging templates. The installer and the release artifacts use
// the same bytes, so their security settings cannot drift apart.
var (
	//go:embed templates/system/sing-box-panel.service
	systemUnitTemplate []byte
	//go:embed templates/user/sing-box-panel.service
	userUnitTemplate []byte
	//go:embed templates/sysusers.d/sing-box-panel.conf
	sysusersTemplate []byte
	//go:embed templates/tmpfiles.d/sing-box-panel.conf
	tmpfilesTemplate []byte
)

// Manager is the panel's systemd lifecycle; it reports the settings that
// installing or starting the service created.
type Manager = systemd.Manager[settings.Settings]

// Definition is the panel service: its settings file is its state.
func Definition() systemd.Definition[settings.Settings] {
	return systemd.Definition[settings.Settings]{
		Name: Name, AccountDescription: "Sing-Box Panel service account",
		SystemUnit: systemUnitTemplate, UserUnit: userUnitTemplate, Sysusers: sysusersTemplate, Tmpfiles: tmpfilesTemplate,
		SystemStatePath: SystemSettingsPath, StateFlag: "--config", StateShortFlag: "-c", StateLabel: "settings", StateFile: true,
		Arguments: func(settingsPath string) string { return "server start --config " + settingsPath },
		State:     panelState{},
	}
}

func New(options systemd.Options) (*Manager, error) {
	return systemd.New(Definition(), options)
}

// panelState is the panel's settings file and the data directory it selects.
type panelState struct{}

func (panelState) DataDir(settingsPath, fallback string, now bool) (string, error) {
	dataDir, err := settings.ConfiguredDataDir(settingsPath)
	switch {
	case errors.Is(err, os.ErrNotExist):
		if _, statErr := os.Lstat(settingsPath); !errors.Is(statErr, os.ErrNotExist) {
			return "", fmt.Errorf("%w: settings must be a regular file", systemd.ErrInvalid)
		}
		dataDir = fallback
		if dataDir == "" {
			dataDir = settings.Defaults().DataDir
		}
		location, err := settings.ReadDataLocation(settingsPath)
		if err == nil {
			if location.Move != nil {
				return "", errors.New("restore settings to finish the pending data migration")
			}
			return location.DataDir, nil
		}
		if !errors.Is(err, os.ErrNotExist) {
			return "", err
		}
		return dataDir, nil
	case err != nil:
		return "", fmt.Errorf("%w: %w", systemd.ErrInvalid, err)
	case now:
		if _, err := settings.Load(settingsPath); err != nil {
			return "", fmt.Errorf("%w: %w", systemd.ErrInvalid, err)
		}
	}
	return dataDir, nil
}

func (panelState) Validate(executable, settingsPath, dataDir string) error {
	if err := preflightSettingsPaths(settingsPath); err != nil {
		return err
	}
	return installation.ValidateCleanup(installation.Report{
		HistoryPath: installation.DefaultHistoryPath(), DataDir: dataDir, SettingsPath: settingsPath,
		Entries: []installation.Entry{{Path: executable}},
	})
}

// preflightSettingsPaths validates the settings sidecars before
// initialization, migration, unit writes or account preparation. The
// mutation paths keep their own checks for concurrent edits.
func preflightSettingsPaths(path string) error {
	directory := filepath.Dir(path)
	if info, err := os.Lstat(directory); err == nil {
		if !info.IsDir() {
			return fmt.Errorf("%w: settings directory %q must be a physical directory", systemd.ErrInvalid, directory)
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	for _, suffix := range []string{"", ".lock", ".pending", ".location"} {
		info, err := os.Lstat(path + suffix)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return fmt.Errorf("inspect settings path %q: %w", path+suffix, err)
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("%w: settings path %q must be a regular file", systemd.ErrInvalid, path+suffix)
		}
	}
	return nil
}

func (panelState) Location(settingsPath string) (string, string, bool, error) {
	location, err := settings.ReadDataLocation(settingsPath)
	if errors.Is(err, os.ErrNotExist) {
		return "", "", false, nil
	}
	if err != nil {
		return "", "", false, err
	}
	target, err := settings.ConfiguredDataDir(settingsPath)
	if errors.Is(err, os.ErrNotExist) {
		if _, statErr := os.Lstat(settingsPath); errors.Is(statErr, os.ErrNotExist) {
			return "", "", false, nil
		}
	}
	if err != nil {
		return "", "", false, err
	}
	return location.DataDir, target, location.Move != nil, nil
}

func (panelState) Relocate(ctx context.Context, settingsPath, dataDir string) error {
	location, err := settings.ReadDataLocation(settingsPath)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if location.DataDir == dataDir && location.Move == nil {
		return nil
	}
	prepared, err := installation.PrepareDataLocation(ctx, settingsPath)
	if err != nil {
		return err
	}
	if prepared.DataDir != dataDir {
		return errors.New("data directory changed during service installation; retry with the current settings")
	}
	return nil
}

func (panelState) Prepare(ctx context.Context, settingsPath, dataDir string, now bool) (*settings.Settings, error) {
	value, created, err := settings.EnsureFile(ctx, settingsPath, dataDir)
	var initial *settings.Settings
	if created {
		initial = &value
	}
	if err != nil {
		return initial, err
	}
	configured, err := settings.ConfiguredDataDir(settingsPath)
	if err != nil || configured != dataDir {
		return initial, errors.New("settings changed during service preparation; inspect and retry")
	}
	if now {
		if _, err := settings.Load(settingsPath); err != nil {
			return initial, err
		}
	}
	return initial, nil
}

func (panelState) Owned(settingsPath string) []systemd.OwnedPath {
	owned := []systemd.OwnedPath{
		{Path: filepath.Dir(settingsPath), Mode: 0o700, Directory: true},
		{Path: settingsPath, Mode: 0o600},
	}
	for _, suffix := range []string{".lock", ".pending", ".location"} {
		owned = append(owned, systemd.OwnedPath{Path: settingsPath + suffix, Mode: 0o600, Optional: true})
	}
	return owned
}

func (panelState) Retained(settingsPath string) ([]string, error) {
	if err := preflightSettingsPaths(settingsPath); err != nil {
		return nil, err
	}
	paths := []string{filepath.Dir(settingsPath)}
	configured, err := settings.ConfiguredDataDir(settingsPath)
	if err == nil {
		paths = append(paths, configured)
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	location, err := settings.ReadDataLocation(settingsPath)
	if err == nil {
		paths = append(paths, location.DataDir)
		if location.Move != nil {
			paths = append(paths, location.Move.Target)
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	return paths, nil
}

func (panelState) Lock(ctx context.Context, settingsPath string) (io.Closer, error) {
	if _, err := os.Lstat(filepath.Dir(settingsPath)); err != nil {
		return nil, nil
	}
	lock, err := settings.TryLock(ctx, settingsPath)
	if err != nil {
		return nil, err
	}
	return lock, nil
}
