// SPDX-License-Identifier: GPL-3.0-or-later

// Filesystem selection is read-only and non-recursive. Its authority is the
// node process's filesystem namespace and OS permissions.

package host

import (
	"context"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"unicode/utf8"
)

var (
	ErrInvalidPath       = errors.New("invalid filesystem selection")
	ErrPathType          = errors.New("filesystem target has the wrong type")
	ErrBrokenLink        = errors.New("filesystem target is a broken symbolic link")
	ErrDirectoryTooLarge = errors.New("directory contains too many entries")
)

type PathMode string

const (
	PathFile                PathMode = "file"
	PathDirectory           PathMode = "directory"
	PathOutputFile          PathMode = "output-file"
	PathSocket              PathMode = "socket"
	maximumDirectoryEntries          = 20000
)

func (mode PathMode) Valid() bool {
	return mode == PathFile || mode == PathDirectory || mode == PathOutputFile || mode == PathSocket
}

type PathEntry struct {
	Name      string `json:"name"`
	Path      string `json:"path"`
	Kind      string `json:"kind"`
	Symlink   bool   `json:"symlink"`
	Available bool   `json:"available"`
}

type PathQuery struct {
	Path       string
	Search     string
	ShowHidden bool
	Offset     int
	Limit      int
}

type PathPage struct {
	Path          string      `json:"path"`
	Parent        string      `json:"parent"`
	RequestedPath string      `json:"requested_path"`
	Fallback      bool        `json:"fallback"`
	Items         []PathEntry `json:"items"`
	Total         int         `json:"total"`
	Offset        int         `json:"offset"`
	Limit         int         `json:"limit"`
}

type PathSelection struct {
	Path    string `json:"path"`
	Parent  string `json:"parent"`
	Kind    string `json:"kind"`
	Exists  bool   `json:"exists"`
	Symlink bool   `json:"symlink"`
}

func absolute(base, path string) (string, error) {
	if !filepath.IsAbs(base) || len(path) > 4096 || !utf8.ValidString(path) || strings.ContainsRune(path, 0) {
		return "", ErrInvalidPath
	}
	if path == "" {
		path = base
	} else if !filepath.IsAbs(path) {
		path = appendPath(base, path)
	}
	// Let the OS resolve dot components. Cleaning a path lexically changes the
	// target of link/../file when link points outside its containing directory.
	return path, nil
}

func appendPath(directory, name string) string {
	return strings.TrimRight(directory, string(filepath.Separator)) + string(filepath.Separator) + name
}

// parentPath removes only the final component, preserving links and dot
// components in the prefix just as they appeared in the selected path.
func parentPath(path string) string {
	directory, _ := filepath.Split(strings.TrimRight(path, string(filepath.Separator)))
	directory = strings.TrimRight(directory, string(filepath.Separator))
	if directory == "" {
		return string(filepath.Separator)
	}
	return directory
}

func kind(info fs.FileInfo) string {
	switch {
	case info.IsDir():
		return "directory"
	case info.Mode().IsRegular():
		return "file"
	case info.Mode()&fs.ModeSocket != 0:
		return "socket"
	default:
		return "other"
	}
}

// inspect follows links for type checks but never changes the selected spelling.
// In particular, a missing symlink target is not a new output-file destination.
func inspect(path string) (fs.FileInfo, bool, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return nil, false, err
	}
	link := info.Mode()&fs.ModeSymlink != 0
	if link {
		info, err = os.Stat(path)
		if errors.Is(err, fs.ErrNotExist) {
			err = ErrBrokenLink
		}
	}
	return info, link, err
}

