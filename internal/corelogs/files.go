// SPDX-License-Identifier: GPL-3.0-or-later
// Package corelogs retains bounded, private, sanitized native process output.
package corelogs

import (
	"bytes"
	"crypto/rand"
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

const maxChunkBytes = 64 << 10

type Policy struct {
	RetentionDays int
	MaxFiles      int
	MaxFileBytes  int64
}

func DefaultPolicy() Policy { return Policy{RetentionDays: 7, MaxFileBytes: 32 << 20} }

var fileName = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}-\d{3,20}\.log$`)
var ansi = regexp.MustCompile(`\x1b\[[0-?]*[ -/]*[@-~]`)
var ErrInvalidFile = errors.New("invalid core log file")
var ErrCurrentFile = errors.New("current UTC day's core logs cannot be deleted")

type File struct {
	Name      string    `json:"name"`
	Size      int64     `json:"size"`
	UpdatedAt time.Time `json:"updated_at"`
	Deletable bool      `json:"deletable"`
}
type Chunk struct {
	File       string `json:"file"`
	Text       string `json:"text"`
	NextOffset int64  `json:"next_offset"`
	Size       int64  `json:"size"`
	Generation string `json:"generation"`
	Reset      bool   `json:"reset,omitempty"`
}
type Files struct {
	dir         string
	mu          sync.Mutex
	now         func() time.Time
	policy      Policy
	active      string
	epoch       string
	generations map[string]string
}

func New(dataDir string) (*Files, error) {
	if dataDir == "" {
		return nil, errors.New("core log directory unavailable")
	}
	dir := filepath.Join(dataDir, "logs", "core")
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, err
	}
	return &Files{dir: dir, now: time.Now, policy: DefaultPolicy(), epoch: rand.Text(), generations: make(map[string]string)}, nil
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
	today := f.now().UTC().Format("2006-01-02")
	for _, entry := range entries {
		if _, valid := logFileIndex(entry.Name()); !valid || !entry.Type().IsRegular() {
			continue
		}
		info, err := entry.Info()
		if errors.Is(err, os.ErrNotExist) {
			continue // Retention or an explicit deletion may remove a listed file.
		}
		if err != nil {
			return nil, err
		}
		result = append(result, File{Name: entry.Name(), Size: info.Size(), UpdatedAt: info.ModTime().UTC(), Deletable: entry.Name()[:10] != today})
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

// Delete removes only a managed capture, never the native log.output file.
// Protect every file from the current UTC day, including rotated segments.
func (f *Files) Delete(name string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if _, valid := logFileIndex(name); !valid {
		return ErrInvalidFile
	}
	if name[:10] == f.now().UTC().Format("2006-01-02") {
		return ErrCurrentFile
	}
	root, err := os.OpenRoot(f.dir)
	if err != nil {
		return err
	}
	defer root.Close()
	info, err := root.Lstat(name)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return ErrInvalidFile
	}
	if err := root.Remove(name); err != nil {
		return err
	}
	delete(f.generations, name)
	return nil
}

// Clear truncates one managed capture, including today's active file. Keep the
// inode: collectors use O_APPEND, so even an already-open writer continues at
// the new end without holes or writing to an unlinked file. Clients must discard
// pending reads and restart their byte cursor at zero after a successful clear.
func (f *Files) Clear(name string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if _, valid := logFileIndex(name); !valid {
		return ErrInvalidFile
	}
	root, err := os.OpenRoot(f.dir)
	if err != nil {
		return err
	}
	defer root.Close()
	info, err := root.Lstat(name)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return ErrInvalidFile
	}
	// Validate the opened file before truncating, rather than using O_TRUNC.
	file, err := root.OpenFile(name, os.O_WRONLY, 0)
	if err != nil {
		return err
	}
	opened, err := file.Stat()
	if err != nil {
		return errors.Join(err, file.Close())
	}
	if !opened.Mode().IsRegular() || !os.SameFile(info, opened) {
		return errors.Join(ErrInvalidFile, file.Close())
	}
	truncateErr := file.Truncate(0)
	if truncateErr == nil {
		// Invalidate cursors even if Close reports an error after truncation.
		f.generations[name] = rand.Text()
	}
	return errors.Join(truncateErr, file.Close())
}

// Read pairs a byte offset with its file generation. A stale generation restarts
// at zero, even when new output has already grown beyond the previous offset.
// The owner must share this Files instance between readers, clearers and writers.
func (f *Files) Read(name string, offset int64, generation string) (Chunk, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
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
	currentGeneration := f.generations[name]
	if currentGeneration == "" {
		currentGeneration = f.epoch
	}
	reset := generation != "" && generation != currentGeneration
	if reset {
		offset = 0
	}
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
	return Chunk{File: name, Text: strings.ToValidUTF8(string(text), "�"), NextOffset: offset + int64(n), Size: size, Generation: currentGeneration, Reset: reset}, nil
}

// EnsureCurrentFile creates today's UTC capture even without process output,
// reuses the newest existing segment, and applies retention to empty files too.
// It never truncates output or triggers size rotation without a write.
func (f *Files) EnsureCurrentFile() error { return f.append(nil) }

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
		if len(data) > 0 && file.Size+int64(len(data)) > f.policy.MaxFileBytes {
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
	f.active = name
	return f.pruneLocked()
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
	buffer := []byte(output.String())
	for len(buffer) > 0 {
		end := len(buffer)
		if end > maxChunkBytes {
			end = bytes.LastIndexByte(buffer[:maxChunkBytes], '\n') + 1
		}
		if end == 0 {
			return 0, errors.New("sanitized log line exceeds chunk limit")
		}
		if err := w.files.append(buffer[:end]); err != nil {
			return 0, err
		}
		buffer = buffer[end:]
	}
	return len(data), nil
}
