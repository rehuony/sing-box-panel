// SPDX-License-Identifier: GPL-3.0-or-later

package selfupdate

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/rehuony/sing-box-panel/internal/atomicfile"
	"github.com/rehuony/sing-box-panel/internal/jsonstrict"
	releasecontract "github.com/rehuony/sing-box-panel/internal/release"
)

// A service keeps its updates in UpdatesDir inside its data directory: an
// incoming directory per release being received, the staged release
// VERSION/ that a system service hands to its unit's install step, the
// request that names it, and the results of attempts.
const (
	UpdatesDir     = "updates"
	RequestName    = "request"
	resultName     = "result"
	lastResultName = "last-result"
	incomingPrefix = ".incoming-"
	maxRequest     = 256
	maxResultBytes = 4096
)

// ErrInstallStepMissing records a staged release that was still requested
// when the service started again: its unit has no install step, as a unit
// an earlier release wrote does not.
var ErrInstallStepMissing = errors.New("the service's unit did not install the staged release; rewrite the unit with systemd install")

// Delivery is how a verified release reached the running service.
type Delivery string

const (
	// DeliveryInstalled: the process replaced its executable; the new
	// version runs after the service restarts.
	DeliveryInstalled Delivery = "installed"
	// DeliveryStaged: a system service that may not replace its executable
	// staged the release; its unit installs it when it starts the service
	// again.
	DeliveryStaged Delivery = "staged"
)

// IncomingDir creates a new directory inside updatesDir to receive a release
// into. StageRelease moves it into place; otherwise the caller removes it.
func IncomingDir(updatesDir string) (string, error) {
	if err := os.MkdirAll(updatesDir, 0o700); err != nil {
		return "", fmt.Errorf("create updates directory: %w", err)
	}
	return os.MkdirTemp(updatesDir, incomingPrefix+"*")
}

// Deliver installs a verified release in place when the process may replace
// its executable. A system service that may not stages it in updatesDir for
// its unit's install step instead; staged must then have been received in an
// IncomingDir of updatesDir. Any other process fails with
// ErrExecutableNotWritable.
func (updater *Updater) Deliver(ctx context.Context, staged Staged, updatesDir string, systemService bool) (Delivery, error) {
	writable, err := updater.Writable()
	if err != nil {
		return "", err
	}
	if writable {
		if _, err := updater.Install(ctx, staged); err != nil {
			return "", err
		}
		return DeliveryInstalled, nil
	}
	if !systemService {
		return "", fmt.Errorf("%w: %w", ErrExecutableNotWritable, fs.ErrPermission)
	}
	if err := StageRelease(staged, updatesDir); err != nil {
		return "", err
	}
	return DeliveryStaged, nil
}

// StageRelease moves the incoming directory holding staged to
// updatesDir/VERSION and then writes the request that names it, which the
// install step of a system service's unit reads when it starts the service.
func StageRelease(staged Staged, updatesDir string) error {
	incoming := filepath.Dir(staged.Path)
	if filepath.Dir(incoming) != filepath.Clean(updatesDir) || !strings.HasPrefix(filepath.Base(incoming), incomingPrefix) {
		return fmt.Errorf("%w: the release was not received in the updates directory", ErrStagedFileInvalid)
	}
	if err := releasecontract.ValidateVersion(staged.Version); err != nil {
		return fmt.Errorf("%w: %q is not a release version", ErrReleaseInvalid, staged.Version)
	}
	final := filepath.Join(updatesDir, staged.Version)
	if err := os.RemoveAll(final); err != nil {
		return fmt.Errorf("replace staged release: %w", err)
	}
	if err := os.Rename(incoming, final); err != nil {
		return fmt.Errorf("stage release: %w", err)
	}
	if err := atomicfile.SyncDir(updatesDir); err != nil {
		return err
	}
	return atomicfile.Write(filepath.Join(updatesDir, RequestName), []byte(staged.Version+"\n"), 0o600, atomicfile.Options{})
}

// CleanUpdates removes incoming directories and staged releases from
// updatesDir, keeping the release a pending request names.
func CleanUpdates(updatesDir string) error {
	entries, err := os.ReadDir(updatesDir)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	pending := ""
	if raw, err := os.ReadFile(filepath.Join(updatesDir, RequestName)); err == nil {
		pending, _ = parseRequest(raw)
	}
	var failures []error
	for _, entry := range entries {
		name := entry.Name()
		if !entry.IsDir() || name == pending {
			continue
		}
		if strings.HasPrefix(name, incomingPrefix) || releasecontract.ValidateVersion(name) == nil {
			if err := os.RemoveAll(filepath.Join(updatesDir, name)); err != nil {
				failures = append(failures, err)
			}
		}
	}
	return errors.Join(failures...)
}

// parseRequest reads a request: one strict release version and an optional
// final newline, nothing else.
func parseRequest(raw []byte) (string, error) {
	if len(raw) > maxRequest {
		return "", fmt.Errorf("%w: the update request is too long", ErrReleaseInvalid)
	}
	version := strings.TrimSuffix(string(raw), "\n")
	if err := releasecontract.ValidateVersion(version); err != nil {
		return "", fmt.Errorf("%w: the update request does not hold a release version", ErrReleaseInvalid)
	}
	return version, nil
}

