// SPDX-License-Identifier: GPL-3.0-or-later

package settings

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"

	"github.com/rehuony/sing-box-panel/internal/jsonstrict"
)

// DataLocation records the last established storage location, not a second
// settings source. It lets a later startup recognize manual data_dir edits.
type DataLocation struct {
	DataDir     string    `json:"data_dir"`
	Established bool      `json:"established"`
	Move        *DataMove `json:"move,omitempty"`
}

type DataMove struct {
	ID     string `json:"id"`
	Target string `json:"target"`
	Ready  bool   `json:"ready"`
}

func ReadDataLocation(path string) (DataLocation, error) {
	info, err := os.Lstat(path + ".location")
	if err != nil {
		return DataLocation{}, err
	}
	if !info.Mode().IsRegular() || info.Size() > 16384 {
		return DataLocation{}, errors.New("invalid data location record")
	}
	raw, err := os.ReadFile(path + ".location")
	if err != nil {
		return DataLocation{}, err
	}
	var value DataLocation
	if err := jsonstrict.Decode(raw, 16384, &value); err != nil {
		return DataLocation{}, err
	}
	if !filepath.IsAbs(value.DataDir) || filepath.Clean(value.DataDir) != value.DataDir {
		return DataLocation{}, errors.New("invalid recorded data directory")
	}
	if value.Move != nil && (value.Move.ID == "" || !filepath.IsAbs(value.Move.Target) || filepath.Clean(value.Move.Target) != value.Move.Target) {
		return DataLocation{}, errors.New("invalid data migration record")
	}
	return value, nil
}

// WriteDataLocation requires the selected settings writer lock.
func WriteDataLocation(path string, value DataLocation) error {
	raw, err := json.Marshal(value)
	if err != nil {
		return err
	}
	return WriteAtomic(path+".location", raw)
}

// RememberDataLocation seeds the record once. Existing recovery state is never
// overwritten by an ordinary settings save or init --force.
func RememberDataLocation(path, dataDir string) error {
	if _, err := ReadDataLocation(path); err == nil {
		return nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	info, err := os.Stat(filepath.Join(dataDir, "panel.db"))
	established := err == nil && info.Mode().IsRegular() && info.Size() > 0
	return WriteDataLocation(path, DataLocation{DataDir: dataDir, Established: established})
}

// ActiveDataDir locates the existing instance even after data_dir was edited.
// It is used for stop/status and database commands, never as a settings override.
func ActiveDataDir(path, configured string) (string, error) {
	value, err := ReadDataLocation(path)
	if errors.Is(err, os.ErrNotExist) {
		return configured, nil
	}
	if err != nil {
		return "", err
	}
	if value.Move != nil && value.Move.Ready {
		return "", errors.New("data directory migration is incomplete; start the panel to recover it")
	}
	return value.DataDir, nil
}

// EstablishDataLocation records successful initialization without altering a
// pending move or selecting a different database.
func EstablishDataLocation(ctx context.Context, path, dataDir string) error {
	lock, err := Lock(ctx, path)
	if err != nil {
		return err
	}
	defer lock.Close()
	value, err := ReadDataLocation(path)
	if err != nil {
		return err
	}
	if value.DataDir != dataDir || value.Move != nil {
		return errors.New("data location changed during initialization")
	}
	if value.Established {
		return nil
	}
	value.Established = true
	return WriteDataLocation(path, value)
}
