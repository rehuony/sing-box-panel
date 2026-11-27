// SPDX-License-Identifier: GPL-3.0-or-later

// Package backup owns the panel's backups: archives of its settings and
// databases in its data directory, whole or by category, made on demand, on a
// schedule and before every restore, and restores of chosen categories that a
// panel stages while it runs and applies before it next opens its databases.
package backup

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"time"

	"filippo.io/age"

	"github.com/rehuony/sing-box-panel/internal/jsonstrict"
	"github.com/rehuony/sing-box-panel/internal/sing-box-panel/settings"
	"github.com/rehuony/sing-box-panel/internal/sing-box-panel/store"
)

const (
	// Format names a panel backup archive.
	Format = "sing-box-panel-backup"
	// Version 5 holds the categories it names, with a manifest that stays
	// readable when the rest is encrypted.
	Version = 5

	manifestName = "manifest.json"
	// payloadName is an encrypted archive's second entry: its manifest and
	// files as a compressed tar stream, encrypted with age.
	payloadName = "payload.tar.gz.age"
	// SettingsFile is the settings document in an archive.
	SettingsFile = "setting.json"

	maximumManifestBytes = 1 << 20
	// maximumDatabaseBytes bounds one database in an archive.
	maximumDatabaseBytes = 1 << 36
)

var (
	// ErrInvalid is an archive that is not a complete panel backup of a
	// supported version.
	ErrInvalid = errors.New("the file is not a complete panel backup of a supported version")
	// ErrPassphraseRequired is an encrypted archive read without a passphrase.
	ErrPassphraseRequired = errors.New("the backup is encrypted; enter its passphrase")
	// ErrPassphraseIncorrect is an encrypted archive the passphrase does not
	// open.
	ErrPassphraseIncorrect = errors.New("the passphrase does not open this backup")
)

// Kind is why a backup was made.
type Kind string

const (
	KindManual     Kind = "manual"
	KindScheduled  Kind = "scheduled"
	KindPreRestore Kind = "pre_restore"
)

func (kind Kind) valid() bool {
	return kind == KindManual || kind == KindScheduled || kind == KindPreRestore
}

// Category is a part of the panel that a backup can hold and a restore can
// replace on its own: the settings file, a category of the panel database,
// or the log database.
type Category string

const (
	CategorySettings      Category = "settings"
	CategoryNodes                  = Category(store.CategoryNodes)
	CategorySubscriptions          = Category(store.CategorySubscriptions)
	CategoryRuleSets               = Category(store.CategoryRuleSets)
	CategoryKeys                   = Category(store.CategoryKeys)
	CategoryHistory                = Category(store.CategoryHistory)
	CategoryLogs          Category = "logs"
)

// Categories are every category in the order they are shown.
var Categories = []Category{
	CategorySettings, CategoryNodes, CategorySubscriptions, CategoryRuleSets, CategoryKeys, CategoryHistory, CategoryLogs,
}

// DefaultCategories are what a backup holds unless asked otherwise:
// everything but the log, which is large and rebuilt by use.
var DefaultCategories = Categories[:len(Categories)-1]

// ErrCategories is a choice of categories that is empty, unknown, or not
// held by the backup.
var ErrCategories = errors.New("choose at least one category the backup holds")

// NormalizeCategories orders categories as Categories does, without
// repeats, and refuses an empty or unknown one.
func NormalizeCategories(categories []Category) ([]Category, error) {
	if len(categories) == 0 {
		return nil, ErrCategories
	}
	for _, category := range categories {
		if !slices.Contains(Categories, category) {
			return nil, ErrCategories
		}
	}
	return slices.DeleteFunc(slices.Clone(Categories), func(category Category) bool {
		return !slices.Contains(categories, category)
	}), nil
}

// databaseCategories are the categories of the panel database among
// categories.
func databaseCategories(categories []Category) []store.Category {
	var selected []store.Category
	for _, category := range store.Categories {
		if slices.Contains(categories, Category(category)) {
			selected = append(selected, category)
		}
	}
	return selected
}

// Manifest is an archive's first entry, readable without a passphrase: what
// it holds and the digest of each file, verified before a restore uses any
// of them. It names no setting or secret.
type Manifest struct {
	Format       string             `json:"format"`
	Version      int                `json:"version"`
	CreatedAt    time.Time          `json:"created_at"`
	Kind         Kind               `json:"kind"`
	PanelVersion string             `json:"panel_version"`
	Encrypted    bool               `json:"encrypted"`
	Categories   []Category         `json:"categories"`
	Snapshot     store.SnapshotInfo `json:"snapshot"`
	Files        []File             `json:"files"`
}

