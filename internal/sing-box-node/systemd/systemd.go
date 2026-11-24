// SPDX-License-Identifier: GPL-3.0-or-later

// Package systemd defines the node's systemd service: its audited templates
// and its settings file, which identifies an installation and selects its
// data directory. The shared systemd package runs the lifecycle.
package systemd

import (
	"context"
	_ "embed"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/rehuony/sing-box-panel/internal/selfupdate"
	"github.com/rehuony/sing-box-panel/internal/settingsfile"
	"github.com/rehuony/sing-box-panel/internal/sing-box-node/settings"
	"github.com/rehuony/sing-box-panel/internal/systemd"
)

const (
	// Name names the node's unit, system account, executable and data
	// directory.
	Name     = "sing-box-node"
	UnitName = Name + ".service"
	// SystemSettingsPath is the settings file of the system service.
	SystemSettingsPath = settings.SystemPath
)

// The audited packaging templates. The installer and the release artifacts use
// the same bytes, so their security settings cannot drift apart.
var (
	//go:embed templates/system/sing-box-node.service
	systemUnitTemplate []byte
	//go:embed templates/user/sing-box-node.service
	userUnitTemplate []byte
	//go:embed templates/sysusers.d/sing-box-node.conf
	sysusersTemplate []byte
	//go:embed templates/tmpfiles.d/sing-box-node.conf
	tmpfilesTemplate []byte
	//go:embed templates/system/sing-box-node-update.path
	updatePathTemplate []byte
	//go:embed templates/system/sing-box-node-update.service
	updateServiceTemplate []byte
)

// The root update helper of the system service: a path unit that watches
// the update request in the data directory and the oneshot service it
// starts, which applies the staged release.
const (
	UpdatePathUnit    = Name + "-update.path"
	UpdateServiceUnit = Name + "-update.service"
)

func updateCompanions() []systemd.Companion {
	return []systemd.Companion{
		{Name: UpdatePathUnit, Template: updatePathTemplate, Watch: selfupdate.UpdatesDir + "/" + selfupdate.RequestName, Enable: true},
		{Name: UpdateServiceUnit, Template: updateServiceTemplate},
	}
}

// Manager is the node's systemd lifecycle; it reports the settings that
// installing or starting the service created.
type Manager = systemd.Manager[settings.Settings]

// Definition is the node service: its settings file is its state.
func Definition() systemd.Definition[settings.Settings] {
	return systemd.Definition[settings.Settings]{
		Name: Name, AccountDescription: "Sing-Box Node service account",
		SystemUnit: systemUnitTemplate, UserUnit: userUnitTemplate, Sysusers: sysusersTemplate, Tmpfiles: tmpfilesTemplate,
		SystemStatePath: SystemSettingsPath, StateFlag: "--config", StateShortFlag: "-c", StateLabel: "settings", StateFile: true,
		Arguments:          func(settingsPath string) string { return "server start --config " + settingsPath },
		State:              nodeState{},
		Companions:         updateCompanions(),
		CompanionArguments: func(dataDir string) string { return "update --apply-staged --data-dir " + dataDir },
	}
}

func New(options systemd.Options) (*Manager, error) {
	return systemd.New(Definition(), options)
}

// nodeState is the node's settings file and the data directory it selects:
// enrollment, node.db, installed cores and core logs.
type nodeState struct{}

func (nodeState) DataDir(settingsPath, fallback string, now bool) (string, error) {
	dataDir, err := settings.LoadDataDir(settingsPath)
	switch {
	case errors.Is(err, os.ErrNotExist):
		if _, statErr := os.Lstat(settingsPath); !errors.Is(statErr, os.ErrNotExist) {
			return "", fmt.Errorf("%w: settings must be a regular file", systemd.ErrInvalid)
		}
		if fallback == "" {
			fallback = settings.Defaults().DataDir
		}
		return fallback, nil
	case err != nil:
		return "", fmt.Errorf("%w: %w", systemd.ErrInvalid, err)
	case now:
		if _, err := settings.Load(settingsPath); err != nil {
			return "", fmt.Errorf("%w: %w", systemd.ErrInvalid, err)
		}
	}
	return dataDir, nil
}

// Validate refuses settings paths that are not regular files before
// anything is written.
func (nodeState) Validate(_, settingsPath, _ string) error {
	directory := filepath.Dir(settingsPath)
	if info, err := os.Lstat(directory); err == nil {
		if !info.IsDir() {
			return fmt.Errorf("%w: settings directory %q must be a physical directory", systemd.ErrInvalid, directory)
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	for _, path := range []string{settingsPath, settingsPath + ".lock"} {
		info, err := os.Lstat(path)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return fmt.Errorf("inspect settings path %q: %w", path, err)
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("%w: settings path %q must be a regular file", systemd.ErrInvalid, path)
		}
	}
	return nil
}

func (nodeState) Location(string) (string, string, bool, error) { return "", "", false, nil }

func (nodeState) Relocate(context.Context, string, string) error { return nil }

func (nodeState) Prepare(ctx context.Context, settingsPath, dataDir string, now bool) (*settings.Settings, error) {
	value, created, err := settings.EnsureFile(ctx, settingsPath, dataDir)
	var initial *settings.Settings
	if created {
		initial = &value
	}
	if err != nil {
		return initial, err
	}
	configured, err := settings.LoadDataDir(settingsPath)
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

func (nodeState) Owned(settingsPath string) []systemd.OwnedPath {
	return []systemd.OwnedPath{
		{Path: filepath.Dir(settingsPath), Mode: 0o700, Directory: true},
		{Path: settingsPath, Mode: 0o600},
		{Path: settingsPath + ".lock", Mode: 0o600, Optional: true},
	}
}

func (nodeState) Retained(settingsPath string) ([]string, error) {
	paths := []string{filepath.Dir(settingsPath)}
	dataDir, err := settings.LoadDataDir(settingsPath)
	if err == nil {
		paths = append(paths, dataDir)
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	return paths, nil
}

func (nodeState) Lock(ctx context.Context, settingsPath string) (io.Closer, error) {
	if _, err := os.Lstat(filepath.Dir(settingsPath)); err != nil {
		return nil, nil
	}
	lock, err := settingsfile.TryLock(ctx, settingsPath)
	if err != nil {
		return nil, err
	}
	return lock, nil
}
