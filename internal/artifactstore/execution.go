// SPDX-License-Identifier: GPL-3.0-or-later

package artifactstore

import (
	"context"
	"crypto/sha256"
	"errors"
	"os"
	"path/filepath"

	"github.com/rehuony/sing-box-panel/internal/coreartifact"
)

func prepareExecutionCopy(
	ctx context.Context,
	sourcePath string,
	expectedDigest coreartifact.SHA256,
	maximumBytes int64,
) (string, string, error) {
	lexicalParent := os.TempDir()
	if err := verifyTrustedLexicalPath(lexicalParent); err != nil {
		return "", "", fail(StepVersion, "unsafe_execution_parent", err)
	}
	resolvedParent, err := filepath.EvalSymlinks(lexicalParent)
	if err != nil {
		return "", "", fail(StepVersion, "execution_parent_resolve", err)
	}
	directory, err := os.MkdirTemp(resolvedParent, "sing-box-panel-version-")
	if err != nil {
		return "", "", fail(StepVersion, "execution_directory", err)
	}
	cleanup := true
	defer func() {
		if cleanup {
			_ = os.RemoveAll(directory)
		}
	}()
	if err := verifyTrustedAncestors(directory); err != nil {
		return "", "", fail(StepVersion, "unsafe_execution_directory", err)
	}
	destinationPath := filepath.Join(directory, "sing-box")
	source, err := os.Open(sourcePath)
	if err != nil {
		return "", "", fail(StepVersion, "execution_source", err)
	}
	destination, err := os.OpenFile(destinationPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		source.Close()
		return "", "", fail(StepVersion, "execution_copy", err)
	}
	written, copyErr := copyBounded(ctx, destination, source, maximumBytes)
	modeErr := destination.Chmod(0o700)
	syncErr := destination.Sync()
	closeDestinationErr := destination.Close()
	closeSourceErr := source.Close()
	if copyErr != nil || written <= 0 || modeErr != nil || syncErr != nil || closeDestinationErr != nil || closeSourceErr != nil {
		return "", "", fail(StepVersion, "execution_copy", errors.Join(copyErr, modeErr, syncErr, closeDestinationErr, closeSourceErr))
	}
	if err := verifyFileDigest(ctx, destinationPath, expectedDigest, maximumBytes); err != nil {
		return "", "", fail(StepVersion, "execution_copy_digest", err)
	}
	cleanup = false
	return destinationPath, directory, nil
}

func verifyFileDigest(ctx context.Context, path string, expected coreartifact.SHA256, maximumBytes int64) error {
	actual, err := digestFile(ctx, path, maximumBytes)
	if err != nil {
		return err
	}
	if actual != expected {
		return ErrDigest
	}
	return nil
}

func digestFile(ctx context.Context, path string, maximumBytes int64) (coreartifact.SHA256, error) {
	file, err := os.Open(path)
	if err != nil {
		return coreartifact.SHA256{}, err
	}
	defer file.Close()
	hash := sha256.New()
	written, err := copyBounded(ctx, hash, file, maximumBytes)
	if err != nil || written <= 0 {
		return coreartifact.SHA256{}, errors.Join(err, ErrDigest)
	}
	var sum [32]byte
	copy(sum[:], hash.Sum(nil))
	return coreartifact.NewSHA256(sum), nil
}