// File is one file of an archive.
type File struct {
	Name   string `json:"name"`
	Size   int64  `json:"size"`
	SHA256 string `json:"sha256"`
}

// Holds reports whether the archive holds category.
func (manifest Manifest) Holds(category Category) bool {
	return slices.Contains(manifest.Categories, category)
}

// files are the files that categories put in an archive.
func files(categories []Category) []string {
	var names []string
	if slices.Contains(categories, CategorySettings) {
		names = append(names, SettingsFile)
	}
	if len(databaseCategories(categories)) > 0 {
		names = append(names, store.PanelFile)
	}
	if slices.Contains(categories, CategoryLogs) {
		names = append(names, store.LogsFile)
	}
	return names
}

func (manifest Manifest) validate() error {
	if manifest.Format != Format || manifest.Version != Version || manifest.CreatedAt.IsZero() || !manifest.Kind.valid() {
		return ErrInvalid
	}
	if categories, err := NormalizeCategories(manifest.Categories); err != nil || !slices.Equal(categories, manifest.Categories) {
		return ErrInvalid
	}
	seen := make(map[string]struct{}, len(manifest.Files))
	for _, file := range manifest.Files {
		limit := int64(maximumDatabaseBytes)
		switch file.Name {
		case SettingsFile:
			limit = settings.MaximumBytes
		case store.PanelFile, store.LogsFile:
		default:
			return ErrInvalid
		}
		if _, duplicate := seen[file.Name]; duplicate {
			return ErrInvalid
		}
		seen[file.Name] = struct{}{}
		if digest, err := hex.DecodeString(file.SHA256); err != nil || len(digest) != sha256.Size || file.Size < 0 || file.Size > limit {
			return ErrInvalid
		}
	}
	expected := files(manifest.Categories)
	if len(seen) != len(expected) {
		return ErrInvalid
	}
	for _, name := range expected {
		if _, ok := seen[name]; !ok {
			return ErrInvalid
		}
	}
	return nil
}

// describe completes manifest with the size and digest of each file in dir.
func describe(manifest *Manifest, dir string, names []string) error {
	for _, name := range names {
		file, err := os.Open(filepath.Join(dir, name))
		if err != nil {
			return err
		}
		digest := sha256.New()
		size, err := io.Copy(digest, file)
		err = errors.Join(err, file.Close())
		if err != nil {
			return err
		}
		manifest.Files = append(manifest.Files, File{Name: name, Size: size, SHA256: hex.EncodeToString(digest.Sum(nil))})
	}
	return nil
}

// writeArchive writes an archive of manifest and its files from dir to
// output as a gzip-compressed tar stream: the manifest, then either the files
// or, with a passphrase, one payload holding the manifest again and the
// files, encrypted to the passphrase with age. The payload is built in dir.
func writeArchive(output io.Writer, manifest Manifest, dir, passphrase string) error {
	manifest.Encrypted = passphrase != ""
	encoded, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return err
	}
	if !manifest.Encrypted {
		return writeTar(output, gzip.DefaultCompression, manifest.CreatedAt, encoded, dir, manifest.Files)
	}
	payload, err := os.OpenFile(filepath.Join(dir, payloadName), os.O_RDWR|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}
	defer func() { _ = payload.Close(); _ = os.Remove(payload.Name()) }()
	recipient, err := age.NewScryptRecipient(passphrase)
	if err != nil {
		return err
	}
	encrypted, err := age.Encrypt(payload, recipient)
	if err != nil {
		return err
	}
	err = writeTar(encrypted, gzip.DefaultCompression, manifest.CreatedAt, encoded, dir, manifest.Files)
	if err := errors.Join(err, encrypted.Close()); err != nil {
		return err
	}
	size, err := payload.Seek(0, io.SeekEnd)
	if err != nil {
		return err
	}
	if _, err := payload.Seek(0, io.SeekStart); err != nil {
		return err
	}
	// The payload is ciphertext, which compression cannot shrink.
	return writeTar(output, gzip.NoCompression, manifest.CreatedAt, encoded, dir, nil, tarEntry{payloadName, size, payload})
}

type tarEntry struct {
	name   string
	size   int64
	reader io.Reader
}

