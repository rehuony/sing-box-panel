// SPDX-License-Identifier: GPL-3.0-or-later

package service

import (
	"testing"

	"github.com/rehuony/sing-box-panel/internal/sing-box-node/store"
)

func TestRuntimeRestartAlwaysRequiresTransition(t *testing.T) {
	t.Parallel()

	if !runtimeIntentNeedsTransition(store.RuntimeIntentRestart, true) {
		t.Fatal("restart of an already exact bundle was treated as a no-op")
	}
	if runtimeIntentNeedsTransition(store.RuntimeIntentStart, true) {
		t.Fatal("start of an already exact bundle should remain idempotent")
	}
	if !runtimeIntentNeedsTransition(store.RuntimeIntentApply, false) {
		t.Fatal("a non-running apply must start the runtime")
	}
}
