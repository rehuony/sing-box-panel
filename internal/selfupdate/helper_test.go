// SPDX-License-Identifier: GPL-3.0-or-later

//go:build darwin || linux

package selfupdate

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	releasecontract "github.com/rehuony/sing-box-panel/internal/release"
)

// helperFixture is a node's system installation: an installed executable
// and a data directory whose updates directory holds a staged release and
// its request.
type helperFixture struct {
	target   string
	dataDir  string
	updates  string
	private  ed25519.PrivateKey
	public   ed25519.PublicKey
	restarts int
}

func newHelperFixture(t *testing.T) *helperFixture {
	t.Helper()
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	fixture := &helperFixture{
		target: filepath.Join(root, "bin", "sing-box-node"), dataDir: filepath.Join(root, "data"),
	}
	fixture.updates = filepath.Join(fixture.dataDir, UpdatesDir)
	fixture.public, fixture.private, err = ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	for _, dir := range []string{filepath.Dir(fixture.target), fixture.updates} {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(fixture.target, []byte("old binary"), 0o755); err != nil {
		t.Fatal(err)
	}
	return fixture
}

// stage writes a release of version signed with key, holding binary, and
// requests it.
func (fixture *helperFixture) stage(t *testing.T, version string, binary []byte, key ed25519.PrivateKey) string {
	t.Helper()
	dir := filepath.Join(fixture.updates, version)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	name := Node.AssetName("linux", "amd64")
	checksums := []byte(fmt.Sprintf("%x  %s\n", sha256.Sum256(binary), name))
	signature, err := releasecontract.Sign(key, version, checksums)
	if err != nil {
		t.Fatal(err)
	}
	for file, data := range map[string][]byte{ChecksumsName: checksums, SignatureName: signature, name: binary} {
		if err := os.WriteFile(filepath.Join(dir, file), data, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(fixture.updates, RequestName), []byte(version+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	return dir
}

func (fixture *helperFixture) updater() *Updater {
	updater := New(Options{
		Binary: Node, GOOS: "linux", GOARCH: "amd64", PublicKey: fixture.public,
		Executable: func() (string, error) { return fixture.target, nil },
	})
	updater.validateStagedExecutable = func(string, Binary, string, string) error { return nil }
	return updater
}

func (fixture *helperFixture) apply(t *testing.T, updater *Updater, current string, restart func(context.Context) error) (*Attempt, error) {
	t.Helper()
	if restart == nil {
		restart = func(context.Context) error { return nil }
	}
	return updater.ApplyStaged(t.Context(), HelperOptions{
		DataDir: fixture.dataDir, Current: current,
		Restart: func(ctx context.Context) error { fixture.restarts++; return restart(ctx) },
	})
}

func (fixture *helperFixture) requireExecutable(t *testing.T, want string) {
	t.Helper()
	data, err := os.ReadFile(fixture.target)
	if err != nil || string(data) != want {
		t.Fatalf("executable = %q, %v; want %q", data, err, want)
	}
}

func TestApplyStagedInstallsRestartsAndRecordsTheResult(t *testing.T) {
	fixture := newHelperFixture(t)
	fixture.stage(t, "v1.3.0", []byte("new binary"), fixture.private)

	// The service that starts reads the result.
	var seen *Attempt
	attempt, err := fixture.apply(t, fixture.updater(), "v1.2.0", func(context.Context) error {
		seen, _ = readAttempt(filepath.Join(fixture.updates, resultName))
		return nil
	})
	if err != nil || attempt == nil || !attempt.Succeeded || attempt.Version != "v1.3.0" || fixture.restarts != 1 {
		t.Fatalf("attempt = %+v, %v after %d restarts", attempt, err, fixture.restarts)
	}
	if seen == nil || !seen.Succeeded || seen.Version != "v1.3.0" {
		t.Fatalf("the restarted service read %+v", seen)
	}
	fixture.requireExecutable(t, "new binary")
	if previous, err := os.ReadFile(fixture.target + PreviousSuffix); err != nil || string(previous) != "old binary" {
		t.Fatalf("previous executable = %q, %v", previous, err)
	}
	if _, err := os.Lstat(filepath.Join(fixture.updates, RequestName)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("the request remains: %v", err)
	}
	read, fresh, err := TakeAttempt(fixture.updates)
	if err != nil || !fresh || read == nil || *read != *attempt {
		t.Fatalf("service reads %+v (fresh %v), %v; want %+v", read, fresh, err, attempt)
	}
	if again, fresh, err := TakeAttempt(fixture.updates); err != nil || fresh || again == nil || *again != *attempt {
		t.Fatalf("second read = %+v (fresh %v), %v; want the same attempt, not fresh", again, fresh, err)
	}
	if attempt, err := fixture.apply(t, fixture.updater(), "v1.3.0", nil); attempt != nil || err != nil {
		t.Fatalf("without a request = %+v, %v; want nothing done", attempt, err)
	}
}

func TestApplyStagedRefusesUntrustedReleases(t *testing.T) {
	_, otherKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	selfBinary, err := os.ReadFile(self)
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name    string
		current string
		prepare func(*testing.T, *helperFixture)
		real    bool
		code    string
	}{
		{name: "tampered binary", current: "v1.2.0", code: "checksum_invalid", prepare: func(t *testing.T, fixture *helperFixture) {
			dir := fixture.stage(t, "v1.3.0", []byte("new binary"), fixture.private)
			if err := os.WriteFile(filepath.Join(dir, Node.AssetName("linux", "amd64")), []byte("evil binary"), 0o600); err != nil {
				t.Fatal(err)
			}
		}},
		{name: "wrong signature", current: "v1.2.0", code: "signature_invalid", prepare: func(t *testing.T, fixture *helperFixture) {
			fixture.stage(t, "v1.3.0", []byte("new binary"), otherKey)
		}},
		{name: "lower version", current: "v1.4.0", code: "version_not_newer", prepare: func(t *testing.T, fixture *helperFixture) {
			fixture.stage(t, "v1.3.0", []byte("new binary"), fixture.private)
		}},
		{name: "equal version", current: "v1.3.0", code: "up_to_date", prepare: func(t *testing.T, fixture *helperFixture) {
			fixture.stage(t, "v1.3.0", []byte("new binary"), fixture.private)
		}},
		{name: "symbolic link to a binary", current: "v1.2.0", code: "staged_file_invalid", prepare: func(t *testing.T, fixture *helperFixture) {
			dir := fixture.stage(t, "v1.3.0", []byte("new binary"), fixture.private)
			name := filepath.Join(dir, Node.AssetName("linux", "amd64"))
			outside := filepath.Join(filepath.Dir(fixture.dataDir), "outside")
			if err := os.Rename(name, outside); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(outside, name); err != nil {
				t.Fatal(err)
			}
		}},
		{name: "symbolic link to a release directory", current: "v1.2.0", code: "staged_file_invalid", prepare: func(t *testing.T, fixture *helperFixture) {
			dir := fixture.stage(t, "v1.3.0", []byte("new binary"), fixture.private)
			outside := filepath.Join(filepath.Dir(fixture.dataDir), "release")
			if err := os.Rename(dir, outside); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(outside, dir); err != nil {
				t.Fatal(err)
			}
		}},
		{name: "symbolic link request", current: "v1.2.0", code: "staged_file_invalid", prepare: func(t *testing.T, fixture *helperFixture) {
			fixture.stage(t, "v1.3.0", []byte("new binary"), fixture.private)
			request := filepath.Join(fixture.updates, RequestName)
			outside := filepath.Join(filepath.Dir(fixture.dataDir), "request")
			if err := os.Rename(request, outside); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(outside, request); err != nil {
				t.Fatal(err)
			}
		}},
		{name: "request naming a path", current: "v1.2.0", code: "release_invalid", prepare: func(t *testing.T, fixture *helperFixture) {
			fixture.stage(t, "v1.3.0", []byte("new binary"), fixture.private)
			if err := os.WriteFile(filepath.Join(fixture.updates, RequestName), []byte("../v1.3.0\n"), 0o600); err != nil {
				t.Fatal(err)
			}
		}},
		{name: "another binary", current: "v1.2.0", real: true, code: "identity_invalid", prepare: func(t *testing.T, fixture *helperFixture) {
			// A signed Go program that is not cmd/sing-box-node: this test binary.
			fixture.stage(t, "v1.3.0", selfBinary, fixture.private)
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			fixture := newHelperFixture(t)
			test.prepare(t, fixture)
			updater := fixture.updater()
			if test.real {
				updater.validateStagedExecutable = validateStagedExecutable
			}
			attempt, err := fixture.apply(t, updater, test.current, nil)
			if err == nil || attempt == nil || attempt.Succeeded || attempt.Code != test.code {
				t.Fatalf("attempt = %+v, %v; want the code %s", attempt, err, test.code)
			}
			fixture.requireExecutable(t, "old binary")
			if fixture.restarts != 0 {
				t.Fatalf("the service restarted %d times for a refused release", fixture.restarts)
			}
			if _, err := os.Lstat(filepath.Join(fixture.updates, RequestName)); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("the request remains after a refused release: %v", err)
			}
			if read, fresh, err := TakeAttempt(fixture.updates); err != nil || !fresh || read == nil || read.Code != test.code {
				t.Fatalf("recorded attempt = %+v (fresh %v), %v", read, fresh, err)
			}
		})
	}
}

func TestApplyStagedRestoresThePreviousExecutableWhenTheServiceFails(t *testing.T) {
	fixture := newHelperFixture(t)
	fixture.stage(t, "v1.3.0", []byte("new binary"), fixture.private)
	var running []string
	var seen []string
	attempt, err := fixture.apply(t, fixture.updater(), "v1.2.0", func(context.Context) error {
		data, readErr := os.ReadFile(fixture.target)
		if readErr != nil {
			return readErr
		}
		running = append(running, string(data))
		if result, err := readAttempt(filepath.Join(fixture.updates, resultName)); err == nil {
			seen = append(seen, result.Code)
		}
		if string(data) == "new binary" {
			return fmt.Errorf("%w: activating (auto-restart)", ErrNotActive)
		}
		return nil
	})
	if !errors.Is(err, ErrNotActive) || attempt == nil || attempt.Code != "service_not_active" {
		t.Fatalf("attempt = %+v, %v; want service_not_active", attempt, err)
	}
	if len(running) != 2 || running[0] != "new binary" || running[1] != "old binary" {
		t.Fatalf("restarted into %q; want the new version, then the previous one", running)
	}
	// The previous version, started again, reads why the update failed.
	if len(seen) != 2 || seen[0] != "" || seen[1] != "service_not_active" {
		t.Fatalf("the restarted services read %q", seen)
	}
	fixture.requireExecutable(t, "old binary")
}

func TestStagedReleasesAreHandedToTheHelperAndCleanedUp(t *testing.T) {
	fixture := newHelperFixture(t)
	incoming, err := IncomingDir(fixture.updates)
	if err != nil {
		t.Fatal(err)
	}
	name := Node.AssetName("linux", "amd64")
	binary := []byte("new binary")
	checksums := []byte(fmt.Sprintf("%x  %s\n", sha256.Sum256(binary), name))
	signature, err := releasecontract.Sign(fixture.private, "v1.3.0", checksums)
	if err != nil {
		t.Fatal(err)
	}
	for file, data := range map[string][]byte{ChecksumsName: checksums, SignatureName: signature, name: binary} {
		if err := os.WriteFile(filepath.Join(incoming, file), data, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	staged, err := fixture.updater().Verify(incoming, "v1.3.0", "v1.2.0")
	if err != nil {
		t.Fatal(err)
	}
	if err := StageRelease(staged, fixture.updates); err != nil {
		t.Fatal(err)
	}
	stale, err := IncomingDir(fixture.updates)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(fixture.updates, "v1.1.0"), 0o700); err != nil {
		t.Fatal(err)
	}
	// A service that starts while its request is pending keeps the release.
	if err := CleanUpdates(fixture.updates); err != nil {
		t.Fatal(err)
	}
	for path, kept := range map[string]bool{
		filepath.Join(fixture.updates, "v1.3.0", name): true, stale: false, filepath.Join(fixture.updates, "v1.1.0"): false,
	} {
		if _, err := os.Lstat(path); (err == nil) != kept {
			t.Fatalf("%s kept = %v, want %v", path, err == nil, kept)
		}
	}
	attempt, err := fixture.apply(t, fixture.updater(), "v1.2.0", nil)
	if err != nil || !attempt.Succeeded {
		t.Fatalf("attempt = %+v, %v", attempt, err)
	}
	if err := CleanUpdates(fixture.updates); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(filepath.Join(fixture.updates, "v1.3.0")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("an applied release remains: %v", err)
	}
}

// TestUnclaimedStagedReleaseIsWithdrawn stages a release whose helper never
// starts: after HelperTimeout the service withdraws it and records why.
func TestUnclaimedStagedReleaseIsWithdrawn(t *testing.T) {
	fixture := newHelperFixture(t)
	staged := fixture.stage(t, "v1.3.0", []byte("new binary"), fixture.private)
	request := filepath.Join(fixture.updates, RequestName)
	written := time.Now()
	if err := os.Chtimes(request, written, written); err != nil {
		t.Fatal(err)
	}
	if attempt, err := WithdrawUnclaimed(fixture.updates, written.Add(HelperTimeout-time.Second)); attempt != nil || err != nil {
		t.Fatalf("a young request = %+v, %v; want it left for the helper", attempt, err)
	}
	attempt, err := WithdrawUnclaimed(fixture.updates, written.Add(HelperTimeout))
	if err != nil || attempt == nil || attempt.Succeeded || attempt.Version != "v1.3.0" || attempt.Code != "helper_unavailable" {
		t.Fatalf("an unclaimed request = %+v, %v; want a helper_unavailable attempt", attempt, err)
	}
	for _, path := range []string{request, staged} {
		if _, err := os.Lstat(path); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("%s remains: %v", path, err)
		}
	}
	if last, fresh, err := TakeAttempt(fixture.updates); err != nil || fresh || last == nil || *last != *attempt {
		t.Fatalf("the service reads %+v (fresh %v), %v; want the recorded attempt", last, fresh, err)
	}
	// Without a request the helper is working, or nothing was staged.
	if attempt, err := WithdrawUnclaimed(fixture.updates, written.Add(time.Hour)); attempt != nil || err != nil {
		t.Fatalf("no request = %+v, %v", attempt, err)
	}
}
