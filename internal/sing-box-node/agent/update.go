// SPDX-License-Identifier: GPL-3.0-or-later

package agent

import (
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"

	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/rehuony/sing-box-panel/internal/buildinfo"
	"github.com/rehuony/sing-box-panel/internal/nodeapi"
	releasecontract "github.com/rehuony/sing-box-panel/internal/release"
	"github.com/rehuony/sing-box-panel/internal/selfupdate"
	"github.com/rehuony/sing-box-panel/internal/sing-box-node/settings"
	nodesystemd "github.com/rehuony/sing-box-panel/internal/sing-box-node/systemd"
	"github.com/rehuony/sing-box-panel/internal/systemd"
)

// UpdateOptions adjusts how the node updates itself. The zero value verifies
// releases with the key embedded at build time, replaces the running
// executable and restarts the node's systemd unit with systemctl.
type UpdateOptions struct {
	PublicKey  ed25519.PublicKey
	Executable func() (string, error)
	// Restart queues a restart of the node's unit in scope.
	Restart func(context.Context, systemd.Scope) error
}

// updates is how a running node updates itself, and its last attempt.
type updates struct {
	options UpdateOptions
	dataDir string
	build   buildinfo.Info

	mu   sync.Mutex
	last *selfupdate.Attempt
	// applying admits one update at a time.
	applying sync.Mutex
}

// errUpdateInProgress refuses an update while another one runs.
var errUpdateInProgress = errors.New("another update of this node is in progress")

func newUpdates(dataDir string, build buildinfo.Info, options UpdateOptions) *updates {
	return &updates{options: options, dataDir: dataDir, build: build}
}

func (current *updates) dir() string { return filepath.Join(current.dataDir, selfupdate.UpdatesDir) }

// Updater returns the node's updater.
func (options UpdateOptions) updater() *selfupdate.Updater {
	return selfupdate.New(selfupdate.Options{Binary: selfupdate.Node, PublicKey: options.PublicKey, Executable: options.Executable})
}

// load reads the last attempt, which the root update helper may have made
// while the node was stopped, and removes finished staging.
func (current *updates) load() {
	attempt, _, err := selfupdate.TakeAttempt(current.dir())
	if err == nil {
		current.mu.Lock()
		current.last = attempt
		current.mu.Unlock()
	}
	_ = selfupdate.CleanUpdates(current.dir())
}

// watch takes each result the root update helper writes while the node
// runs, such as its refusal of a staged release, and withdraws a staged
// release the helper never took, every interval until ctx ends; the next
// state report carries the attempt.
func (current *updates) watch(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			attempt, err := selfupdate.TakeFreshAttempt(current.dir())
			if err == nil && attempt == nil {
				attempt, err = selfupdate.WithdrawUnclaimed(current.dir(), time.Now())
			}
			if err == nil && attempt != nil {
				current.mu.Lock()
				current.last = attempt
				current.mu.Unlock()
			}
		}
	}
}

// report returns the last attempt for a state report.
func (current *updates) report() *nodeapi.UpdateAttempt {
	current.mu.Lock()
	defer current.mu.Unlock()
	if current.last == nil {
		return nil
	}
	return &nodeapi.UpdateAttempt{
		Version: current.last.Version, Succeeded: current.last.Succeeded, Code: current.last.Code,
		FinishedAt: timestamppb.New(current.last.FinishedAt),
	}
}

func (current *updates) record(version string, err error) {
	attempt := selfupdate.Attempt{Version: version, Succeeded: err == nil, FinishedAt: time.Now().UTC()}
	if err != nil {
		attempt.Code = selfupdate.FailureCode(err)
	}
	current.mu.Lock()
	current.last = &attempt
	current.mu.Unlock()
	_ = selfupdate.RecordAttempt(current.dir(), attempt)
}

// releaseFetcher receives the panel's release of version into dir.
type releaseFetcher func(ctx context.Context, version, dir string) error

// apply updates the node to version: it fetches the release, verifies it
// with the node's own key and installs it, or stages it for the root update
// helper of a system service. restart, when set, restarts the node's unit
// into the new version and must run after the panel has the result.
func (current *updates) apply(ctx context.Context, version string, fetch releaseFetcher) (state nodeapi.UpdateState, restart func(), err error) {
	if version == current.build.Version {
		return nodeapi.UpdateState_UPDATE_STATE_UP_TO_DATE, nil, nil
	}
	// A second update, from a double click or beside a batch, is refused
	// rather than racing the first on the same staged release.
	if !current.applying.TryLock() {
		return 0, nil, errUpdateInProgress
	}
	defer current.applying.Unlock()
	updater := current.options.updater()
	if err := updater.Ready(current.build.Version); err != nil {
		return 0, nil, err
	}
	scope, supervised := systemd.RunningScope()
	defer func() {
		// The helper reports a staged release's outcome.
		if state != nodeapi.UpdateState_UPDATE_STATE_STAGED {
			current.record(version, err)
		}
	}()
	incoming, err := selfupdate.IncomingDir(current.dir())
	if err != nil {
		return 0, nil, err
	}
	defer func() { _ = os.RemoveAll(incoming) }()
	if err := fetch(ctx, version, incoming); err != nil {
		return 0, nil, err
	}
	staged, err := updater.Verify(incoming, version, current.build.Version)
	if err != nil {
		return 0, nil, err
	}
	delivery, err := updater.Deliver(ctx, staged, current.dir(), supervised && scope == systemd.ScopeSystem)
	switch {
	case err != nil:
		return 0, nil, err
	case delivery == selfupdate.DeliveryStaged:
		return nodeapi.UpdateState_UPDATE_STATE_STAGED, nil, nil
	case !supervised:
		return nodeapi.UpdateState_UPDATE_STATE_RESTART_REQUIRED, nil, nil
	}
	return nodeapi.UpdateState_UPDATE_STATE_RESTARTING, func() {
		// Sending only hands the result to the transport; give it time to
		// reach the panel before systemd stops the node.
		time.Sleep(restartGrace)
		restartCtx, cancel := context.WithTimeout(context.Background(), time.Minute)
		defer cancel()
		_ = current.restart(restartCtx, scope)
	}, nil
}

