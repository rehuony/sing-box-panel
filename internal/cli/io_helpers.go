// SPDX-License-Identifier: GPL-3.0-or-later

package cli

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/rehuony/sing-box-panel/internal/auth"
)

func readInputFile(stdin io.Reader, filePath string, maximum int64, label string) ([]byte, error) {
	var reader io.Reader
	var closeFile func() error
	if filePath == "-" {
		reader = stdin
	} else {
		file, err := os.Open(filePath)
		if err != nil {
			return nil, fmt.Errorf("open %s input: %w", label, err)
		}
		reader = file
		closeFile = file.Close
	}
	if closeFile != nil {
		defer closeFile()
	}
	data, err := io.ReadAll(io.LimitReader(reader, maximum+1))
	if err != nil {
		return nil, fmt.Errorf("read %s input: %w", label, err)
	}
	if int64(len(data)) > maximum {
		return nil, fmt.Errorf("%s input exceeds %d bytes", label, maximum)
	}
	return data, nil
}

// readPasswordStdin removes only the final line ending, never password spaces.
func readPasswordStdin(reader io.Reader) ([]byte, error) {
	data, err := io.ReadAll(io.LimitReader(reader, 515))
	if err != nil {
		clear(data)
		return nil, errors.New("could not read password")
	}
	if len(data) > 514 {
		clear(data)
		return nil, auth.ErrPassword
	}
	return bytes.TrimSuffix(bytes.TrimSuffix(data, []byte("\n")), []byte("\r")), nil
}
