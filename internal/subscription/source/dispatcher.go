// SPDX-License-Identifier: GPL-3.0-or-later

// Package source parses and normalizes complete third-party subscription
// candidates without performing network I/O.
package source

import (
	"bytes"
	"errors"
	"fmt"
	"unicode/utf8"

	"github.com/rehuony/sing-box-panel/internal/subscription/document"
	"github.com/rehuony/sing-box-panel/internal/subscription/node"
)

type Format string

const (
	FormatAuto        Format = "auto"
	FormatSingBoxJSON Format = "sing-box-json"
	FormatMihomoYAML  Format = "mihomo-yaml"
	FormatURIList     Format = "uri-list"
)

var ErrInvalidSource = errors.New("invalid subscription source")

// Parse normalizes one complete third-party candidate. Any syntax,
// identity, or required-field error rejects the whole version.
func Parse(format Format, raw []byte, sourceID string) ([]node.Node, Format, error) {
	if len(raw) == 0 || len(raw) > document.MaximumBytes || !utf8.Valid(raw) || !validNodeSourceID(sourceID) {
		return nil, "", ErrInvalidSource
	}
	detected := format
	if format == FormatAuto {
		detected = detectSourceFormat(raw)
	}
	var values []map[string]any
	var err error
	switch detected {
	case FormatSingBoxJSON:
		values, err = parseSingBoxSource(raw)
	case FormatMihomoYAML:
		values, err = parseMihomoSource(raw)
	case FormatURIList:
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

func detectSourceFormat(raw []byte) Format {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) != 0 && (trimmed[0] == '{' || trimmed[0] == '[') {
		return FormatSingBoxJSON
	}
	if bytes.Contains(trimmed, []byte("proxies:")) {
		return FormatMihomoYAML
	}
	return FormatURIList
}
