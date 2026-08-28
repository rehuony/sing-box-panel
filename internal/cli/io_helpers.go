// SPDX-License-Identifier: GPL-3.0-or-later

package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
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

func prettyJSON(value any) string {
	var output bytes.Buffer
	encoder := json.NewEncoder(&output)
	encoder.SetEscapeHTML(false)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(value); err != nil {
		return fmt.Sprintf("%v", value)
	}
	return strings.TrimSuffix(output.String(), "\n")
}
