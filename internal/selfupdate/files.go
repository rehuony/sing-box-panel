// SPDX-License-Identifier: GPL-3.0-or-later

package selfupdate

import (
	"fmt"
	"io"
	"os"
)

func readBounded(reader io.Reader, maximum int64) ([]byte, error) {
	data, err := io.ReadAll(io.LimitReader(reader, maximum+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > maximum {
		return nil, fmt.Errorf("response exceeds %d bytes", maximum)
	}
	return data, nil
}

func syncDirectory(path string) error {
	directory, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("open executable directory: %w", err)
	}
	defer directory.Close()
	if err := directory.Sync(); err != nil {
		return fmt.Errorf("sync executable directory: %w", err)
	}
	return nil
}
