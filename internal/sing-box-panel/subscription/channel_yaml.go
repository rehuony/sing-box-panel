// SPDX-License-Identifier: GPL-3.0-or-later

package subscription

import (
	"bytes"
	"fmt"
	"strconv"
	"unicode/utf8"

	"go.yaml.in/yaml/v3"
)

// go-yaml v3 escapes supplementary Unicode, including emoji, even with Unicode
// output enabled. Restore these characters inside double-quoted scalars only;
// comments, block scalars and literal backslash sequences must stay untouched.
// Upstream fix: https://github.com/yaml/go-yaml/pull/395.
func restoreYAMLUnicode(content []byte) ([]byte, error) {
	if !bytes.Contains(content, []byte(`\U`)) {
		return content, nil
	}
	var document yaml.Node
	if err := yaml.Unmarshal(content, &document); err != nil {
		return nil, fmt.Errorf("decode generated YAML: %w", err)
	}
	lines := []int{0}
	for i, b := range content {
		if b == '\n' {
			lines = append(lines, i+1)
		}
	}
	var output bytes.Buffer
	copied := 0
	var visit func(*yaml.Node)
	visit = func(node *yaml.Node) {
		if node.Kind == yaml.ScalarNode && node.Style&yaml.DoubleQuotedStyle != 0 {
			start := lines[node.Line-1]
			// YAML columns count characters, not UTF-8 bytes. Explicit tags, when
			// present, precede the opening quote at this position.
			for column := 1; column < node.Column; column++ {
				_, size := utf8.DecodeRune(content[start:])
				start += size
			}
			start += bytes.IndexByte(content[start:], '"')
			for i := start + 1; content[i] != '"'; i++ {
				if content[i] != '\\' {
					continue
				}
				i++ // Skip escaped quotes/backslashes instead of treating them as syntax.
				if content[i] != 'U' {
					continue
				}
				end := i + 9 // The parser has validated all eight hexadecimal digits.
				value, err := strconv.ParseUint(string(content[i+1:end]), 16, 32)
				if err == nil && value >= 0x10000 && value <= utf8.MaxRune {
					output.Write(content[copied : i-1])
					output.WriteRune(rune(value))
					copied = end
				}
				i = end - 1
			}
		}
		for _, child := range node.Content {
			visit(child)
		}
	}
	visit(&document)
	if copied == 0 {
		return content, nil
	}
	output.Write(content[copied:])
	return output.Bytes(), nil
}
