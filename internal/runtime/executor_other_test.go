//go:build !linux

// SPDX-License-Identifier: GPL-3.0-or-later

package runtime

import (
	"errors"
	"testing"
)

func TestDefaultExecutorIsExplicitlyUnavailableOutsideLinux(t *testing.T) {
	t.Parallel()

	_, err := NewManager(Options{RuntimeDir: t.TempDir()})
	if !errors.Is(err, ErrUnavailable) {
		t.Fatalf("NewManager error = %v, want ErrUnavailable", err)
	}
}
