// SPDX-License-Identifier: GPL-3.0-or-later

//go:build linux

package runtimeidentity

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/rehuony/sing-box-panel/internal/store"
)

type procInspector struct {
	root string
}

func platformInspector() ProcessInspector {
	return procInspector{root: "/proc"}
}

func (inspector procInspector) Verify(
	ctx context.Context,
	observation store.RuntimeObservation,
	artifact store.CoreArtifact,
) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	token, err := inspector.ProcessStartToken(ctx, observation.PID)
	if err != nil {
		return err
	}
	if token != observation.ProcessStartToken {
		return errors.New("process start token changed")
	}
	if artifact.ID != observation.CoreArtifactID || artifact.ExactVersion != observation.ExactCoreVersion ||
		artifact.ReportedVersion != observation.ExactCoreVersion || artifact.ArchiveSHA256 != observation.ArchiveSHA256 ||
		artifact.BinarySHA256 != observation.BinarySHA256 {
		return errors.New("persisted artifact identity changed")
	}
	processExecutable, err := os.Stat(filepath.Join(inspector.procRoot(), strconv.Itoa(observation.PID), "exe"))
	if err != nil {
		return fmt.Errorf("inspect live executable: %w", err)
	}
	artifactExecutable, err := os.Stat(artifact.BinaryPath)
	if err != nil {
		return fmt.Errorf("inspect immutable executable: %w", err)
	}
	if !processExecutable.Mode().IsRegular() || !artifactExecutable.Mode().IsRegular() || !os.SameFile(processExecutable, artifactExecutable) {
		return errors.New("live executable is not the recorded artifact")
	}
	return nil
}

func (inspector procInspector) ProcessStartToken(ctx context.Context, pid int) (string, error) {
	if pid <= 0 {
		return "", errors.New("process id must be positive")
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}
	data, err := os.ReadFile(filepath.Join(inspector.procRoot(), strconv.Itoa(pid), "stat"))
	if err != nil {
		return "", fmt.Errorf("read process stat: %w", err)
	}
	closing := strings.LastIndexByte(string(data), ')')
	if closing < 1 || closing+2 >= len(data) {
		return "", errors.New("process stat format is invalid")
	}
	fields := strings.Fields(string(data[closing+1:]))
	// After the parenthesized command, index zero is field 3 (state), making
	// field 22 (starttime) index 19.
	if len(fields) <= 19 {
		return "", errors.New("process stat is missing starttime")
	}
	if _, err := strconv.ParseUint(fields[19], 10, 64); err != nil {
		return "", errors.New("process starttime is invalid")
	}
	return fields[19], nil
}

func (inspector procInspector) procRoot() string {
	if inspector.root == "" {
		return "/proc"
	}
	return inspector.root
}
