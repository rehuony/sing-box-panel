// SPDX-License-Identifier: GPL-3.0-or-later

package backup

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/rehuony/sing-box-panel/internal/database"
	"github.com/rehuony/sing-box-panel/internal/jsonstrict"
	"github.com/rehuony/sing-box-panel/internal/settingsfile"
	"github.com/rehuony/sing-box-panel/internal/sing-box-panel/settings"
	"github.com/rehuony/sing-box-panel/internal/sing-box-panel/store"
)

const (
	stagePrefix = ".restore-"
	stageRecord = "staged.json"
	// pendingName marks the staged restore the panel applies before it next
	// opens its databases.
	pendingName = "restore.pending"
)

// ErrTooNew is a backup made by a newer panel than this one.
var ErrTooNew = errors.New("the backup was made by a newer panel version; update this panel first")

var stageID = regexp.MustCompile(`^[0-9]{1,20}$`)

// Staged is a verified backup waiting to be restored, as its preview shows
// it.
type Staged struct {
	ID       string   `json:"id"`
	Manifest Manifest `json:"manifest"`
	// Server is the listener, base path and origin of the settings the backup
	// holds, which a restore takes only when asked to; nil without settings.
	Server *settings.Server `json:"server,omitempty"`
	// SamePanel reports whether the backup is of the panel that runs here, so
	// that its nodes reconnect without enrolling again.
	SamePanel bool      `json:"same_panel"`
	StagedAt  time.Time `json:"staged_at"`
}

type pendingRestore struct {
	ID          string     `json:"id"`
	TakeService bool       `json:"take_service"`
	Categories  []Category `json:"categories"`
	RequestedAt time.Time  `json:"requested_at"`
}

// Stage reads an archive into a private folder of the backups directory and
// verifies it whole: its manifest and every file's digest, that its
// databases are intact, of this panel and no newer than it, and that its
// settings, if any, are valid here. An older panel database is brought to
// this binary's schema. Staging replaces any earlier staged restore that is
// not about to be applied.
func (service *Service) Stage(ctx context.Context, input io.Reader, passphrase string) (Staged, error) {
	if !service.mu.TryLock() {
		return Staged{}, ErrBusy
	}
	defer service.mu.Unlock()
	directory, err := service.ensureDirectory()
	if err != nil {
		return Staged{}, err
	}
	if err := service.discardStages(""); err != nil {
		return Staged{}, err
	}
	stage, err := os.MkdirTemp(directory, stagePrefix)
	if err != nil {
		return Staged{}, err
	}
	staged, err := service.stage(ctx, input, passphrase, stage)
	if err != nil {
		_ = os.RemoveAll(stage)
		return Staged{}, err
	}
	return staged, nil
}

func (service *Service) stage(ctx context.Context, input io.Reader, passphrase, stage string) (Staged, error) {
	manifest, err := readArchive(input, passphrase, stage)
	if err != nil {
		return Staged{}, err
	}
	snapshot, err := store.InspectSnapshot(ctx, stage)
	if errors.Is(err, database.ErrSchemaTooNew) {
		return Staged{}, ErrTooNew
	}
	if err != nil {
		return Staged{}, fmt.Errorf("%w: %v", ErrInvalid, err)
	}
	if snapshot.PanelID == "" {
		snapshot.PanelID = manifest.Snapshot.PanelID
	}
	manifest.Snapshot = snapshot
	if err := store.UpgradeSnapshot(ctx, stage); err != nil {
		return Staged{}, fmt.Errorf("%w: its database cannot be upgraded: %v", ErrInvalid, err)
	}
	staged := Staged{ID: strings.TrimPrefix(filepath.Base(stage), stagePrefix), Manifest: manifest, StagedAt: service.options.Now().UTC()}
	if manifest.Holds(CategorySettings) {
		local, err := settings.Read(service.options.SettingsPath)
		if err != nil {
			return Staged{}, err
		}
		restored, err := os.ReadFile(filepath.Join(stage, SettingsFile))
		if err != nil {
			return Staged{}, err
		}
		if _, _, err := settings.RestoredDocument(service.options.SettingsPath, local, restored, false); err != nil {
			return Staged{}, fmt.Errorf("%w: its settings are not valid here: %v", ErrInvalid, err)
		}
		_, taken, err := settings.RestoredDocument(service.options.SettingsPath, local, restored, true)
		if err != nil {
			return Staged{}, fmt.Errorf("%w: its settings are not valid here: %v", ErrInvalid, err)
		}
		staged.Server = &taken.Server
	}
	if service.options.PanelID != nil && snapshot.PanelID != "" {
		current, err := service.options.PanelID(ctx)
		staged.SamePanel = err == nil && current == snapshot.PanelID
	}
	encoded, err := json.Marshal(staged)
	if err != nil {
		return Staged{}, err
	}
	if err := os.WriteFile(filepath.Join(stage, stageRecord), encoded, 0o600); err != nil {
		return Staged{}, err
	}
	return staged, matchOwnerTree(stage, service.options.DataDir)
}

