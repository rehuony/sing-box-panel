// SPDX-License-Identifier: GPL-3.0-or-later

package cli

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/rehuony/sing-box-panel/internal/capability"
	"github.com/spf13/cobra"
)

type capabilityPackResult struct {
	File          string `json:"file"`
	Repository    string `json:"repository"`
	CommitSHA     string `json:"commit_sha"`
	ManifestCount int    `json:"manifest_count"`
	SHA256        string `json:"sha256"`
}

func newCoreCapabilityPackCommand(state *options) *cobra.Command {
	var directoryPath, commit, filePath string
	var force bool
	command := &cobra.Command{
		Use:   "pack",
		Short: "Build a canonical commit-bound generation from local exact-version manifests",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			missing := make([]string, 0, 3)
			for _, flag := range []struct{ name, value string }{
				{name: "directory", value: directoryPath},
				{name: "commit", value: commit},
				{name: "file", value: filePath},
			} {
				if strings.TrimSpace(flag.value) == "" {
					missing = append(missing, "--"+flag.name)
				}
			}
			if len(missing) != 0 {
				return &Error{
					Kind:    ErrorUsage,
					Code:    "capability_pack_flag_required",
					Message: strings.Join(missing, ", ") + " are required",
				}
			}

			files, err := readCapabilityManifestDirectory(directoryPath)
			if err != nil {
				return &Error{
					Kind: ErrorValidation, Code: "capability_pack_directory_invalid",
					Message: err.Error(), Cause: err,
				}
			}
			generation, err := capability.BuildGeneration(commit, files)
			if err != nil {
				return &Error{
					Kind: ErrorValidation, Code: "capability_pack_invalid",
					Message: err.Error(), Cause: err,
				}
			}
			canonical, err := generation.CanonicalJSON()
			if err != nil {
				return &Error{
					Kind: ErrorDomain, Code: "capability_pack_encode_failed",
					Message: err.Error(), Cause: err,
				}
			}
			digest, err := generation.Digest()
			if err != nil {
				return &Error{
					Kind: ErrorDomain, Code: "capability_pack_digest_failed",
					Message: err.Error(), Cause: err,
				}
			}
			if filePath == "-" {
				if _, err := cmd.OutOrStdout().Write(canonical); err != nil {
					return &Error{
						Kind: ErrorDomain, Code: "capability_pack_output_failed",
						Message: err.Error(), Cause: err,
					}
				}
				return nil
			}
			if err := writeAtomicCapabilityPack(filePath, canonical, force); err != nil {
				if errors.Is(err, fs.ErrExist) {
					return &Error{
						Kind: ErrorConflict, Code: "capability_pack_output_exists",
						Message: "capability pack destination already exists; use --force to replace it",
						Cause:   err,
					}
				}
				return &Error{
					Kind: ErrorValidation, Code: "capability_pack_output_failed",
					Message: err.Error(), Cause: err,
				}
			}
			result := capabilityPackResult{
				File: filePath, Repository: generation.Repository(), CommitSHA: generation.Commit(),
				ManifestCount: len(generation.Manifests()), SHA256: digest.String(),
			}
			return writeResult(
				cmd.OutOrStdout(), state.format, result,
				fmt.Sprintf("packed %d capability manifests to %s", result.ManifestCount, filePath),
			)
		},
	}
	command.Flags().StringVar(&directoryPath, "directory", "", "directory containing only <major>.<minor>.<patch>.json manifests")
	command.Flags().StringVar(&commit, "commit", "", "immutable lowercase 40 or 64 character repository commit SHA")
	command.Flags().StringVar(&filePath, "file", "", "destination generation JSON file, or - for stdout")
	command.Flags().BoolVar(&force, "force", false, "atomically replace an existing destination file")
	return command
}

