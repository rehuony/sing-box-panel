// SPDX-License-Identifier: GPL-3.0-or-later

package corelogs

import (
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// Follow copies only newly appended output from the saved configuration's log
// path. API consumers still see opaque captured-file IDs, never arbitrary paths.
// Existing file contents are not imported; retained logs use our normal bounded,
// private, sanitized writer. The native configuration itself is unchanged.
func (f *Files) Follow(config []byte, workingDir string) (io.Closer, error) {
	var value struct {
		Log struct {
			Disabled bool   `json:"disabled"`
			Output   string `json:"output"`
		} `json:"log"`
	}
	if err := json.Unmarshal(config, &value); err != nil {
		return nil, err
	}
	if value.Log.Disabled || value.Log.Output == "" {
		return nil, nil
	}
	path := value.Log.Output
	if !filepath.IsAbs(path) {
		path = filepath.Join(workingDir, path)
	}
	path = filepath.Clean(path)
	if parent, err := filepath.EvalSymlinks(filepath.Dir(path)); err == nil {
		path = filepath.Join(parent, filepath.Base(path))
	}
	captureDir := f.dir
	if resolved, err := filepath.EvalSymlinks(captureDir); err == nil {
		captureDir = resolved
	}
	// Refuse a capture feedback loop even if a caller names an existing capture.
	if rel, err := filepath.Rel(captureDir, path); err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return nil, ErrInvalidFile
	}
	tail := &fileFollower{path: path, writer: &writer{files: f}, stop: make(chan struct{}), done: make(chan struct{})}
	file, info, err := openOutputFile(path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	if file != nil {
		tail.info = info
		tail.offset = info.Size()
		_ = file.Close()
	}
	go tail.run()
	return tail, nil
}

type fileFollower struct {
	path       string
	writer     *writer
	info       os.FileInfo
	offset     int64
	stop, done chan struct{}
	once       sync.Once
	err        error
}

func (f *fileFollower) Close() error {
	f.once.Do(func() { close(f.stop) })
	<-f.done
	return f.err
}

func (f *fileFollower) run() {
	defer close(f.done)
	ticker := time.NewTicker(250 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-f.stop:
			// Bound the final drain independently of an external writer that might
			// continue appending after the owned process exits.
			f.err = errors.Join(f.err, f.readNew(32<<20), f.writer.Flush())
			return
		case <-ticker.C:
			if err := f.readNew(1 << 20); err != nil {
				f.err = err
				// Keep ownership until Close, but do not spin on an unsafe target.
				<-f.stop
				_ = f.writer.Flush()
				return
			}
		}
	}
}

func (f *fileFollower) readNew(limit int64) error {
	file, info, err := openOutputFile(f.path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	defer file.Close()
	if f.info == nil || !os.SameFile(f.info, info) || info.Size() < f.offset {
		if err := f.writer.Flush(); err != nil {
			return err
		}
		f.offset = 0
	}
	f.info = info
	remaining := min(limit, max(0, info.Size()-f.offset))
	buffer := make([]byte, min(int64(maxChunkBytes), remaining))
	for remaining > 0 {
		n, err := file.ReadAt(buffer[:min(int64(len(buffer)), remaining)], f.offset)
		if n > 0 {
			if _, writeErr := f.writer.Write(buffer[:n]); writeErr != nil {
				return writeErr
			}
			f.offset += int64(n)
			remaining -= int64(n)
		}
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return err
		}
	}
	return nil
}
