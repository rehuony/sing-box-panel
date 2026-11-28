// SPDX-License-Identifier: GPL-3.0-or-later

package backup

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/rehuony/sing-box-panel/internal/database"
	"github.com/rehuony/sing-box-panel/internal/sing-box-panel/store"
)

// DirectoryName is the folder of a data directory that holds its backups.
const DirectoryName = "backups"

var (
	// ErrNotFound is a backup or staged restore that does not exist.
	ErrNotFound = errors.New("backup not found")
	// ErrBusy is a backup or restore asked for while another runs.
	ErrBusy = errors.New("another backup or restore is running")
)

// archiveName is a backup's file name: when it was made, a sequence number
// when several were made in one second, and why.
var archiveName = regexp.MustCompile(`^sing-box-panel-([0-9]{8}-[0-9]{6})(?:-([0-9]{1,3}))?-(manual|scheduled|pre_restore)\.tar\.gz$`)

const nameTime = "20060102-150405"

// Info describes one backup in the backups directory, as its manifest
// does; Categories is empty for an archive whose manifest cannot be read.
type Info struct {
	Name       string               `json:"name"`
	Kind       Kind                 `json:"kind"`
	CreatedAt  time.Time            `json:"created_at"`
	Size       int64                `json:"size"`
	Encrypted  bool                 `json:"encrypted"`
	Categories []Category           `json:"categories"`
	Counts     store.SnapshotCounts `json:"counts"`
}

// Options are what a Service needs of the panel it backs up.
type Options struct {
	DataDir      string
	SettingsPath string
	// PanelVersion is the build that makes a backup.
	PanelVersion string
	// Document reads the saved settings document as a backup holds it.
	Document func(context.Context) ([]byte, error)
	// PanelID reads the identity nodes bind to now; empty when unknown.
	PanelID func(context.Context) (string, error)
	Now     func() time.Time
}

// Service makes, lists and removes the backups of one panel's data
// directory, and stages restores from them or from an uploaded archive.
type Service struct {
	options Options
	// mu admits one backup or staged restore at a time.
	mu sync.Mutex
}

func New(options Options) *Service {
	if options.Now == nil {
		options.Now = time.Now
	}
	return &Service{options: options}
}

// Directory is the folder that holds the backups.
func (service *Service) Directory() string {
	return filepath.Join(service.options.DataDir, DirectoryName)
}

func (service *Service) ensureDirectory() (string, error) {
	directory := service.Directory()
	if err := os.Mkdir(directory, 0o700); err != nil && !errors.Is(err, os.ErrExist) {
		return "", err
	}
	if err := matchOwner(directory, service.options.DataDir); err != nil {
		return "", err
	}
	return directory, nil
}

// CreateOptions describe one backup.
type CreateOptions struct {
	// Kind is why the backup is made; a scheduled one holds its history
	// without the raw traffic samples.
	Kind Kind
	// Categories are what the backup holds.
	Categories []Category
	// Passphrase encrypts the archive with age when it is not empty.
	Passphrase string
}

// Create backs up the categories asked for, the settings as saved, into a new
// archive in the backups directory.
func (service *Service) Create(ctx context.Context, options CreateOptions) (Info, error) {
	if !service.mu.TryLock() {
		return Info{}, ErrBusy
	}
	defer service.mu.Unlock()
	return service.create(ctx, options)
}

func (service *Service) create(ctx context.Context, options CreateOptions) (Info, error) {
	if !options.Kind.valid() {
		return Info{}, fmt.Errorf("unknown backup kind %q", options.Kind)
	}
	categories, err := NormalizeCategories(options.Categories)
	if err != nil {
		return Info{}, err
	}
	directory, err := service.ensureDirectory()
	if err != nil {
		return Info{}, err
	}
	staging, err := os.MkdirTemp(directory, ".create-")
	if err != nil {
		return Info{}, err
	}
	defer os.RemoveAll(staging)
	if slices.Contains(categories, CategorySettings) {
		document, err := service.options.Document(ctx)
		if err != nil {
			return Info{}, fmt.Errorf("read saved settings: %w", err)
		}
		if err := os.WriteFile(filepath.Join(staging, SettingsFile), document, 0o600); err != nil {
			return Info{}, err
		}
	}
	snapshot, err := store.Snapshot(ctx, service.options.DataDir, staging, databaseCategories(categories),
		slices.Contains(categories, CategoryLogs), options.Kind == KindScheduled)
	if err != nil {
		return Info{}, err
	}
	if snapshot.PanelID == "" && service.options.PanelID != nil {
		snapshot.PanelID, _ = service.options.PanelID(ctx)
	}
	createdAt := service.options.Now().UTC().Truncate(time.Second)
	manifest := Manifest{
		Format: Format, Version: Version, CreatedAt: createdAt, Kind: options.Kind,
		PanelVersion: service.options.PanelVersion, Categories: categories, Snapshot: snapshot,
	}
	if err := describe(&manifest, staging, files(categories)); err != nil {
		return Info{}, err
	}
	temporary, err := os.CreateTemp(directory, ".archive-")
	if err != nil {
		return Info{}, err
	}
	defer os.Remove(temporary.Name())
	err = writeArchive(temporary, manifest, staging, options.Passphrase)
	if err == nil {
		err = temporary.Sync()
	}
	if err := errors.Join(err, temporary.Close()); err != nil {
		return Info{}, fmt.Errorf("write backup archive: %w", err)
	}
	if err := matchOwner(temporary.Name(), service.options.DataDir); err != nil {
		return Info{}, err
	}
	name, err := publish(directory, temporary.Name(), createdAt, options.Kind)
	if err != nil {
		return Info{}, err
	}
	return service.stat(name)
}