// ListPaths locates an existing directory from a current field value. Missing paths
// ascend to their nearest existing ancestor; permission failures never do.
func ListPaths(ctx context.Context, base string, query PathQuery) (PathPage, error) {
	if query.Offset < 0 || query.Limit < 0 || query.Limit > 200 || len(query.Search) > 256 {
		return PathPage{}, ErrInvalidPath
	}
	if query.Limit == 0 {
		query.Limit = 50
	}
	requested, err := absolute(base, query.Path)
	if err != nil {
		return PathPage{}, err
	}
	directory := requested
	fallback := false
	for {
		if err := ctx.Err(); err != nil {
			return PathPage{}, err
		}
		info, _, err := inspect(directory)
		if err == nil && info.IsDir() {
			break
		}
		if err != nil && !errors.Is(err, fs.ErrNotExist) && !errors.Is(err, ErrBrokenLink) {
			return PathPage{}, err
		}
		fallback = fallback || err != nil
		parent := parentPath(directory)
		if parent == directory {
			return PathPage{}, err
		}
		directory = parent
	}
	handle, err := os.Open(directory)
	if err != nil {
		return PathPage{}, err
	}
	defer handle.Close()
	items := make([]PathEntry, 0)
	search := strings.ToLower(query.Search)
	count := 0
	for {
		if err := ctx.Err(); err != nil {
			return PathPage{}, err
		}
		entries, err := handle.ReadDir(256)
		if err != nil && !errors.Is(err, io.EOF) {
			return PathPage{}, err
		}
		count += len(entries)
		if count > maximumDirectoryEntries {
			return PathPage{}, ErrDirectoryTooLarge
		}
		for _, entry := range entries {
			if err := ctx.Err(); err != nil {
				return PathPage{}, err
			}
			name := entry.Name()
			// JSON cannot round-trip Unix filenames containing invalid UTF-8.
			if !utf8.ValidString(name) || (!query.ShowHidden && strings.HasPrefix(name, ".")) || !strings.Contains(strings.ToLower(name), search) {
				continue
			}
			path := appendPath(directory, name)
			info, link, inspectErr := inspect(path)
			item := PathEntry{Name: name, Path: path, Kind: "other", Symlink: link, Available: inspectErr == nil}
			if inspectErr == nil {
				item.Kind = kind(info)
			}
			items = append(items, item)
		}
		if errors.Is(err, io.EOF) {
			break
		}
	}
	slices.SortFunc(items, func(a, b PathEntry) int {
		if (a.Kind == "directory") != (b.Kind == "directory") {
			if a.Kind == "directory" {
				return -1
			}
			return 1
		}
		return strings.Compare(a.Name, b.Name)
	})
	total := len(items)
	if query.Offset >= total {
		query.Offset = max(0, (total-1)/query.Limit*query.Limit)
	}
	return PathPage{Path: directory, Parent: parentPath(directory), RequestedPath: requested, Fallback: fallback,
		Items: items[query.Offset:min(total, query.Offset+query.Limit)], Total: total, Offset: query.Offset, Limit: query.Limit}, nil
}

// ResolvePath checks only existence and target type. It does not read contents,
// create output files, or promise the path will still be valid at core startup.
func ResolvePath(ctx context.Context, base, path string, mode PathMode) (PathSelection, error) {
	if err := ctx.Err(); err != nil {
		return PathSelection{}, err
	}
	if !mode.Valid() || path == "" {
		return PathSelection{}, ErrInvalidPath
	}
	if mode == PathOutputFile && (strings.HasSuffix(path, string(filepath.Separator)) || filepath.Base(path) == "." || filepath.Base(path) == "..") {
		return PathSelection{}, ErrInvalidPath
	}
	absolutePath, err := absolute(base, path)
	if err != nil {
		return PathSelection{}, err
	}
	info, link, err := inspect(absolutePath)
	result := PathSelection{Path: absolutePath, Parent: parentPath(absolutePath), Symlink: link, Exists: err == nil, Kind: "file"}
	if mode == PathOutputFile && errors.Is(err, fs.ErrNotExist) {
		parent, _, parentErr := inspect(result.Parent)
		if parentErr != nil {
			return PathSelection{}, parentErr
		}
		if !parent.IsDir() {
			return PathSelection{}, ErrPathType
		}
		return result, ctx.Err()
	}
	if err != nil {
		return PathSelection{}, err
	}
	result.Kind = kind(info)
	expected := string(mode)
	if mode == PathOutputFile {
		expected = "file"
	}
	if result.Kind != expected {
		return PathSelection{}, ErrPathType
	}
	return result, ctx.Err()
}
