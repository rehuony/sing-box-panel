// SPDX-License-Identifier: GPL-3.0-or-later

package main

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/rehuony/sing-box-panel/internal/singbox"
)

const (
	supportGuidePath  = "docs/guides/core-versions.md"
	supportTableStart = "<!-- singbox-support-table:start -->"
	supportTableEnd   = "<!-- singbox-support-table:end -->"
)

func syncSupportGuide(root string, catalog sourceCatalog, checkOnly bool) error {
	filename := filepath.Join(root, supportGuidePath)
	current, err := os.ReadFile(filename)
	if err != nil {
		return fmt.Errorf("read support guide: %w", err)
	}
	generated, err := renderSupportGuide(string(current), catalog)
	if err != nil {
		return err
	}
	if checkOnly {
		if !bytes.Equal(current, generated) {
			return errors.New("support guide table is stale; run `go tool singbox-support generate`")
		}
		return nil
	}
	return writeFileIfChanged(filename, generated)
}

func renderSupportGuide(current string, catalog sourceCatalog) ([]byte, error) {
	if strings.Count(current, supportTableStart) != 1 || strings.Count(current, supportTableEnd) != 1 {
		return nil, errors.New("support guide requires exactly one support table marker pair")
	}
	prefix, remainder, _ := strings.Cut(current, supportTableStart)
	_, suffix, found := strings.Cut(remainder, supportTableEnd)
	if !found {
		return nil, errors.New("support guide table markers are out of order")
	}
	type group struct {
		source, family string
		versions       []string
	}
	var groups []group
	for _, version := range catalog.Versions {
		index := len(groups)
		for candidate, value := range groups {
			if value.source == version.SchemaSource && value.family == version.InboundFamily {
				index = candidate
				break
			}
		}
		if index == len(groups) {
			groups = append(groups, group{source: version.SchemaSource, family: version.InboundFamily})
		}
		groups[index].versions = append(groups[index].versions, version.ExactVersion)
	}
	var table strings.Builder
	table.WriteString("\n<!-- Generated from internal/singbox/catalog.json; DO NOT EDIT. -->\n\n")
	table.WriteString("| Exact releases | Configuration Schema source | Inbound family |\n| --- | --- | --- |\n")
	for _, value := range groups {
		source := "Unavailable"
		switch value.source {
		case singbox.SchemaSourceNative:
			source = "Official binary's native `schema` command"
		case singbox.SchemaSourceReviewed113:
			source = "Project-reviewed `reviewed-1.13` source definition"
		}
		family := value.family
		if family == "" {
			family = "Unavailable"
		}
		fmt.Fprintf(&table, "| %s | %s | %s |\n", strings.Join(value.versions, ", "), source, family)
	}
	return []byte(prefix + supportTableStart + table.String() + supportTableEnd + suffix), nil
}
