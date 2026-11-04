// SPDX-License-Identifier: GPL-3.0-or-later

package host

import (
	"context"
	"errors"
	"io/fs"
	"net"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func fixtureFile(t *testing.T, path string) {
	t.Helper()
	if err := os.WriteFile(path, []byte("never returned by the browser"), 0600); err != nil {
		t.Fatal(err)
	}
}

func TestListAndResolvePaths(t *testing.T) {
	ctx := t.Context()
	base := t.TempDir()
	if err := os.Mkdir(filepath.Join(base, "z-dir"), 0700); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{".hidden", "a #证书.pem", "b.key", "A.pem"} {
		fixtureFile(t, filepath.Join(base, name))
	}
	if err := os.Symlink("a #证书.pem", filepath.Join(base, "link.pem")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("missing", filepath.Join(base, "broken")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("z-dir", filepath.Join(base, "dir-link")); err != nil {
		t.Fatal(err)
	}
	page, err := ListPaths(ctx, base, PathQuery{Limit: 2})
	if err != nil {
		t.Fatal(err)
	}
	if page.Total != 7 || len(page.Items) != 2 || page.Items[0].Name != "dir-link" || page.Items[1].Name != "z-dir" {
		t.Fatalf("directory-first listing: %+v", page)
	}
	filtered, err := ListPaths(ctx, base, PathQuery{Search: ".PEM", Limit: 1, Offset: 99})
	if err != nil || filtered.Total != 3 || filtered.Offset != 2 || len(filtered.Items) != 1 {
		t.Fatalf("filtered/clamped page: %+v %v", filtered, err)
	}
	hidden, err := ListPaths(ctx, base, PathQuery{ShowHidden: true})
	if err != nil || hidden.Total != 8 {
		t.Fatalf("hidden: %+v %v", hidden, err)
	}
	for _, item := range hidden.Items {
		if item.Name == "broken" && (item.Available || !item.Symlink) {
			t.Fatalf("broken link selectable: %+v", item)
		}
	}
	for _, path := range []string{"a #证书.pem", "link.pem"} {
		selected, err := ResolvePath(ctx, base, path, PathFile)
		if err != nil || !selected.Exists || selected.Path != filepath.Join(base, path) {
			t.Fatalf("resolve %q: %+v %v", path, selected, err)
		}
	}
	linked, err := ResolvePath(ctx, base, "dir-link", PathDirectory)
	if err != nil || !linked.Symlink || linked.Path != filepath.Join(base, "dir-link") {
		t.Fatalf("directory link: %+v %v", linked, err)
	}
	for _, path := range []string{"z-dir/new.log", "b.key"} {
		selected, err := ResolvePath(ctx, base, path, PathOutputFile)
		if err != nil || selected.Kind != "file" || selected.Exists != (path == "b.key") {
			t.Fatalf("output %q: %+v %v", path, selected, err)
		}
	}
	if _, err := os.Stat(filepath.Join(base, "z-dir/new.log")); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("selection created output: %v", err)
	}
}

func TestSelectionFailuresAndFallback(t *testing.T) {
	base := t.TempDir()
	fixtureFile(t, filepath.Join(base, "file"))
	if err := os.Symlink("absent", filepath.Join(base, "broken")); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		path string
		mode PathMode
		want error
	}{
		{"file", PathDirectory, ErrPathType}, {".", PathFile, ErrPathType}, {"missing", PathFile, fs.ErrNotExist},
		{"missing/new", PathOutputFile, fs.ErrNotExist}, {"broken", PathOutputFile, ErrBrokenLink},
		{".", PathOutputFile, ErrInvalidPath}, {"..", PathOutputFile, ErrInvalidPath}, {"file/", PathOutputFile, ErrInvalidPath},
		{"", PathFile, ErrInvalidPath}, {"x\x00y", PathFile, ErrInvalidPath}, {"file", "wrong", ErrInvalidPath},
	} {
		t.Run(tc.path+string(tc.mode), func(t *testing.T) {
			if _, err := ResolvePath(t.Context(), base, tc.path, tc.mode); !errors.Is(err, tc.want) {
				t.Fatalf("got %v, want %v", err, tc.want)
			}
		})
	}
	for _, path := range []string{"missing/nested/file", "broken", "file"} {
		page, err := ListPaths(t.Context(), base, PathQuery{Path: path})
		if err != nil || page.Path != base || page.Fallback != (path != "file") {
			t.Fatalf("fallback %q: %+v %v", path, page, err)
		}
	}
	page, err := ListPaths(t.Context(), filepath.Join(base, "missing/runtime"), PathQuery{})
	if err != nil || page.Path != base || !page.Fallback {
		t.Fatalf("uninitialized runtime: %+v %v", page, err)
	}
	for _, q := range []PathQuery{{Limit: 201}, {Offset: -1}, {Path: "bad\x00path"}} {
		if _, err := ListPaths(t.Context(), base, q); !errors.Is(err, ErrInvalidPath) {
			t.Fatalf("invalid query: %v", err)
		}
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := ListPaths(ctx, base, PathQuery{}); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	if _, err := ResolvePath(ctx, base, "file", PathFile); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	empty := t.TempDir()
	page, err = ListPaths(t.Context(), empty, PathQuery{Offset: 100})
	if err != nil || !reflect.DeepEqual(page.Items, []PathEntry{}) || page.Offset != 0 {
		t.Fatalf("empty listing: %+v %v", page, err)
	}
}

func TestPermissionErrorsDoNotAscend(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root bypasses directory permissions")
	}
	base := t.TempDir()
	denied := filepath.Join(base, "denied")
	if err := os.Mkdir(denied, 0000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(denied, 0700) })
	for _, path := range []string{denied, filepath.Join(denied, "file")} {
		if _, err := ListPaths(t.Context(), base, PathQuery{Path: path}); !errors.Is(err, fs.ErrPermission) {
			t.Fatalf("permission error: %v", err)
		}
	}
}