// StageStored stages a restore from a backup in the backups directory.
func (service *Service) StageStored(ctx context.Context, name, passphrase string) (Staged, error) {
	file, _, err := service.Open(name)
	if err != nil {
		return Staged{}, err
	}
	defer file.Close()
	return service.Stage(ctx, file, passphrase)
}

// Staged returns a staged restore.
func (service *Service) Staged(id string) (Staged, error) {
	return readStage(service.Directory(), id)
}

func readStage(directory, id string) (Staged, error) {
	if !stageID.MatchString(id) {
		return Staged{}, ErrNotFound
	}
	raw, err := os.ReadFile(filepath.Join(directory, stagePrefix+id, stageRecord))
	if errors.Is(err, os.ErrNotExist) {
		return Staged{}, ErrNotFound
	}
	if err != nil {
		return Staged{}, err
	}
	var staged Staged
	if err := jsonstrict.Decode(raw, maximumManifestBytes, &staged); err != nil || staged.ID != id {
		return Staged{}, ErrInvalid
	}
	return staged, nil
}

// Discard removes a staged restore that is not about to be applied.
func (service *Service) Discard(id string) error {
	if !service.mu.TryLock() {
		return ErrBusy
	}
	defer service.mu.Unlock()
	if _, err := service.Staged(id); err != nil {
		return err
	}
	if pending, err := readPending(service.Directory()); err == nil && pending.ID == id {
		return ErrBusy
	}
	return os.RemoveAll(filepath.Join(service.Directory(), stagePrefix+id))
}

// discardStages removes every staged restore but keep and the one about to
// be applied.
func (service *Service) discardStages(keep string) error {
	entries, err := os.ReadDir(service.Directory())
	if err != nil {
		return err
	}
	pending, _ := readPending(service.Directory())
	for _, entry := range entries {
		id, ok := strings.CutPrefix(entry.Name(), stagePrefix)
		if !ok || id == keep || id == pending.ID {
			continue
		}
		if err := os.RemoveAll(filepath.Join(service.Directory(), entry.Name())); err != nil {
			return err
		}
	}
	return nil
}

// Clean removes what an interrupted backup left behind. It runs before the
// panel makes any backup.
func (service *Service) Clean() error {
	entries, err := os.ReadDir(service.Directory())
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".create-") || strings.HasPrefix(entry.Name(), ".archive-") {
			if err := os.RemoveAll(filepath.Join(service.Directory(), entry.Name())); err != nil {
				return err
			}
		}
	}
	return nil
}

// selectable is categories in their order when the backup holds each.
func selectable(manifest Manifest, categories []Category) ([]Category, error) {
	normalized, err := NormalizeCategories(categories)
	if err != nil {
		return nil, err
	}
	for _, category := range normalized {
		if !manifest.Holds(category) {
			return nil, ErrCategories
		}
	}
	return normalized, nil
}

// Plan is what restoring some categories of a staged backup changes beyond
// them.
type Plan struct {
	Effects store.MergeEffects `json:"effects"`
	// SignsOut reports whether the restore ends every session, as restoring
	// the settings and their credentials does.
	SignsOut bool `json:"signs_out"`
}

// Plan counts what restoring categories of the staged restore id would
// change in the categories it keeps, without changing anything.
func (service *Service) Plan(ctx context.Context, id string, categories []Category) (Plan, error) {
	staged, err := service.Staged(id)
	if err != nil {
		return Plan{}, err
	}
	if categories, err = selectable(staged.Manifest, categories); err != nil {
		return Plan{}, err
	}
	effects, err := store.PlanMerge(ctx, service.options.DataDir, filepath.Join(service.Directory(), stagePrefix+id), databaseCategories(categories))
	if err != nil {
		return Plan{}, err
	}
	return Plan{Effects: effects, SignsOut: slices.Contains(categories, CategorySettings)}, nil
}

// RequestRestore backs up the panel as it is now, then marks categories of
// the staged restore id to be applied when the panel next starts, before it
// opens its databases. takeService also takes the backup's listener, base
// path and origin with its settings. It returns the backup made of the
// current state, from which a mistaken restore can be undone.
func (service *Service) RequestRestore(ctx context.Context, id string, takeService bool, categories []Category) (Info, error) {
	if !service.mu.TryLock() {
		return Info{}, ErrBusy
	}
	defer service.mu.Unlock()
	staged, err := service.Staged(id)
	if err != nil {
		return Info{}, err
	}
	if categories, err = selectable(staged.Manifest, categories); err != nil {
		return Info{}, err
	}
	// A restore can change every category of the database, so the backup holds them all.
	current := slices.Clone(DefaultCategories)
	if slices.Contains(categories, CategoryLogs) {
		current = append(current, CategoryLogs)
	}
	before, err := service.create(ctx, CreateOptions{Kind: KindPreRestore, Categories: current})
	if err != nil {
		return Info{}, fmt.Errorf("back up the current state before restoring: %w", err)
	}
	encoded, err := json.Marshal(pendingRestore{ID: id, TakeService: takeService, Categories: categories, RequestedAt: service.options.Now().UTC()})
	if err != nil {
		return Info{}, err
	}
	marker := filepath.Join(service.Directory(), pendingName)
	if err := settings.WriteAtomic(marker, encoded); err != nil {
		return Info{}, err
	}
	return before, matchOwner(marker, service.options.DataDir)
}