func readCapabilityManifestDirectory(directoryPath string) ([]capability.ManifestFile, error) {
	clean := filepath.Clean(directoryPath)
	directoryInfo, err := os.Lstat(clean)
	if err != nil {
		return nil, fmt.Errorf("inspect capability manifest directory: %w", err)
	}
	if directoryInfo.Mode()&os.ModeSymlink != 0 {
		return nil, errors.New("capability manifest directory must not be a symbolic link")
	}
	if !directoryInfo.IsDir() {
		return nil, errors.New("capability manifest directory is not a directory")
	}
	entries, err := os.ReadDir(clean)
	if err != nil {
		return nil, fmt.Errorf("read capability manifest directory: %w", err)
	}
	if len(entries) == 0 {
		return nil, errors.New("capability manifest directory is empty")
	}
	if len(entries) > capability.MaximumGenerationEntries {
		return nil, fmt.Errorf(
			"capability manifest directory exceeds %d entries",
			capability.MaximumGenerationEntries,
		)
	}

	files := make([]capability.ManifestFile, 0, len(entries))
	var totalBytes int64
	for _, entry := range entries {
		path := filepath.Join(clean, entry.Name())
		before, err := os.Lstat(path)
		if err != nil {
			return nil, fmt.Errorf("inspect capability manifest %q: %w", entry.Name(), err)
		}
		if before.Mode()&os.ModeSymlink != 0 {
			return nil, fmt.Errorf("capability manifest %q must not be a symbolic link", entry.Name())
		}
		if !before.Mode().IsRegular() {
			return nil, fmt.Errorf("capability manifest %q is not a regular file", entry.Name())
		}
		if before.Size() > capability.MaximumManifestBytes {
			return nil, fmt.Errorf(
				"capability manifest %q exceeds %d bytes",
				entry.Name(),
				capability.MaximumManifestBytes,
			)
		}
		data, err := readRegularCapabilityManifest(path, entry.Name(), before)
		if err != nil {
			return nil, err
		}
		if int64(len(data)) > int64(capability.MaximumGenerationBytes)-totalBytes {
			return nil, fmt.Errorf(
				"capability manifest directory exceeds %d input bytes",
				capability.MaximumGenerationBytes,
			)
		}
		totalBytes += int64(len(data))
		files = append(files, capability.ManifestFile{Name: entry.Name(), Data: data})
	}
	return files, nil
}

func readRegularCapabilityManifest(path, name string, before os.FileInfo) ([]byte, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open capability manifest %q: %w", name, err)
	}
	opened, err := file.Stat()
	if err != nil {
		_ = file.Close()
		return nil, fmt.Errorf("inspect opened capability manifest %q: %w", name, err)
	}
	if !opened.Mode().IsRegular() || !os.SameFile(before, opened) {
		_ = file.Close()
		return nil, fmt.Errorf("capability manifest %q changed before it was opened", name)
	}
	data, readErr := io.ReadAll(io.LimitReader(file, int64(capability.MaximumManifestBytes)+1))
	after, statErr := file.Stat()
	pathAfter, pathErr := os.Lstat(path)
	closedErr := file.Close()
	if readErr != nil {
		return nil, fmt.Errorf("read capability manifest %q: %w", name, readErr)
	}
	if statErr != nil || pathErr != nil || !after.Mode().IsRegular() ||
		pathAfter.Mode()&os.ModeSymlink != 0 || !pathAfter.Mode().IsRegular() ||
		!os.SameFile(opened, after) || !os.SameFile(after, pathAfter) ||
		after.Size() != opened.Size() || !after.ModTime().Equal(opened.ModTime()) ||
		int64(len(data)) != after.Size() {
		if cause := errors.Join(statErr, pathErr); cause != nil {
			return nil, fmt.Errorf("capability manifest %q changed while it was being read: %w", name, cause)
		}
		return nil, fmt.Errorf("capability manifest %q changed while it was being read", name)
	}
	if closedErr != nil {
		return nil, fmt.Errorf("close capability manifest %q: %w", name, closedErr)
	}
	if len(data) > capability.MaximumManifestBytes {
		return nil, fmt.Errorf(
			"capability manifest %q exceeds %d bytes",
			name,
			capability.MaximumManifestBytes,
		)
	}
	return data, nil
}

func writeAtomicCapabilityPack(path string, data []byte, force bool) error {
	clean := filepath.Clean(path)
	if clean == "." || clean == string(filepath.Separator) {
		return errors.New("capability pack destination is invalid")
	}
	directory := filepath.Dir(clean)
	temporary, err := os.CreateTemp(directory, ".sing-box-panel-capability-pack-*.tmp")
	if err != nil {
		return fmt.Errorf("create temporary capability pack: %w", err)
	}
	temporaryPath := temporary.Name()
	removeTemporary := true
	defer func() {
		_ = temporary.Close()
		if removeTemporary {
			_ = os.Remove(temporaryPath)
		}
	}()
	if err := temporary.Chmod(0o600); err != nil {
		return fmt.Errorf("set temporary capability pack permissions: %w", err)
	}
	if _, err := temporary.Write(data); err != nil {
		return fmt.Errorf("write temporary capability pack: %w", err)
	}
	if err := temporary.Sync(); err != nil {
		return fmt.Errorf("sync temporary capability pack: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("close temporary capability pack: %w", err)
	}
	if force {
		if err := os.Rename(temporaryPath, clean); err != nil {
			return fmt.Errorf("replace capability pack: %w", err)
		}
		removeTemporary = false
	} else {
		if err := os.Link(temporaryPath, clean); err != nil {
			return fmt.Errorf("publish capability pack without replacement: %w", err)
		}
		if err := os.Remove(temporaryPath); err != nil {
			return fmt.Errorf("remove linked capability pack temporary file: %w", err)
		}
		removeTemporary = false
	}
	if err := syncExportDirectory(directory); err != nil {
		return fmt.Errorf("sync capability pack directory: %w", err)
	}
	return nil
}
