// SPDX-License-Identifier: GPL-3.0-or-later

package runtime

import (
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"

	"github.com/rehuony/sing-box-panel/internal/coreartifact"
)

func materializeStartupConfig(
	runtimeDir string,
	digest coreartifact.SHA256,
	data []byte,
) (string, error) {
	if err := ensurePrivateDirectory(runtimeDir); err != nil {
		return "", errors.Join(ErrMaterialization, err)
	}
	configDirectory := filepath.Join(runtimeDir, "configs")
	if err := ensurePrivateDirectory(configDirectory); err != nil {
		return "", errors.Join(ErrMaterialization, err)
	}
	finalPath := filepath.Join(configDirectory, digest.String()+".json")
	if err := verifyExistingConfig(finalPath, data); err == nil {
		if err := syncDirectory(configDirectory); err != nil {
			return "", errors.Join(ErrMaterialization, err)
		}
		return finalPath, nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", errors.Join(ErrMaterialization, err)
	}

	temporary, err := os.CreateTemp(configDirectory, ".startup-config-")
	if err != nil {
		return "", errors.Join(ErrMaterialization, err)
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if err := temporary.Chmod(0o600); err != nil {
		temporary.Close()
		return "", errors.Join(ErrMaterialization, err)
	}
	if _, err := io.Copy(temporary, bytes.NewReader(data)); err != nil {
		temporary.Close()
		return "", errors.Join(ErrMaterialization, err)
	}
	if err := temporary.Sync(); err != nil {
		temporary.Close()
		return "", errors.Join(ErrMaterialization, err)
	}
	if err := temporary.Close(); err != nil {
		return "", errors.Join(ErrMaterialization, err)
	}

	if err := os.Link(temporaryPath, finalPath); err != nil {
		if !errors.Is(err, os.ErrExist) {
			return "", errors.Join(ErrMaterialization, err)
		}
		if err := verifyExistingConfig(finalPath, data); err != nil {
			return "", errors.Join(ErrMaterialization, err)
		}
	}
	if err := syncDirectory(configDirectory); err != nil {
		return "", errors.Join(ErrMaterialization, err)
	}
	return finalPath, nil
}

func ensurePrivateDirectory(path string) error {
	if err := os.MkdirAll(path, 0o700); err != nil {
		return err
	}
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() || info.Mode().Perm()&0o077 != 0 {
		return errors.New("runtime directory is not private")
	}
	return nil
}

func verifyExistingConfig(path string, expected []byte) error {
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() || info.Mode().Perm() != 0o600 ||
		info.Size() != int64(len(expected)) {
		return errors.New("materialized config metadata is invalid")
	}
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()
	openedInfo, err := file.Stat()
	if err != nil || !os.SameFile(info, openedInfo) {
		return errors.Join(errors.New("materialized config changed while opening"), err)
	}
	actual, err := io.ReadAll(io.LimitReader(file, int64(len(expected))+1))
	if err != nil {
		return err
	}
	pathAfterInfo, err := os.Lstat(path)
	if err != nil || pathAfterInfo.Mode()&os.ModeSymlink != 0 || !os.SameFile(openedInfo, pathAfterInfo) {
		return errors.Join(errors.New("materialized config changed while reading"), err)
	}
	if !bytes.Equal(actual, expected) {
		return errors.New("materialized config content does not match")
	}
	return nil
}

func syncDirectory(path string) error {
	directory, err := os.Open(path)
	if err != nil {
		return err
	}
	defer directory.Close()
	return directory.Sync()
}
