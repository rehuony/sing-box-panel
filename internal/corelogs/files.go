// SPDX-License-Identifier: GPL-3.0-or-later
// Package corelogs retains bounded, private, sanitized native process output.
package corelogs

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/rehuony/sing-box-panel/internal/store"
)

const maxFileBytes = 32 << 20
const maxChunkBytes = 64 << 10
const maxFiles = 32

var fileName = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}-\d{3,20}\.log$`)
var ansi = regexp.MustCompile(`\x1b\[[0-?]*[ -/]*[@-~]`)
var ErrInvalidFile = errors.New("invalid core log file")

type File struct {
	Name      string    `json:"name"`
	Size      int64     `json:"size"`
	UpdatedAt time.Time `json:"updated_at"`
}
type Chunk struct {
	File       string `json:"file"`
	Text       string `json:"text"`
	NextOffset int64  `json:"next_offset"`
	Size       int64  `json:"size"`
}
type Files struct {
	dir string
	mu  sync.Mutex
	now func() time.Time
}

func New(dataDir string) (*Files, error) {
	if dataDir == "" {
		return nil, errors.New("core log directory unavailable")
	}
	dir := filepath.Join(dataDir, "logs", "core")
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, err
	}
	return &Files{dir: dir, now: time.Now}, nil
}

func logFileIndex(name string) (uint64, bool) {
	if !fileName.MatchString(name) {
		return 0, false
	}
	sequence := name[11 : len(name)-4]
	index, err := strconv.ParseUint(sequence, 10, 64)
	return index, err == nil && sequence == fmt.Sprintf("%03d", index)
}

func (f *Files) List() ([]File, error) {
	root, err := os.OpenRoot(f.dir)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	dir, err := root.Open(".")
	if err != nil {
		return nil, err
	}
	defer dir.Close()
	entries, err := dir.ReadDir(-1)
	if err != nil {
		return nil, err
	}
	result := []File{}
	for _, entry := range entries {
		if _, valid := logFileIndex(entry.Name()); !valid || !entry.Type().IsRegular() {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			return nil, err
		}
		result = append(result, File{Name: entry.Name(), Size: info.Size(), UpdatedAt: info.ModTime().UTC()})
	}
	sort.Slice(result, func(i, j int) bool {
		left, right := result[i].Name, result[j].Name
		if left[:10] != right[:10] {
			return left[:10] > right[:10]
		}
		leftIndex, _ := logFileIndex(left)
		rightIndex, _ := logFileIndex(right)
		return leftIndex > rightIndex
	})
	return result, nil
}
func (f *Files) Read(name string, offset int64) (Chunk, error) {
	if _, valid := logFileIndex(name); !valid || offset < -1 {
		return Chunk{}, ErrInvalidFile
	}
	root, err := os.OpenRoot(f.dir)
	if err != nil {
		return Chunk{}, err
	}
	defer root.Close()
	info, err := root.Lstat(name)
	if err != nil {
		return Chunk{}, err
	}
	if !info.Mode().IsRegular() {
		return Chunk{}, ErrInvalidFile
	}
	file, err := root.Open(name)
	if err != nil {
		return Chunk{}, err
	}
	defer file.Close()
	info, err = file.Stat()
	if err != nil || !info.Mode().IsRegular() {
		return Chunk{}, ErrInvalidFile
	}
	size := info.Size()
	tail := offset < 0
	if tail {
		offset = max(0, size-maxChunkBytes)
	}
	if offset > size {
		return Chunk{}, ErrInvalidFile
	}
	buffer := make([]byte, min(maxChunkBytes, size-offset))
	n, err := file.ReadAt(buffer, offset)
	if err != nil && err != io.EOF {
		return Chunk{}, err
	}
	buffer = buffer[:n]
	// End each chunk at a complete line so JSON encoding never splits a UTF-8
	// rune. Pending writes remain available to the next cursor read.
	if end := bytes.LastIndexByte(buffer, '\n'); end >= 0 {
		buffer = buffer[:end+1]
	} else {
		buffer = nil
	}
	n = len(buffer)
	// Skip an incomplete prefix only for the first tail read. Later chunks are
	// reassembled by the reader, without duplicating or dropping bytes.
	text := buffer
	if tail && offset > 0 {
		if i := bytes.IndexByte(text, '\n'); i >= 0 {
			text = text[i+1:]
		} else {
			text = nil
		}
	}
	return Chunk{File: name, Text: strings.ToValidUTF8(string(text), "�"), NextOffset: offset + int64(n), Size: size}, nil
}
func (f *Files) append(data []byte) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	root, err := os.OpenRoot(f.dir)
	if err != nil {
		return err
	}
	defer root.Close()
	files, err := f.List()
	if err != nil {
		return err
	}
	prefix := f.now().UTC().Format("2006-01-02")
	var index uint64
	// Resume the newest file for this day, including after a panel restart.
	// Reusing a gap left by retention would immediately discard fresh output.
	for _, file := range files {
		if file.Name[:10] != prefix {
			continue
		}
		index, _ = logFileIndex(file.Name)
		if file.Size+int64(len(data)) > maxFileBytes {
			if index == ^uint64(0) {
				return errors.New("core log sequence exhausted")
			}
			index++
		}
		break
	}
	name := fmt.Sprintf("%s-%03d.log", prefix, index)
	info, err := root.Lstat(name)
	if err == nil && !info.Mode().IsRegular() {
		return ErrInvalidFile
	}
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	file, err := root.OpenFile(name, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	_, writeErr := file.Write(data)
	closeErr := file.Close()
	if err := errors.Join(writeErr, closeErr); err != nil {
		return err
	}
	files, err = f.List()
	if err != nil {
		return err
	}
	remaining := len(files) - maxFiles
	for i := len(files) - 1; i >= 0 && remaining > 0; i-- {
		// Never discard the active file, even if the system date moved backwards.
		if files[i].Name == name {
			continue
		}
		if err := root.Remove(files[i].Name); err != nil {
			return err
		}
		remaining--
	}
	return nil
}

type writer struct {
	files    *Files
	mu       sync.Mutex
	pending  []byte
	overflow bool
}

func (f *Files) Writer() io.Writer { return &writer{files: f} }

// Flush terminates a process's last partial line before the writer is reused.
func (w *writer) Flush() error {
	_, err := w.Write([]byte{'\n'})
	return err
}
func (w *writer) Write(data []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	var output strings.Builder
	for _, b := range data {
		if b == '\n' {
			line := ansi.ReplaceAllString(string(w.pending), "")
			if w.overflow {
				line = "[core output omitted: line exceeds limit]"
			}
			if strings.TrimSpace(line) != "" {
				output.WriteString(store.SanitizeCoreLogLine(line))
				output.WriteByte('\n')
			}
			w.pending = nil
			w.overflow = false
		} else if len(w.pending) < store.MaximumLogMessageBytes {
			w.pending = append(w.pending, b)
		} else {
			w.overflow = true
		}
	}
	if output.Len() > 0 {
		if err := w.files.append([]byte(output.String())); err != nil {
			return 0, err
		}
	}
	return len(data), nil
}
