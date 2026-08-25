// SPDX-License-Identifier: GPL-3.0-or-later

package artifactstore

import (
	"archive/tar"
	"bufio"
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	pathpkg "path"
	"strings"
)

func extractSingBox(
	ctx context.Context,
	archivePath, destinationPath string,
	limits Limits,
) error {
	archive, err := os.Open(archivePath)
	if err != nil {
		return fail(StepArchive, "open", err)
	}
	defer archive.Close()
	compressed := bufio.NewReader(archive)
	gzipReader, err := gzip.NewReader(compressed)
	if err != nil {
		return fail(StepArchive, "gzip", errors.Join(ErrArchive, err))
	}
	defer gzipReader.Close()
	gzipReader.Multistream(false)
	maximumTarBytes := limits.MaximumExpandedBytes + int64(limits.MaximumFiles+2)*1024
	expandedReader := &boundedExpandedReader{source: gzipReader, maximum: maximumTarBytes}
	tarReader := tar.NewReader(expandedReader)
	files := 0
	var expanded int64
	foundBinary := false
	for {
		if err := ctx.Err(); err != nil {
			return fail(StepArchive, "cancelled", err)
		}
		header, err := tarReader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fail(StepArchive, "tar", errors.Join(ErrArchive, err))
		}
		files++
		if files > limits.MaximumFiles {
			return fail(StepArchive, "file_count", errors.Join(ErrArchive, ErrTooLarge))
		}
		if err := validateArchivePath(header.Name, header.Typeflag == tar.TypeDir); err != nil {
			return fail(StepArchive, "unsafe_path", errors.Join(ErrArchive, err))
		}
		if header.Size < 0 || header.Size > limits.MaximumFileBytes || expanded > limits.MaximumExpandedBytes-header.Size {
			return fail(StepArchive, "expanded_size", errors.Join(ErrArchive, ErrTooLarge))
		}
		expanded += header.Size
		switch header.Typeflag {
		case tar.TypeDir:
			continue
		case tar.TypeReg, tar.TypeRegA:
		default:
			return fail(StepArchive, "unsafe_type", ErrArchive)
		}
		if pathpkg.Base(pathpkg.Clean(header.Name)) != "sing-box" {
			written, err := copyBounded(ctx, io.Discard, tarReader, header.Size)
			if err != nil || written != header.Size {
				return fail(StepArchive, "entry_read", errors.Join(ErrArchive, err))
			}
			continue
		}
		if foundBinary {
			return fail(StepArchive, "duplicate_binary", ErrArchive)
		}
		binary, err := os.OpenFile(destinationPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
		if err != nil {
			return fail(StepArchive, "binary_create", err)
		}
		written, copyErr := copyBounded(ctx, binary, tarReader, header.Size)
		var modeErr error
		if copyErr == nil && written == header.Size {
			modeErr = binary.Chmod(0o700)
		}
		syncErr := binary.Sync()
		closeErr := binary.Close()
		if copyErr != nil || written != header.Size || modeErr != nil || syncErr != nil || closeErr != nil {
			return fail(StepArchive, "binary_write", errors.Join(ErrArchive, copyErr, modeErr, syncErr, closeErr))
		}
		foundBinary = true
	}
	if err := validateCompressedTail(ctx, expandedReader, compressed); err != nil {
		return err
	}
	if !foundBinary {
		return fail(StepArchive, "binary_missing", ErrArchive)
	}
	return nil
}

func validateCompressedTail(ctx context.Context, expanded *boundedExpandedReader, compressed *bufio.Reader) error {
	buffer := make([]byte, 32<<10)
	for {
		if err := ctx.Err(); err != nil {
			return fail(StepArchive, "cancelled", err)
		}
		count, err := expanded.Read(buffer)
		for _, value := range buffer[:count] {
			if value != 0 {
				return fail(StepArchive, "trailing_data", ErrArchive)
			}
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			return fail(StepArchive, "gzip_checksum", errors.Join(ErrArchive, err))
		}
		if count == 0 {
			return fail(StepArchive, "gzip_progress", errors.Join(ErrArchive, io.ErrNoProgress))
		}
	}
	if _, err := compressed.Peek(1); err == nil {
		return fail(StepArchive, "multiple_gzip_members", ErrArchive)
	} else if !errors.Is(err, io.EOF) {
		return fail(StepArchive, "gzip_trailing_read", errors.Join(ErrArchive, err))
	}
	return nil
}

type boundedExpandedReader struct {
	source  io.Reader
	read    int64
	maximum int64
}

func (reader *boundedExpandedReader) Read(destination []byte) (int, error) {
	remaining := reader.maximum - reader.read
	if remaining < 0 {
		return 0, ErrTooLarge
	}
	if int64(len(destination)) > remaining+1 {
		destination = destination[:remaining+1]
	}
	count, err := reader.source.Read(destination)
	if reader.read+int64(count) > reader.maximum {
		return 0, ErrTooLarge
	}
	reader.read += int64(count)
	return count, err
}

func validateArchivePath(name string, directory bool) error {
	if name == "" || strings.Contains(name, "\\") || pathpkg.IsAbs(name) || strings.ContainsRune(name, 0) {
		return fmt.Errorf("unsafe archive path")
	}
	cleaned := pathpkg.Clean(name)
	if cleaned == "." || cleaned == ".." || strings.HasPrefix(cleaned, "../") {
		return fmt.Errorf("unsafe archive path")
	}
	trimmed := strings.TrimPrefix(name, "./")
	if directory {
		trimmed = strings.TrimSuffix(trimmed, "/")
	}
	if trimmed != cleaned {
		return fmt.Errorf("non-canonical archive path")
	}
	for _, component := range strings.Split(trimmed, "/") {
		if component == "" || component == "." || component == ".." {
			return fmt.Errorf("unsafe archive path component")
		}
	}
	return nil
}
