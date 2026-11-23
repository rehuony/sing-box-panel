// SPDX-License-Identifier: GPL-3.0-or-later

package agent

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	goruntime "runtime"
	"testing"
	"time"

	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/rehuony/sing-box-panel/internal/buildinfo"
	"github.com/rehuony/sing-box-panel/internal/nodeapi"
	"github.com/rehuony/sing-box-panel/internal/release"
	"github.com/rehuony/sing-box-panel/internal/selfupdate"
	"github.com/rehuony/sing-box-panel/internal/sing-box-node/store"
)

func updateCommand(requestID, version string, deadline time.Duration) *nodeapi.Command {
	return &nodeapi.Command{
		RequestId: requestID, Deadline: timestamppb.New(time.Now().Add(deadline)),
		Body: &nodeapi.Command_UpdateNode{UpdateNode: &nodeapi.UpdateNode{Version: version}},
	}
}

// TestCommandsWaitForAFreeSlotUntilTheirDeadline runs commands while every
// slot is taken: one waits for a slot that frees in time, and one whose
// deadline passes first is refused as busy.
func TestCommandsWaitForAFreeSlotUntilTheirDeadline(t *testing.T) {
	database, err := store.Open(t.Context(), filepath.Join(t.TempDir(), "node.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	// A node that cannot update answers at once once it has a slot.
	commands := &executor{database: database, now: time.Now, slots: make(chan struct{}, maximumCommands)}
	for range maximumCommands {
		commands.slots <- struct{}{}
	}
	if result := commands.execute(t.Context(), updateCommand("request_busy", "v1.1.0", 50*time.Millisecond)); result.GetProblem().GetCode() != "node_busy" {
		t.Fatalf("a command past its deadline without a slot = %v, want node_busy", result)
	}
	go func() {
		time.Sleep(50 * time.Millisecond)
		<-commands.slots
	}()
	if result := commands.execute(t.Context(), updateCommand("request_waits", "v1.1.0", 5*time.Second)); result.GetProblem().GetCode() != "update_unsupported" {
		t.Fatalf("a command whose slot freed in time = %v, want it to run", result)
	}
}

// TestUpdateCommandAnswersHowItLeftTheNode covers the update command's own
// answers: up to date, refused while another update runs, and, where
// releases exist, a release the node's key does not verify.
func TestUpdateCommandAnswersHowItLeftTheNode(t *testing.T) {
	t.Setenv("SING_BOX_SYSTEMD_SCOPE", "")
	trusted, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	_, untrusted, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	executable := filepath.Join(t.TempDir(), "sing-box-node")
	if err := os.WriteFile(executable, []byte("old binary"), 0o755); err != nil {
		t.Fatal(err)
	}
	current := newUpdates(t.TempDir(), buildinfo.Info{Version: "v1.0.0"}, UpdateOptions{
		PublicKey: trusted, Executable: func() (string, error) { return executable, nil },
	})
	fetched := 0
	commands := &executor{updates: current, fetchRelease: func(_ context.Context, version, dir string) error {
		fetched++
		// A release signed with a key the node does not trust.
		name := selfupdate.Node.AssetName(goruntime.GOOS, goruntime.GOARCH)
		binary := []byte("new binary")
		checksums := []byte(fmt.Sprintf("%x  %s\n", sha256.Sum256(binary), name))
		signature, err := release.Sign(untrusted, version, checksums)
		if err != nil {
			return err
		}
		for file, data := range map[string][]byte{selfupdate.ChecksumsName: checksums, selfupdate.SignatureName: signature, name: binary} {
			if err := os.WriteFile(filepath.Join(dir, file), data, 0o600); err != nil {
				return err
			}
		}
		return nil
	}}
	if outcome := commands.updateNode(t.Context(), "request_current", &nodeapi.UpdateNode{Version: "v1.0.0"}); outcome.(*nodeapi.Result_Update).Update.GetState() != nodeapi.UpdateState_UPDATE_STATE_UP_TO_DATE || fetched != 0 {
		t.Fatalf("an update to the running version = %v after %d fetches, want up to date", outcome, fetched)
	}
	current.applying.Lock()
	outcome := commands.updateNode(t.Context(), "request_second", &nodeapi.UpdateNode{Version: "v1.1.0"})
	current.applying.Unlock()
	if problem, ok := outcome.(*nodeapi.Result_Problem); !ok || problem.Problem.GetCode() != "update_in_progress" || fetched != 0 {
		t.Fatalf("an update beside a running one = %v after %d fetches, want update_in_progress", outcome, fetched)
	}
	if !selfupdate.SupportedPlatform(goruntime.GOOS, goruntime.GOARCH) {
		t.Skip("releases carry binaries only for linux/amd64 and linux/arm64")
	}
	outcome = commands.updateNode(t.Context(), "request_untrusted", &nodeapi.UpdateNode{Version: "v1.1.0"})
	if problem, ok := outcome.(*nodeapi.Result_Problem); !ok || problem.Problem.GetCode() != "update_signature_invalid" {
		t.Fatalf("an untrusted release = %v, want update_signature_invalid", outcome)
	}
	if data, err := os.ReadFile(executable); err != nil || string(data) != "old binary" {
		t.Fatalf("executable = %q, %v; want it untouched", data, err)
	}
	if last := current.report(); last == nil || last.GetSucceeded() || last.GetVersion() != "v1.1.0" || last.GetCode() != "signature_invalid" {
		t.Fatalf("reported attempt = %v, want the refused release", last)
	}
	if recorded, fresh, err := selfupdate.TakeAttempt(current.dir()); err != nil || fresh || recorded == nil || recorded.Code != "signature_invalid" {
		t.Fatalf("recorded attempt = %+v (fresh %v), %v", recorded, fresh, err)
	}
}
