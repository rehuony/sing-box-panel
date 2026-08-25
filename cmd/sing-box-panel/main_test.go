// SPDX-License-Identifier: GPL-3.0-or-later

package main

import (
	"context"
	"os"
	"sync/atomic"
	"syscall"
	"testing"
)

func TestWatchSignalContextRecordsConventionalExitCodeBeforeCancel(t *testing.T) {
	tests := []struct {
		name   string
		signal os.Signal
		want   int32
	}{
		{name: "SIGINT", signal: os.Interrupt, want: 130},
		{name: "SIGTERM", signal: syscall.SIGTERM, want: 143},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			signals := make(chan os.Signal, 1)
			done := make(chan struct{})
			var exitCode atomic.Int32
			signals <- testCase.signal

			watchSignalContext(signals, done, cancel, &exitCode)

			if ctx.Err() != context.Canceled {
				t.Fatalf("signal context error = %v", ctx.Err())
			}
			if got := exitCode.Load(); got != testCase.want {
				t.Fatalf("signal exit code = %d, want %d", got, testCase.want)
			}
		})
	}
}
