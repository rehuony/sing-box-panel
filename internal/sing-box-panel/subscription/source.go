// SPDX-License-Identifier: GPL-3.0-or-later

// Source parsing normalizes third-party subscription candidates without
// performing network I/O.
package subscription

import (
	"bytes"
	"errors"
	"fmt"
	"unicode/utf8"
)

type SourceFormat string

const (
	SourceFormatAuto        SourceFormat = "auto"
	SourceFormatSingBoxJSON SourceFormat = "sing-box-json"
	SourceFormatMihomoYAML  SourceFormat = "mihomo-yaml"
	SourceFormatURIList     SourceFormat = "uri-list"
)

var ErrInvalidSource = errors.New("invalid subscription source")

// Parse normalizes one complete third-party candidate. Any syntax,
// identity, or required-field error rejects the whole version.
func ParseSource(format SourceFormat, raw []byte, sourceID string) ([]Node, SourceFormat, error) {
	if len(raw) == 0 || len(raw) > MaximumDocumentBytes || !utf8.Valid(raw) || !validNodeSourceID(sourceID) {
		return nil, "", ErrInvalidSource
	}
	detected := format
	if format == SourceFormatAuto {
		detected = detectSourceFormat(raw)
	}
	var values []map[string]any
	var err error
	switch detected {
	case SourceFormatSingBoxJSON:
		values, err = parseSingBoxSource(raw)
	case SourceFormatMihomoYAML:
		values, err = parseMihomoSource(raw)
	case SourceFormatURIList:
		values, err = parseURIListSource(raw)
	default:
		return nil, "", ErrInvalidSource
	}
	if err != nil {
		return nil, "", fmt.Errorf("%w: %v", ErrInvalidSource, err)
	}
	nodes, err := normalizedSourceNodes(values, sourceID)
	if err != nil {
		return nil, "", err
	}
	return nodes, detected, nil
}

func detectSourceFormat(raw []byte) SourceFormat {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) != 0 && (trimmed[0] == '{' || trimmed[0] == '[') {
		return SourceFormatSingBoxJSON
	}
	if bytes.Contains(trimmed, []byte("proxies:")) {
		return SourceFormatMihomoYAML
	}
	return SourceFormatURIList
}