func readPending(directory string) (pendingRestore, error) {
	raw, err := os.ReadFile(filepath.Join(directory, pendingName))
	if err != nil {
		return pendingRestore{}, err
	}
	var pending pendingRestore
	if err := jsonstrict.Decode(raw, maximumManifestBytes, &pending); err != nil || !stageID.MatchString(pending.ID) {
		return pendingRestore{}, ErrInvalid
	}
	return pending, nil
}

// Applied is a restore ApplyPending carried out, or why it did not.
type Applied struct {
	Manifest    Manifest
	TakeService bool
	Categories  []Category
	Effects     store.MergeEffects
	// Skipped says why a requested restore was dropped instead.
	Skipped string
}

// ApplyPending applies the restore a running panel requested, while no
// process has the data directory's databases open: the caller holds the
// panel's lease and opens them afterwards. It replaces the chosen categories
// of the panel database and repairs references to what is gone, replaces the
// log database when chosen, and writes the restored settings with this
// installation's data directory, ending every session with them. A restore
// interrupted midway is completed by the next call. It returns nil when no
// restore was requested.
func ApplyPending(ctx context.Context, settingsPath, dataDir string) (*Applied, error) {
	directory := filepath.Join(dataDir, DirectoryName)
	marker := filepath.Join(directory, pendingName)
	pending, err := readPending(directory)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	skip := func(reason string) (*Applied, error) {
		if err := settings.RemoveDurable(marker); err != nil {
			return nil, err
		}
		return &Applied{Skipped: reason}, nil
	}
	if err != nil {
		return skip("the restore request could not be read")
	}
	staged, err := readStage(directory, pending.ID)
	if err != nil {
		return skip("the staged backup is gone")
	}
	categories, err := selectable(staged.Manifest, pending.Categories)
	if err != nil {
		return skip("the restore request names categories the backup does not hold")
	}
	stage := filepath.Join(directory, stagePrefix+pending.ID)
	lock, err := settingsfile.Lock(ctx, settingsPath)
	if err != nil {
		return nil, err
	}
	defer lock.Close()
	if err := settings.CheckPending(settingsPath); err != nil {
		return skip("an interrupted settings save needed recovery first")
	}
	restoreSettings := slices.Contains(categories, CategorySettings)
	var document []byte
	if restoreSettings {
		local, err := settings.Read(settingsPath)
		if err != nil {
			return nil, err
		}
		restored, err := os.ReadFile(filepath.Join(stage, SettingsFile))
		if err != nil {
			return nil, err
		}
		if document, _, err = settings.RestoredDocument(settingsPath, local, restored, pending.TakeService); err != nil {
			return skip("the backup's settings are not valid here")
		}
	}
	var effects store.MergeEffects
	if tables := databaseCategories(categories); len(tables) > 0 || restoreSettings {
		effects, err = store.MergeSnapshot(ctx, dataDir, stage, tables, restoreSettings)
		if err != nil {
			if ctx.Err() != nil {
				return nil, err
			}
			// The merge changed nothing; the panel starts as it was.
			return skip("the backup's records do not fit this panel's database")
		}
		for _, name := range []string{store.PanelFile, store.PanelFile + "-wal", store.PanelFile + "-shm"} {
			if err := matchOwner(filepath.Join(dataDir, name), dataDir); err != nil && !errors.Is(err, os.ErrNotExist) {
				return nil, err
			}
		}
	}
	if slices.Contains(categories, CategoryLogs) {
		source, target := filepath.Join(stage, store.LogsFile), filepath.Join(dataDir, store.LogsFile)
		// Gone when an interrupted apply already moved it.
		if _, err := os.Lstat(source); err == nil {
			// The replaced database's journal must never meet the new database.
			for _, sidecar := range []string{target + "-wal", target + "-shm"} {
				if err := os.Remove(sidecar); err != nil && !errors.Is(err, os.ErrNotExist) {
					return nil, err
				}
			}
			if err := os.Rename(source, target); err != nil {
				return nil, err
			}
			if err := matchOwner(target, dataDir); err != nil {
				return nil, err
			}
		} else if !errors.Is(err, os.ErrNotExist) {
			return nil, err
		}
	}
	if err := syncDirectory(dataDir); err != nil {
		return nil, err
	}
	if restoreSettings {
		if err := settings.WriteAtomic(settingsPath, document); err != nil {
			return nil, err
		}
	}
	if err := settings.RemoveDurable(marker); err != nil {
		return nil, err
	}
	if err := os.RemoveAll(stage); err != nil {
		return nil, err
	}
	return &Applied{Manifest: staged.Manifest, TakeService: pending.TakeService, Categories: categories, Effects: effects}, nil
}

func matchOwnerTree(root, reference string) error {
	return filepath.WalkDir(root, func(path string, _ os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		return matchOwner(path, reference)
	})
}
