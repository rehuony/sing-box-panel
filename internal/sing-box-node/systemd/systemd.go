// SPDX-License-Identifier: GPL-3.0-or-later

// Package systemd defines the node's systemd service: its audited templates
// and its data directory, which identifies an installation. The shared
// systemd package runs the lifecycle.
package systemd

import (
	"context"
	_ "embed"
	"errors"
	"fmt"
	"io"

	"github.com/rehuony/sing-box-panel/internal/selfupdate"
	"github.com/rehuony/sing-box-panel/internal/sing-box-node/settings"
	"github.com/rehuony/sing-box-panel/internal/systemd"
)

const (
	// Name names the node's unit, system account, executable and data
	// directory.
	Name     = "sing-box-node"
	UnitName = Name + ".service"
	// SystemDataDir is the data directory of the system service.
	SystemDataDir = "/var/lib/" + Name
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

// Manager is the node's systemd lifecycle; the node creates no state when
// it is installed.
type Manager = systemd.Manager[struct{}]

// ErrNotRunnable reports a node that its service could not run: it is
// neither enrolled nor has its console enabled.
var ErrNotRunnable = errors.New("the node is not enrolled and its console is disabled; run sing-box-node enroll --code <code> or sing-box-node console enable first")

// Definition is the node service: its data directory is its state.
func Definition() systemd.Definition[struct{}] {
	return systemd.Definition[struct{}]{
		Name: Name, AccountDescription: "Sing-Box Node service account",
		SystemUnit: systemUnitTemplate, UserUnit: userUnitTemplate, Sysusers: sysusersTemplate, Tmpfiles: tmpfilesTemplate,
		SystemStatePath: SystemDataDir, StateFlag: "--data-dir", StateLabel: "data directory",
		Arguments:          func(dataDir string) string { return "--data-dir " + dataDir + " run" },
		State:              nodeState{},
		Companions:         updateCompanions(),
		CompanionArguments: func(dataDir string) string { return "--data-dir " + dataDir + " update --apply-staged" },
	}
}

func New(options systemd.Options) (*Manager, error) {
	return systemd.New(Definition(), options)
}

// nodeState is the node's data directory: enrollment, console settings,
// node.db and installed cores.
type nodeState struct{}

func (nodeState) DataDir(dataDir, _ string, now bool) (string, error) {
	if now {
		if err := runnable(dataDir); err != nil {
			return "", err
		}
	}
	return dataDir, nil
}

// runnable checks that sing-box-node run can start in dataDir: the node is
// enrolled, or its console is enabled so it can be enrolled from there.
func runnable(dataDir string) error {
	_, err := settings.Load(dataDir)
	if !errors.Is(err, settings.ErrNotEnrolled) {
		if err != nil {
			return fmt.Errorf("%w: %w", systemd.ErrInvalid, err)
		}
		return nil
	}
	console, err := settings.LoadConsole(dataDir)
	if err != nil {
		return fmt.Errorf("%w: %w", systemd.ErrInvalid, err)
	}
	if !console.Enabled {
		return fmt.Errorf("%w: %w", systemd.ErrInvalid, ErrNotRunnable)
	}
	return nil
}

func (nodeState) Validate(string, string, string) error { return nil }

func (nodeState) Location(string) (string, string, bool, error) { return "", "", false, nil }

func (nodeState) Relocate(context.Context, string, string) error { return nil }

func (nodeState) Prepare(context.Context, string, string, bool) (*struct{}, error) { return nil, nil }

func (nodeState) Owned(string) []systemd.OwnedPath { return nil }

func (nodeState) Retained(string) ([]string, error) { return nil, nil }

func (nodeState) Lock(context.Context, string) (io.Closer, error) { return nil, nil }
