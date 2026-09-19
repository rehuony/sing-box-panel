// SPDX-License-Identifier: GPL-3.0-or-later

package installation

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/rehuony/sing-box-panel/internal/application"
	"github.com/rehuony/sing-box-panel/internal/panelprocess"
	"github.com/rehuony/sing-box-panel/internal/settings"
	"github.com/rehuony/sing-box-panel/internal/store"
)

const dataMoveMarker = ".data-migration"

type dataMoveOwner struct {
	ID           string `json:"id"`
	SettingsPath string `json:"settings_path"`
	Source       string `json:"source"`
	Target       string `json:"target"`
}

// PrepareDataLocation runs before the server opens storage. A settings edit
// requests a move; only a stopped, exclusively owned instance can perform it.
func PrepareDataLocation(ctx context.Context, path string) (settings.Settings, error) {
	path, err := filepath.Abs(path)
	if err != nil {
		return settings.Settings{}, err
	}
	lock, err := settings.Lock(ctx, path)
	if err != nil {
		return settings.Settings{}, err
	}
	defer lock.Close()
	value, err := settings.LoadForStartup(path)
	if err != nil {
		return value, err
	}
	location, err := settings.ReadDataLocation(path)
	if errors.Is(err, os.ErrNotExist) {
		if _, markerErr := os.Lstat(filepath.Join(value.DataDir, dataMoveMarker)); markerErr == nil {
			return value, errors.New("data directory belongs to an unfinished migration")
		}
		err = settings.RememberDataLocation(path, value.DataDir)
		return value, err
	}
	if err != nil {
		return value, err
	}
	if location.Move == nil && location.DataDir == value.DataDir {
		// The location may have committed just before a crash removed its marker.
		marker, err := readDataMoveOwner(value.DataDir)
		if errors.Is(err, os.ErrNotExist) {
			return value, nil
		}
		if err != nil {
			return value, err
		}
		if marker.SettingsPath != path || marker.Target != value.DataDir {
			return value, errors.New("data directory belongs to another migration")
		}
		return value, settings.RemoveDurable(filepath.Join(value.DataDir, dataMoveMarker))
	}
	if err := settings.CheckPending(path); err != nil {
		if !errors.Is(err, settings.ErrPending) {
			return value, err
		}
		if err := recoverSettingsBeforeDataMove(ctx, location.DataDir, value); err != nil {
			return value, err
		}
		value, err = settings.LoadForStartup(path)
		if err != nil {
			return value, err
		}
		if location.Move == nil && location.DataDir == value.DataDir {
			return value, nil
		}
	}
	if location.Move != nil && location.Move.Target != value.DataDir {
		return value, errors.New("finish the pending data directory migration before selecting another destination")
	}
	if err := migrateDataDirectory(ctx, path, &location, value.DataDir); err != nil {
		return value, fmt.Errorf("migrate panel data directory: %w", err)
	}
	return value, nil
}