// restartGrace separates the result of an update from the restart into it.
const restartGrace = 2 * time.Second

func (current *updates) restart(ctx context.Context, scope systemd.Scope) error {
	if current.options.Restart != nil {
		return current.options.Restart(ctx, scope)
	}
	manager, err := nodesystemd.New(systemd.Options{})
	if err != nil {
		return err
	}
	return manager.RequestRestart(ctx, scope)
}

// updateFailure is the problem an UpdateNode command answers with.
func updateFailure(err error) nodeapi.ResultOutcome {
	if errors.Is(err, errUpdateInProgress) {
		return problem("update_in_progress", err.Error())
	}
	return problem("update_"+selfupdate.FailureCode(err), err.Error())
}

// FetchRelease receives the panel's node release from its release endpoint
// with the node's enrollment into dir: the binary for this platform, the
// release's checksum manifest and its signature. want, when set, is the
// version the panel must send. It returns the release's version; the caller
// verifies the files with its own key.
func FetchRelease(ctx context.Context, enrollment settings.Enrollment, build buildinfo.Info, want, dir string) (string, error) {
	client, err := nodeapi.NewClient(enrollment.URL)
	if err != nil {
		return "", err
	}
	defer client.Close()
	return fetchRelease(ctx, client, enrollment, build, want, dir)
}

func fetchRelease(ctx context.Context, client *nodeapi.Client, enrollment settings.Enrollment, build buildinfo.Info, want, dir string) (string, error) {
	stream, err := client.FetchRelease(ctx)
	if err != nil {
		return "", releaseUnavailable(err)
	}
	defer stream.Close(nil)
	if err := stream.Send(&nodeapi.FetchReleaseRequest{Frame: &nodeapi.FetchReleaseRequest_Hello{Hello: &nodeapi.Hello{
		NodeId: enrollment.NodeID, Protocol: protocol(), BuildVersion: build.Version, Platform: platform(),
	}}}); err != nil {
		return "", releaseUnavailable(err)
	}
	frame, err := stream.RecvWithin(handshakeTimeout)
	if err != nil {
		return "", releaseUnavailable(err)
	}
	challenge := frame.GetChallenge()
	if challenge == nil || challenge.GetPanelId() != enrollment.PanelID {
		return "", fmt.Errorf("%w: the panel did not identify as the enrolled panel", selfupdate.ErrReleaseUnavailable)
	}
	if err := stream.Send(&nodeapi.FetchReleaseRequest{Frame: &nodeapi.FetchReleaseRequest_Proof{
		Proof: nodeapi.SignReleaseProof(enrollment.Key, enrollment.PanelID, enrollment.NodeID, challenge.GetNonce()),
	}}); err != nil {
		return "", releaseUnavailable(err)
	}
	frame, err = stream.RecvWithin(handshakeTimeout)
	if err != nil {
		return "", releaseUnavailable(err)
	}
	header := frame.GetHeader()
	digest, digestErr := hex.DecodeString(header.GetSha256())
	switch {
	case header == nil:
		return "", fmt.Errorf("%w: the panel sent no release header", selfupdate.ErrReleaseInvalid)
	case releasecontract.ValidateVersion(header.GetVersion()) != nil, want != "" && header.GetVersion() != want:
		return "", fmt.Errorf("%w: the panel sent release %q", selfupdate.ErrReleaseInvalid, header.GetVersion())
	case header.GetSize() <= 0 || header.GetSize() > selfupdate.MaxBinaryBytes, digestErr != nil || len(digest) != sha256.Size,
		len(header.GetChecksums()) > selfupdate.MaxChecksumBytes, len(header.GetSignature()) > selfupdate.MaxSignatureBytes:
		return "", fmt.Errorf("%w: the release header is malformed", selfupdate.ErrReleaseInvalid)
	}
	for name, data := range map[string][]byte{selfupdate.ChecksumsName: header.GetChecksums(), selfupdate.SignatureName: header.GetSignature()} {
		if err := os.WriteFile(filepath.Join(dir, name), data, 0o600); err != nil {
			return "", err
		}
	}
	file, err := os.OpenFile(filepath.Join(dir, selfupdate.Node.AssetName(platform().GetOs(), platform().GetArch())), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return "", err
	}
	defer file.Close()
	hash := sha256.New()
	var written int64
	for {
		frame, err := stream.RecvWithin(transferIdleTimeout)
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return "", releaseUnavailable(err)
		}
		chunk := frame.GetChunk()
		if written += int64(len(chunk)); written > header.GetSize() {
			return "", fmt.Errorf("%w: the binary is larger than its header says", selfupdate.ErrReleaseInvalid)
		}
		if _, err := io.MultiWriter(file, hash).Write(chunk); err != nil {
			return "", err
		}
	}
	if written != header.GetSize() || hex.EncodeToString(hash.Sum(nil)) != header.GetSha256() {
		return "", fmt.Errorf("%w: the binary does not match its header", selfupdate.ErrChecksumInvalid)
	}
	if err := file.Sync(); err != nil {
		return "", err
	}
	return header.GetVersion(), file.Close()
}

func releaseUnavailable(err error) error {
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return err
	}
	return fmt.Errorf("%w: %s", selfupdate.ErrReleaseUnavailable, describe(err))
}
