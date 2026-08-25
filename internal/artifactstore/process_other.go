// SPDX-License-Identifier: GPL-3.0-or-later

//go:build !unix

package artifactstore

import (
	"os/exec"
)

func configureVersionCommand(*exec.Cmd) error { return ErrUnsafeExecution }

func terminateVersionCommand(*exec.Cmd) error { return nil }