// writeTar writes manifest and then files from dir, or the entries given,
// as a gzip-compressed tar stream.
func writeTar(output io.Writer, level int, modified time.Time, manifest []byte, dir string, files []File, entries ...tarEntry) (err error) {
	compressed, err := gzip.NewWriterLevel(output, level)
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, compressed.Close()) }()
	archive := tar.NewWriter(compressed)
	defer func() { err = errors.Join(err, archive.Close()) }()
	write := func(name string, size int64, reader io.Reader) error {
		header := &tar.Header{Typeflag: tar.TypeReg, Name: name, Size: size, Mode: 0o600, ModTime: modified, Format: tar.FormatPAX}
		if err := archive.WriteHeader(header); err != nil {
			return err
		}
		_, err := io.CopyN(archive, reader, size)
		return err
	}
	if err := write(manifestName, int64(len(manifest)), bytes.NewReader(manifest)); err != nil {
		return err
	}
	for _, entry := range files {
		file, err := os.Open(filepath.Join(dir, entry.Name))
		if err != nil {
			return err
		}
		err = write(entry.Name, entry.Size, file)
		if err := errors.Join(err, file.Close()); err != nil {
			return err
		}
	}
	for _, entry := range entries {
		if err := write(entry.name, entry.size, entry.reader); err != nil {
			return err
		}
	}
	return nil
}

// readManifestEntry reads the manifest that a tar stream starts with, and
// its bytes as written.
func readManifestEntry(archive *tar.Reader) (Manifest, []byte, error) {
	header, err := archive.Next()
	if err != nil || header.Name != manifestName || header.Typeflag != tar.TypeReg || header.Size <= 0 || header.Size > maximumManifestBytes {
		return Manifest{}, nil, ErrInvalid
	}
	raw, err := io.ReadAll(io.LimitReader(archive, header.Size))
	if err != nil {
		return Manifest{}, nil, errors.Join(ErrInvalid, err)
	}
	var manifest Manifest
	if err := jsonstrict.Decode(raw, maximumManifestBytes, &manifest); err != nil {
		return Manifest{}, nil, ErrInvalid
	}
	if err := manifest.validate(); err != nil {
		return Manifest{}, nil, err
	}
	return manifest, raw, nil
}

// readArchive reads an archive from input into dir, which must be empty,
// decrypting its payload with passphrase when it is encrypted. It accepts
// only a supported manifest first and then exactly its files, each of its
// size and digest, so that dir holds only what the manifest names when it
// succeeds; an encrypted payload must repeat the manifest unchanged.
func readArchive(input io.Reader, passphrase, dir string) (Manifest, error) {
	compressed, err := gzip.NewReader(input)
	if err != nil {
		return Manifest{}, ErrInvalid
	}
	archive := tar.NewReader(compressed)
	manifest, raw, err := readManifestEntry(archive)
	if err != nil {
		return Manifest{}, err
	}
	if manifest.Encrypted {
		if passphrase == "" {
			return Manifest{}, ErrPassphraseRequired
		}
		header, err := archive.Next()
		if err != nil || header.Name != payloadName || header.Typeflag != tar.TypeReg {
			return Manifest{}, ErrInvalid
		}
		identity, err := age.NewScryptIdentity(passphrase)
		if err != nil {
			return Manifest{}, ErrPassphraseIncorrect
		}
		decrypted, err := age.Decrypt(io.LimitReader(archive, header.Size), identity)
		if err != nil {
			return Manifest{}, ErrPassphraseIncorrect
		}
		if compressed, err = gzip.NewReader(decrypted); err != nil {
			return Manifest{}, ErrInvalid
		}
		archive = tar.NewReader(compressed)
		inner, innerRaw, err := readManifestEntry(archive)
		if err != nil || !bytes.Equal(innerRaw, raw) || !inner.Encrypted {
			return Manifest{}, ErrInvalid
		}
	}
	expected := make(map[string]File, len(manifest.Files))
	for _, file := range manifest.Files {
		expected[file.Name] = file
	}
	for len(expected) > 0 {
		header, err := archive.Next()
		if err != nil {
			return Manifest{}, errors.Join(ErrInvalid, err)
		}
		entry, ok := expected[header.Name]
		if !ok || header.Typeflag != tar.TypeReg || header.Size != entry.Size {
			return Manifest{}, ErrInvalid
		}
		delete(expected, header.Name)
		if err := extract(archive, filepath.Join(dir, entry.Name), entry); err != nil {
			return Manifest{}, err
		}
	}
	return manifest, nil
}

func extract(input io.Reader, path string, entry File) (err error) {
	output, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, output.Close()) }()
	digest := sha256.New()
	written, err := io.Copy(io.MultiWriter(output, digest), io.LimitReader(input, entry.Size))
	if err != nil {
		return errors.Join(ErrInvalid, err)
	}
	if written != entry.Size || hex.EncodeToString(digest.Sum(nil)) != entry.SHA256 {
		return fmt.Errorf("%w: %s does not match its digest", ErrInvalid, entry.Name)
	}
	return output.Sync()
}
