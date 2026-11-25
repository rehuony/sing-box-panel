// SPDX-License-Identifier: GPL-3.0-or-later

//go:build darwin || linux

package selfupdate

import (
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
	target  string
	dataDir string
	updates string
	private ed25519.PrivateKey
	public  ed25519.PublicKey
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

func (fixture *helperFixture) apply(t *testing.T, updater *Updater, current string) (*Attempt, error) {
	t.Helper()
	return updater.ApplyStaged(t.Context(), HelperOptions{DataDir: fixture.dataDir, Current: current})
}

func (fixture *helperFixture) requireExecutable(t *testing.T, want string) {
	t.Helper()
	data, err := os.ReadFile(fixture.target)
	if err != nil || string(data) != want {
		t.Fatalf("executable = %q, %v; want %q", data, err, want)
	}
}

// TestApplyStagedInstallsOneWayAndRecordsTheResult installs a staged
// release: the executable's directory then holds the executable alone,
// without the replaced binary, a lock file or what earlier updates left.
func TestApplyStagedInstallsOneWayAndRecordsTheResult(t *testing.T) {
	fixture := newHelperFixture(t)
	fixture.stage(t, "v1.3.0", []byte("new binary"), fixture.private)
	bin := filepath.Dir(fixture.target)
	stale := filepath.Join(bin, ".sing-box-node-update-1")
	for _, leftover := range []string{fixture.target + ".previous", filepath.Join(bin, ".sing-box-node.update.lock"), stale} {
		if err := os.WriteFile(leftover, []byte("left"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	old := time.Now().Add(-2 * staleTemporaryAge)
	if err := os.Chtimes(stale, old, old); err != nil {
		t.Fatal(err)
	}

	attempt, err := fixture.apply(t, fixture.updater(), "v1.2.0")
	if err != nil || attempt == nil || !attempt.Succeeded || attempt.Version != "v1.3.0" {
		t.Fatalf("attempt = %+v, %v", attempt, err)
	}
	fixture.requireExecutable(t, "new binary")
	entries, err := os.ReadDir(bin)
	if err != nil || len(entries) != 1 || entries[0].Name() != filepath.Base(fixture.target) {
		names := []string{}
		for _, entry := range entries {
			names = append(names, entry.Name())
		}
		t.Fatalf("beside the executable: %q, %v; want the executable alone", names, err)
	}
	if _, err := os.Lstat(filepath.Join(fixture.updates, RequestName)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("the request remains: %v", err)
	}
	// The service that starts next reads the result once as fresh.
	read, fresh, err := TakeAttempt(fixture.updates)
	if err != nil || !fresh || read == nil || *read != *attempt {
		t.Fatalf("service reads %+v (fresh %v), %v; want %+v", read, fresh, err, attempt)
	}
	if again, fresh, err := TakeAttempt(fixture.updates); err != nil || fresh || again == nil || *again != *attempt {
		t.Fatalf("second read = %+v (fresh %v), %v; want the same attempt, not fresh", again, fresh, err)
	}
	if attempt, err := fixture.apply(t, fixture.updater(), "v1.3.0"); attempt != nil || err != nil {
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
			attempt, err := fixture.apply(t, updater, test.current)
			if err == nil || attempt == nil || attempt.Succeeded || attempt.Code != test.code {
				t.Fatalf("attempt = %+v, %v; want the code %s", attempt, err, test.code)
			}
			fixture.requireExecutable(t, "old binary")
			if _, err := os.Lstat(filepath.Join(fixture.updates, RequestName)); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("the request remains after a refused release: %v", err)
			}
			if read, fresh, err := TakeAttempt(fixture.updates); err != nil || !fresh || read == nil || read.Code != test.code {
				t.Fatalf("recorded attempt = %+v (fresh %v), %v", read, fresh, err)
			}
		})
	}
}

func TestStagedReleasesAreHandedToTheInstallStepAndCleanedUp(t *testing.T) {
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
	attempt, err := fixture.apply(t, fixture.updater(), "v1.2.0")
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

// TestUnappliedStagedReleaseIsWithdrawn starts a service whose unit has no
// install step: the request is still there, so the service withdraws the
// release and records why.
func TestUnappliedStagedReleaseIsWithdrawn(t *testing.T) {
	fixture := newHelperFixture(t)
	staged := fixture.stage(t, "v1.3.0", []byte("new binary"), fixture.private)
	now := time.Now()
	attempt, err := WithdrawUnapplied(fixture.updates, now)
	if err != nil || attempt == nil || attempt.Succeeded || attempt.Version != "v1.3.0" || attempt.Code != "install_step_missing" {
		t.Fatalf("an unapplied request = %+v, %v; want an install_step_missing attempt", attempt, err)
	}
	for _, path := range []string{filepath.Join(fixture.updates, RequestName), staged} {
		if _, err := os.Lstat(path); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("%s remains: %v", path, err)
		}
	}
	if last, fresh, err := TakeAttempt(fixture.updates); err != nil || fresh || last == nil || *last != *attempt {
		t.Fatalf("the service reads %+v (fresh %v), %v; want the recorded attempt", last, fresh, err)
	}
	if attempt, err := WithdrawUnapplied(fixture.updates, now); attempt != nil || err != nil {
		t.Fatalf("no request = %+v, %v", attempt, err)
	}
}