// publish links the finished archive under the first free name of its
// second, so that no backup ever replaces another.
func publish(directory, temporary string, createdAt time.Time, kind Kind) (string, error) {
	for sequence := 0; sequence < 1000; sequence++ {
		name := "sing-box-panel-" + createdAt.Format(nameTime)
		if sequence > 0 {
			name += "-" + strconv.Itoa(sequence)
		}
		name += "-" + string(kind) + ".tar.gz"
		err := os.Link(temporary, filepath.Join(directory, name))
		if err == nil {
			return name, syncDirectory(directory)
		}
		if !errors.Is(err, os.ErrExist) {
			return "", err
		}
	}
	return "", errors.New("too many backups in one second")
}

// List returns the backups, newest first.
func (service *Service) List(ctx context.Context) ([]Info, error) {
	entries, err := os.ReadDir(service.Directory())
	if errors.Is(err, os.ErrNotExist) {
		return []Info{}, nil
	}
	if err != nil {
		return nil, err
	}
	backups := make([]Info, 0, len(entries))
	for _, entry := range entries {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if !archiveName.MatchString(entry.Name()) || !entry.Type().IsRegular() {
			continue
		}
		info, err := service.stat(entry.Name())
		if err != nil {
			continue
		}
		backups = append(backups, info)
	}
	slices.SortFunc(backups, func(left, right Info) int {
		if comparison := right.CreatedAt.Compare(left.CreatedAt); comparison != 0 {
			return comparison
		}
		return strings.Compare(right.Name, left.Name)
	})
	return backups, nil
}

func (service *Service) stat(name string) (Info, error) {
	match := archiveName.FindStringSubmatch(name)
	if match == nil {
		return Info{}, ErrNotFound
	}
	createdAt, err := time.ParseInLocation(nameTime, match[1], time.UTC)
	if err != nil {
		return Info{}, ErrNotFound
	}
	path := filepath.Join(service.Directory(), name)
	file, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return Info{}, ErrNotFound
	}
	if err != nil {
		return Info{}, err
	}
	if !file.Mode().IsRegular() {
		return Info{}, ErrNotFound
	}
	info := Info{Name: name, Kind: Kind(match[3]), CreatedAt: createdAt, Size: file.Size(), Categories: []Category{}}
	if manifest, err := readManifest(path); err == nil {
		info.Encrypted, info.Categories, info.Counts = manifest.Encrypted, manifest.Categories, manifest.Snapshot.Counts
	}
	return info, nil
}

// readManifest reads the manifest of an archive, encrypted or not, without
// reading its files.
func readManifest(path string) (Manifest, error) {
	file, err := os.Open(path)
	if err != nil {
		return Manifest{}, err
	}
	defer file.Close()
	compressed, err := gzip.NewReader(file)
	if err != nil {
		return Manifest{}, ErrInvalid
	}
	manifest, _, err := readManifestEntry(tar.NewReader(compressed))
	return manifest, err
}

// Open opens a backup to read it, such as to download it.
func (service *Service) Open(name string) (*os.File, Info, error) {
	info, err := service.stat(name)
	if err != nil {
		return nil, Info{}, err
	}
	file, err := os.Open(filepath.Join(service.Directory(), name))
	if err != nil {
		return nil, Info{}, err
	}
	return file, info, nil
}

// Delete removes a backup.
func (service *Service) Delete(name string) error {
	if _, err := service.stat(name); err != nil {
		return err
	}
	if err := os.Remove(filepath.Join(service.Directory(), name)); err != nil {
		return err
	}
	return syncDirectory(service.Directory())
}

// LatestScheduled returns when the newest scheduled backup was made.
func (service *Service) LatestScheduled(ctx context.Context) (time.Time, bool, error) {
	backups, err := service.List(ctx)
	if err != nil {
		return time.Time{}, false, err
	}
	for _, backup := range backups {
		if backup.Kind == KindScheduled {
			return backup.CreatedAt, true, nil
		}
	}
	return time.Time{}, false, nil
}

// PruneScheduled removes the scheduled backups beyond the newest keep.
// Manual backups and those made before a restore stay until removed.
func (service *Service) PruneScheduled(ctx context.Context, keep int) (int, error) {
	backups, err := service.List(ctx)
	if err != nil {
		return 0, err
	}
	removed := 0
	for _, backup := range backups {
		if backup.Kind != KindScheduled {
			continue
		}
		if keep > 0 {
			keep--
			continue
		}
		if err := os.Remove(filepath.Join(service.Directory(), backup.Name)); err != nil && !errors.Is(err, os.ErrNotExist) {
			return removed, err
		}
		removed++
	}
	if removed > 0 {
		return removed, syncDirectory(service.Directory())
	}
	return 0, nil
}

func syncDirectory(directory string) error {
	file, err := os.Open(directory)
	if err != nil {
		return err
	}
	defer file.Close()
	return file.Sync()
}

// ClassifyOperationFailure names a backup or restore failure for the panel
// log, in stable codes and texts that never hold a path or passphrase.
func ClassifyOperationFailure(err error) (code, message string, ok bool) {
	for _, known := range []struct {
		cause error
		code  string
	}{
		{ErrPassphraseRequired, "backup_passphrase_required"},
		{ErrCategories, "backup_categories_invalid"},
		{ErrPassphraseIncorrect, "backup_passphrase_incorrect"},
		{ErrTooNew, "backup_too_new"},
		{ErrInvalid, "backup_invalid"},
		{ErrBusy, "backup_busy"},
		{ErrNotFound, "backup_not_found"},
		{database.ErrSnapshotNeedsRecovery, "backup_database_needs_recovery"},
	} {
		if errors.Is(err, known.cause) {
			return known.code, known.cause.Error(), true
		}
	}
	return "", "", false
}