// Attempt is the outcome of an update: the version it installed or tried
// to, and, when it failed, a short code from FailureCode.
type Attempt struct {
	Version    string    `json:"version"`
	Succeeded  bool      `json:"succeeded"`
	Code       string    `json:"code,omitempty"`
	FinishedAt time.Time `json:"finished_at"`
}

// TakeAttempt reads the last update attempt in updatesDir, if any. A result
// that the install step wrote since the service last read one is fresh: it
// moves to the last result, so each is fresh once.
func TakeAttempt(updatesDir string) (*Attempt, bool, error) {
	fresh := filepath.Join(updatesDir, resultName)
	last := filepath.Join(updatesDir, lastResultName)
	attempt, err := readAttempt(fresh)
	if err == nil {
		if err := os.Rename(fresh, last); err != nil {
			return nil, false, err
		}
		return attempt, true, nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		// A malformed result cannot be reported; drop it so it is not read again.
		_ = os.Remove(fresh)
	}
	attempt, err = readAttempt(last)
	if errors.Is(err, os.ErrNotExist) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	return attempt, false, nil
}

// WithdrawUnapplied withdraws a staged release whose request is still there
// when the service starts: the install step takes the request before the
// service starts, so its unit has none. It removes the request and the staged
// release and records a failed attempt with the code install_step_missing,
// which it returns; nil means nothing was requested.
func WithdrawUnapplied(updatesDir string, now time.Time) (*Attempt, error) {
	request := filepath.Join(updatesDir, RequestName)
	raw, err := os.ReadFile(request)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	version, _ := parseRequest(raw)
	if err := os.Remove(request); err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	if version != "" {
		if err := os.RemoveAll(filepath.Join(updatesDir, version)); err != nil {
			return nil, err
		}
	}
	attempt := Attempt{Version: version, Code: FailureCode(ErrInstallStepMissing), FinishedAt: now.UTC()}
	return &attempt, RecordAttempt(updatesDir, attempt)
}

// RecordAttempt saves an attempt the service itself made as the last result.
func RecordAttempt(updatesDir string, attempt Attempt) error {
	if err := os.MkdirAll(updatesDir, 0o700); err != nil {
		return err
	}
	data, err := json.Marshal(attempt)
	if err != nil {
		return err
	}
	return atomicfile.Write(filepath.Join(updatesDir, lastResultName), append(data, '\n'), 0o600, atomicfile.Options{})
}

func readAttempt(path string) (*Attempt, error) {
	file, err := openStaged(path, maxResultBytes)
	if err != nil {
		if _, statErr := os.Lstat(path); errors.Is(statErr, os.ErrNotExist) {
			return nil, os.ErrNotExist
		}
		return nil, err
	}
	defer file.Close()
	raw, err := readBounded(file, maxResultBytes)
	if err != nil {
		return nil, err
	}
	var attempt Attempt
	if err := jsonstrict.Decode(raw, maxResultBytes, &attempt); err != nil {
		return nil, err
	}
	if attempt.Version != "" && releasecontract.ValidateVersion(attempt.Version) != nil || !validCode(attempt.Code) {
		return nil, errors.New("the update result is malformed")
	}
	return &attempt, nil
}

func validCode(code string) bool {
	if len(code) > 64 {
		return false
	}
	for _, character := range code {
		if (character < 'a' || character > 'z') && character != '_' {
			return false
		}
	}
	return true
}

// FailureCode names why an update failed, in a short stable code for
// results and problem details: canceled, timed_out, up_to_date,
// version_not_newer, unsupported, verification_key_invalid,
// signature_invalid, checksum_invalid, identity_invalid,
// staged_file_invalid, release_unavailable, release_invalid,
// executable_changed, permission_denied, install_step_missing, or
// install_failed for anything else.
func FailureCode(err error) string {
	for _, known := range []struct {
		target error
		code   string
	}{
		{context.Canceled, "canceled"},
		{context.DeadlineExceeded, "timed_out"},
		{ErrUpToDate, "up_to_date"},
		{ErrVersionNotNewer, "version_not_newer"},
		{ErrUnsupportedPlatform, "unsupported"},
		{ErrInvalidVersion, "unsupported"},
		{ErrVerificationKeyInvalid, "verification_key_invalid"},
		{ErrSignatureInvalid, "signature_invalid"},
		{ErrChecksumInvalid, "checksum_invalid"},
		{ErrStagedExecutableInvalid, "identity_invalid"},
		{ErrStagedFileInvalid, "staged_file_invalid"},
		{ErrAssetMissing, "release_unavailable"},
		{ErrReleaseUnavailable, "release_unavailable"},
		{ErrReleaseInvalid, "release_invalid"},
		{ErrExecutableChanged, "executable_changed"},
		{ErrExecutableNotWritable, "permission_denied"},
		{fs.ErrPermission, "permission_denied"},
		{ErrInstallStepMissing, "install_step_missing"},
	} {
		if errors.Is(err, known.target) {
			return known.code
		}
	}
	return "install_failed"
}