func TestSocketSelection(t *testing.T) {
	// Unix socket paths have a smaller limit than ordinary paths on macOS.
	base, err := os.MkdirTemp("/tmp", "sbp-path-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(base) })
	listener, err := net.Listen("unix", filepath.Join(base, "socket"))
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	selected, err := ResolvePath(t.Context(), base, "socket", PathSocket)
	if err != nil || selected.Kind != "socket" {
		t.Fatalf("socket: %+v %v", selected, err)
	}
	if _, err := ResolvePath(t.Context(), base, "socket", PathFile); !errors.Is(err, ErrPathType) {
		t.Fatal(err)
	}
}

func TestDotComponentsPreserveSymlinkTargets(t *testing.T) {
	base := t.TempDir()
	real := filepath.Join(base, "real")
	if err := os.MkdirAll(filepath.Join(real, "nested"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(real, "nested"), filepath.Join(base, "alias")); err != nil {
		t.Fatal(err)
	}
	fixtureFile(t, filepath.Join(base, "key.pem"))
	fixtureFile(t, filepath.Join(real, "key.pem"))
	want, err := os.Stat(filepath.Join(real, "key.pem"))
	if err != nil {
		t.Fatal(err)
	}
	for _, prefix := range []string{"alias/..", base + "/alias/.."} {
		t.Run(prefix, func(t *testing.T) {
			selected, err := ResolvePath(t.Context(), base, prefix+"/key.pem", PathFile)
			if err != nil {
				t.Fatal(err)
			}
			actual, err := os.Stat(selected.Path)
			if err != nil || !os.SameFile(want, actual) {
				t.Fatalf("selection changed target: %+v %v", selected, err)
			}
			page, err := ListPaths(t.Context(), base, PathQuery{Path: prefix})
			if err != nil || len(page.Items) != 2 {
				t.Fatalf("wrong directory listing: %+v %v", page, err)
			}
			listed, err := ResolvePath(t.Context(), base, page.Items[1].Path, PathFile)
			if err != nil || listed.Path != selected.Path {
				t.Fatalf("listed entry changed target: %+v %v", listed, err)
			}
			output, err := ResolvePath(t.Context(), base, prefix+"/new.log", PathOutputFile)
			if err != nil || output.Exists || output.Parent != page.Path {
				t.Fatalf("output parent changed: %+v %v", output, err)
			}
			fallback, err := ListPaths(t.Context(), base, PathQuery{Path: prefix + "/missing/child"})
			if err != nil || !fallback.Fallback || fallback.Path != page.Path {
				t.Fatalf("fallback changed parent: %+v %v", fallback, err)
			}
		})
	}
	if _, err := ResolvePath(t.Context(), base, "missing/../key.pem", PathFile); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("missing component was erased: %v", err)
	}
}
