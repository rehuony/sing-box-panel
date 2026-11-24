// SPDX-License-Identifier: GPL-3.0-or-later

package agent

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"errors"
	"testing"
	"time"

	"github.com/rehuony/sing-box-panel/internal/buildinfo"
	"github.com/rehuony/sing-box-panel/internal/sing-box-node/settings"
)

func TestLinkStartsOverWhenTheEnrollmentChanges(t *testing.T) {
	dataDir := t.TempDir()
	enrollment := func(nodeID string) settings.Enrollment {
		_, key, err := ed25519.GenerateKey(rand.Reader)
		if err != nil {
			t.Fatal(err)
		}
		return settings.Enrollment{Endpoint: "panel.example:443", PanelID: "panel", NodeID: nodeID, Key: key}
	}
	if err := settings.SaveEnrollment(dataDir, enrollment("first")); err != nil {
		t.Fatal(err)
	}
	link, err := OpenLink(dataDir, buildinfo.Info{})
	if err != nil {
		t.Fatal(err)
	}
	// Each session runs until its enrollment changes; "failing" never opens.
	sessions := make(chan string, 8)
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	go link.supervise(ctx, func(ctx context.Context, enrollment settings.Enrollment, connected func()) error {
		sessions <- enrollment.NodeID
		if enrollment.NodeID == "failing" {
			return errors.New("the panel refused the session")
		}
		connected()
		<-ctx.Done()
		return ctx.Err()
	}, func(string, string, ...any) {})
	waitFor := func(want string, state ConnectionState) Status {
		t.Helper()
		select {
		case got := <-sessions:
			if got != want {
				t.Fatalf("session of %q, want %q", got, want)
			}
		case <-time.After(5 * time.Second):
			t.Fatalf("no session of %q", want)
		}
		for deadline := time.Now().Add(5 * time.Second); ; time.Sleep(time.Millisecond) {
			status := link.Status()
			if status.State == state {
				return status
			}
			if time.Now().After(deadline) {
				t.Fatalf("status = %+v, want %s", status, state)
			}
		}
	}
	if status := waitFor("first", ConnectionConnected); status.Enrollment.NodeID != "first" || status.LastError != "" {
		t.Fatalf("status = %+v", status)
	}

	failing := enrollment("failing")
	link.replace(&failing)
	if status := waitFor("failing", ConnectionDisconnected); status.LastError == "" || status.RetryAt == nil {
		t.Fatalf("a failed session left %+v", status)
	}
	second := enrollment("second")
	link.replace(&second)
	waitFor("second", ConnectionConnected)

	status, err := link.Unenroll()
	if err != nil || status.State != ConnectionUnenrolled || status.Enrollment != nil {
		t.Fatalf("unenroll = %+v, %v", status, err)
	}
	if _, err := settings.LoadEnrollment(dataDir); !errors.Is(err, settings.ErrNotEnrolled) {
		t.Fatalf("the enrollment outlived unenrolling: %v", err)
	}
	select {
	case got := <-sessions:
		t.Fatalf("an unenrolled node opened a session of %q", got)
	case <-time.After(50 * time.Millisecond):
	}
}

// TestRunningNodeSwitchesToAnEnrollmentSavedWhileItRuns saves a new
// enrollment the way sing-box-node enroll does while the node holds a
// session: the node starts over with it, and keeps it when the file is
// later removed or unreadable.
func TestRunningNodeSwitchesToAnEnrollmentSavedWhileItRuns(t *testing.T) {
	dataDir := t.TempDir()
	enrollment := func(nodeID string) settings.Enrollment {
		_, key, err := ed25519.GenerateKey(rand.Reader)
		if err != nil {
			t.Fatal(err)
		}
		return settings.Enrollment{Endpoint: "panel.example:443", PanelID: "panel", NodeID: nodeID, Key: key}
	}
	if err := settings.SaveEnrollment(dataDir, enrollment("first")); err != nil {
		t.Fatal(err)
	}
	link, err := OpenLink(dataDir, buildinfo.Info{})
	if err != nil {
		t.Fatal(err)
	}
	sessions := make(chan string, 8)
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	go link.supervise(ctx, func(ctx context.Context, enrollment settings.Enrollment, connected func()) error {
		sessions <- enrollment.NodeID
		connected()
		<-ctx.Done()
		return ctx.Err()
	}, func(string, string, ...any) {})
	go link.watch(ctx, 10*time.Millisecond, func(string, string, ...any) {})
	next := func(want string) {
		t.Helper()
		select {
		case got := <-sessions:
			if got != want {
				t.Fatalf("session of %q, want %q", got, want)
			}
		case <-time.After(5 * time.Second):
			t.Fatalf("no session of %q", want)
		}
	}
	next("first")
	if err := settings.SaveEnrollment(dataDir, enrollment("second")); err != nil {
		t.Fatal(err)
	}
	next("second")
	if status := link.Status(); status.Enrollment == nil || status.Enrollment.NodeID != "second" {
		t.Fatalf("status after the saved enrollment = %+v", status)
	}
	// An unchanged, missing or unreadable file leaves the session alone.
	if link.reload(func(string, string, ...any) {}) {
		t.Fatal("an unchanged enrollment started the session over")
	}
	if err := settings.RemoveEnrollment(dataDir); err != nil {
		t.Fatal(err)
	}
	if link.reload(func(string, string, ...any) {}) || !link.Enrolled() {
		t.Fatal("a removed enrollment file ended the running enrollment")
	}
	select {
	case got := <-sessions:
		t.Fatalf("a session of %q started without a new enrollment", got)
	case <-time.After(50 * time.Millisecond):
	}
}