func migrateDataDirectory(ctx context.Context, settingsPath string, location *settings.DataLocation, target string) error {
	source := location.DataDir
	overlap, err := DataDirectoriesOverlap(source, target)
	if err != nil {
		return err
	}
	if overlap {
		return errors.New("source and destination data directories must be separate physical locations")
	}
	for _, dir := range []string{source, target} {
		inside, err := DataDirectoryContainsPath(dir, settingsPath)
		if err != nil {
			return err
		}
		if inside {
			return errors.New("settings must be outside both data directories during migration")
		}
		report := Report{DataDir: dir, SettingsPath: settingsPath, Entries: []Entry{{Path: executablePath()}}}
		if err := ValidateCleanup(report); err != nil {
			return err
		}
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	var sourceInfo os.FileInfo
	var database *store.Store
	if location.Move == nil || !location.Move.Ready {
		sourceInfo, err = os.Lstat(source)
		if errors.Is(err, os.ErrNotExist) && location.Move == nil && !location.Established {
			return settings.WriteDataLocation(settingsPath, settings.DataLocation{DataDir: target})
		}
		if err != nil || !sourceInfo.IsDir() || sourceInfo.Mode()&os.ModeSymlink != 0 {
			return errors.New("source must remain a physical directory")
		}
		lease, err := panelprocess.AcquireLease(source)
		if err != nil {
			return err
		}
		defer lease.Close()
		if !location.Established && location.Move == nil {
			entries, err := os.ReadDir(source)
			if err != nil {
				return err
			}
			if len(entries) == 1 && entries[0].Name() == panelprocess.LeaseFileName {
				lock, err := store.LockDirectoryForCleanup(source)
				if err != nil {
					return err
				}
				defer lock.Close()
				entries, err = os.ReadDir(source)
				if err != nil || len(entries) != 1 || entries[0].Name() != panelprocess.LeaseFileName {
					return errors.New("source changed while preparing its initial data location")
				}
				if entries, err := os.ReadDir(target); err == nil && len(entries) != 0 {
					return errors.New("destination data directory is not empty")
				} else if err != nil && !errors.Is(err, os.ErrNotExist) {
					return err
				}
				return settings.WriteDataLocation(settingsPath, settings.DataLocation{DataDir: target})
			}
		}
		if marker, markerErr := readDataMoveOwner(source); markerErr == nil {
			if location.Move == nil || marker.ID != location.Move.ID || marker.Target != target || marker.Source != source {
				return errors.New("source belongs to another data migration")
			}
		} else if !errors.Is(markerErr, os.ErrNotExist) {
			return markerErr
		}
		if err := requirePhysicalDataFile(source); err != nil {
			return err
		}
		identity, err := store.ReadApplicationID(ctx, filepath.Join(source, "panel.db"))
		if err != nil || identity != store.ApplicationID {
			return errors.New("source does not contain a recognized panel database")
		}
		database, err = store.OpenForMigration(ctx, filepath.Join(source, "panel.db"))
		if err != nil {
			return err
		}
		defer database.Close()
		if err := requireStoppedCore(ctx, database); err != nil {
			return err
		}
		locks, err := lockDataMoveTree(ctx, source)
		if err != nil {
			return err
		}
		defer closeDataMoveLocks(locks)
	}
	if location.Move == nil {
		if entries, err := os.ReadDir(target); err == nil && len(entries) != 0 {
			return errors.New("destination data directory is not empty")
		} else if err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		var id [16]byte
		if _, err := rand.Read(id[:]); err != nil {
			return err
		}
		location.Move = &settings.DataMove{ID: hex.EncodeToString(id[:]), Target: target}
		if err := settings.WriteDataLocation(settingsPath, *location); err != nil {
			return err
		}
	}
	owner := dataMoveOwner{ID: location.Move.ID, SettingsPath: settingsPath, Source: source, Target: target}
	if err := os.MkdirAll(target, 0700); err != nil {
		return err
	}
	targetInfo, err := os.Lstat(target)
	if err != nil || !targetInfo.IsDir() || targetInfo.Mode()&os.ModeSymlink != 0 {
		return errors.New("destination must be a physical directory")
	}
	if err := claimDataMoveTarget(target, owner); err != nil {
		return err
	}
	if err := os.Chmod(target, 0700); err != nil {
		return err
	}
	targetLease, err := panelprocess.AcquireLease(target)
	if err != nil {
		return err
	}
	defer targetLease.Close()
	// The marker prevents normal database opens even across interrupted moves.
	if !location.Move.Ready {
		if err := writeDataMoveOwner(source, owner); err != nil {
			return err
		}
		if err := copyDataMoveTree(ctx, source, target); err != nil {
			return err
		}
		for _, name := range []string{"panel.db", "panel.db-wal", "panel.db-shm"} {
			if err := os.Remove(filepath.Join(target, name)); err != nil && !errors.Is(err, os.ErrNotExist) {
				return err
			}
		}
		if err := database.SnapshotDataFile(ctx, filepath.Join(target, "panel.db")); err != nil {
			return err
		}
		moved, err := store.OpenForMigration(ctx, filepath.Join(target, "panel.db"))
		if err != nil {
			return err
		}
		err = moved.RebaseDataPaths(ctx, source, target, owner.ID)
		if err == nil {
			err = moved.VerifyDataMove(ctx, owner.ID, source, target)
		}
		err = errors.Join(err, moved.Close())
		if err != nil {
			return err
		}
		if err := syncDataMoveTree(ctx, target); err != nil {
			return err
		}
		if err := verifyDataMoveFiles(ctx, source, target); err != nil {
			return err
		}
		location.Move.Ready = true
		if err := settings.WriteDataLocation(settingsPath, *location); err != nil {
			return err
		}
		// Exclusive ownership remains held through the final removal.
		if err := removeMovedSource(ctx, source, sourceInfo); err != nil {
			return err
		}
	} else {
		if err := requirePhysicalDataFile(target); err != nil {
			return err
		}
		moved, err := store.OpenForMigration(ctx, filepath.Join(target, "panel.db"))
		if err != nil {
			return err
		}
		err = moved.VerifyDataMove(ctx, owner.ID, source, target)
		err = errors.Join(err, moved.Close())
		if err != nil {
			return err
		}
		if info, err := os.Lstat(source); err == nil {
			marker, err := readDataMoveOwner(source)
			if err != nil || marker != owner {
				return errors.New("source migration identity changed")
			}
			lease, err := panelprocess.AcquireLease(source)
			if err != nil {
				return err
			}
			defer lease.Close()
			lock, err := store.LockDirectoryForCleanup(source)
			if err != nil {
				return err
			}
			defer lock.Close()
			locks, err := lockDataMoveTree(ctx, source)
			if err != nil {
				return err
			}
			defer closeDataMoveLocks(locks)
			if err := verifyDataMoveFiles(ctx, source, target); err != nil {
				return err
			}
			if err := removeMovedSource(ctx, source, info); err != nil {
				return err
			}
		} else if !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	if err := settings.WriteDataLocation(settingsPath, settings.DataLocation{DataDir: target, Established: true}); err != nil {
		return err
	}
	return settings.RemoveDurable(filepath.Join(target, dataMoveMarker))
}

func recoverSettingsBeforeDataMove(ctx context.Context, dataDir string, value settings.Settings) error {
	if err := requirePhysicalDataFile(dataDir); err != nil {
		return err
	}
	identity, err := store.ReadApplicationID(ctx, filepath.Join(dataDir, "panel.db"))
	if err != nil || identity != store.ApplicationID {
		return errors.New("original database is required to recover settings before migration")
	}
	lease, err := panelprocess.AcquireLease(dataDir)
	if err != nil {
		return err
	}
	defer lease.Close()
	database, err := store.OpenForMigration(ctx, filepath.Join(dataDir, "panel.db"))
	if err != nil {
		return err
	}
	defer database.Close()
	return application.FromStoreWithSettings(database, value).RecoverPanelSettingsFileLocked(ctx)
}

func requirePhysicalDataFile(dir string) error {
	info, err := os.Lstat(filepath.Join(dir, "panel.db"))
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return errors.New("migration requires a regular database file inside the data directory")
	}
	return nil
}

func requireStoppedCore(ctx context.Context, database *store.Store) error {
	observation, err := database.RuntimeObservation(ctx)
	if errors.Is(err, store.ErrRuntimeObservationNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	token, err := application.NewRuntimeIdentityResolver(database).ProcessStartToken(ctx, observation.PID)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("cannot prove the managed core has exited: %w", err)
	}
	if token == observation.ProcessStartToken {
		return errors.New("stop the managed core before moving its data directory")
	}
	return nil
}

func executablePath() string { path, _ := os.Executable(); return path }

func readDataMoveOwner(dir string) (dataMoveOwner, error) {
	path := filepath.Join(dir, dataMoveMarker)
	info, err := os.Lstat(path)
	if err != nil {
		return dataMoveOwner{}, err
	}
	if !info.Mode().IsRegular() || info.Size() > 16384 {
		return dataMoveOwner{}, errors.New("invalid data migration marker")
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return dataMoveOwner{}, err
	}
	var value dataMoveOwner
	err = json.Unmarshal(raw, &value)
	return value, err
}

func writeDataMoveOwner(dir string, owner dataMoveOwner) error {
	raw, err := json.Marshal(owner)
	if err != nil {
		return err
	}
	return settings.WriteAtomic(filepath.Join(dir, dataMoveMarker), raw)
}

func claimDataMoveTarget(dir string, owner dataMoveOwner) error {
	current, err := readDataMoveOwner(dir)
	if err == nil && current == owner {
		return nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return errors.New("destination belongs to another data migration")
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	if len(entries) != 0 {
		return errors.New("destination data directory is not empty")
	}
	return writeDataMoveOwner(dir, owner)
}
