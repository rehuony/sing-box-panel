// SPDX-License-Identifier: GPL-3.0-or-later

package runtime

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/rehuony/sing-box-panel/internal/coreartifact"
)

var fixedCommandEnvironment = []string{
	"LANG=C",
	"LC_ALL=C",
	"PATH=/usr/bin:/bin",
}

func cloneAndValidateBundle(bundle AppliedBundle, maximumConfigBytes int64) (AppliedBundle, error) {
	bundle.StartupConfig = bytes.Clone(bundle.StartupConfig)
	if !validIdentifier(bundle.ID) || !validIdentifier(bundle.ArtifactID) || bundle.ExactVersion.IsZero() ||
		bundle.ArtifactDigest.IsZero() || bundle.StartupConfigDigest.IsZero() {
		return AppliedBundle{}, ErrInvalidBundle
	}
	if bundle.BinaryPath == "" || !filepath.IsAbs(bundle.BinaryPath) ||
		filepath.Clean(bundle.BinaryPath) != bundle.BinaryPath {
		return AppliedBundle{}, ErrInvalidBundle
	}
	if len(bundle.StartupConfig) == 0 || int64(len(bundle.StartupConfig)) > maximumConfigBytes {
		return AppliedBundle{}, ErrInvalidBundle
	}
	return bundle, nil
}

func validIdentifier(value string) bool {
	if len(value) == 0 || len(value) > 128 || !utf8.ValidString(value) {
		return false
	}
	for _, character := range value {
		if unicode.IsControl(character) {
			return false
		}
	}
	return true
}

func verifyStartupConfigDigest(data []byte, expected coreartifact.SHA256) error {
	actual := coreartifact.NewSHA256(sha256.Sum256(data))
	if actual != expected {
		return ErrStartupConfigDigest
	}
	return nil
}

func verifyBinaryDigest(
	ctx context.Context,
	path string,
	expected coreartifact.SHA256,
	maximumBytes int64,
) (coreartifact.SHA256, error) {
	if err := contextError(ctx); err != nil {
		return coreartifact.SHA256{}, err
	}
	pathInfo, err := os.Lstat(path)
	if err != nil {
		return coreartifact.SHA256{}, errors.Join(ErrArtifactDigest, err)
	}
	if pathInfo.Mode()&os.ModeSymlink != 0 || !pathInfo.Mode().IsRegular() ||
		pathInfo.Mode().Perm()&0o111 == 0 || pathInfo.Size() <= 0 || pathInfo.Size() > maximumBytes {
		return coreartifact.SHA256{}, ErrArtifactDigest
	}

	file, err := os.Open(path)
	if err != nil {
		return coreartifact.SHA256{}, errors.Join(ErrArtifactDigest, err)
	}
	defer file.Close()
	openedInfo, err := file.Stat()
	if err != nil || !os.SameFile(pathInfo, openedInfo) {
		return coreartifact.SHA256{}, errors.Join(ErrArtifactDigest, err)
	}

	hasher := sha256.New()
	reader := &contextReader{ctx: ctx, reader: io.LimitReader(file, maximumBytes+1)}
	written, err := io.Copy(hasher, reader)
	if err != nil {
		return coreartifact.SHA256{}, errors.Join(ErrArtifactDigest, err)
	}
	if written != pathInfo.Size() || written > maximumBytes {
		return coreartifact.SHA256{}, ErrArtifactDigest
	}
	afterInfo, err := file.Stat()
	if err != nil || !os.SameFile(openedInfo, afterInfo) || afterInfo.Size() != openedInfo.Size() ||
		!afterInfo.ModTime().Equal(openedInfo.ModTime()) {
		return coreartifact.SHA256{}, errors.Join(ErrArtifactDigest, err)
	}
	pathAfterInfo, err := os.Lstat(path)
	if err != nil || pathAfterInfo.Mode()&os.ModeSymlink != 0 || !os.SameFile(afterInfo, pathAfterInfo) {
		return coreartifact.SHA256{}, errors.Join(ErrArtifactDigest, err)
	}

	var sum [sha256.Size]byte
	copy(sum[:], hasher.Sum(nil))
	actual := coreartifact.NewSHA256(sum)
	if actual != expected {
		return actual, ErrArtifactDigest
	}
	return actual, nil
}

type contextReader struct {
	ctx    context.Context
	reader io.Reader
}

func (reader *contextReader) Read(data []byte) (int, error) {
	if err := contextError(reader.ctx); err != nil {
		return 0, err
	}
	return reader.reader.Read(data)
}

func parseVersionOutput(output []byte) (coreartifact.ExactVersion, error) {
	for _, line := range strings.Split(string(output), "\n") {
		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}
		if len(fields) != 3 || fields[0] != "sing-box" || fields[1] != "version" {
			return coreartifact.ExactVersion{}, fmt.Errorf("unexpected version banner")
		}
		return coreartifact.ParseExactVersion(fields[2])
	}
	return coreartifact.ExactVersion{}, fmt.Errorf("missing version banner")
}

func contextError(ctx context.Context) error {
	if ctx == nil {
		return errors.New("context is nil")
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
		return nil
	}
}
