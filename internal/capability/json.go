// SPDX-License-Identifier: GPL-3.0-or-later

package capability

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/rehuony/sing-box-panel/internal/coreartifact"
)

const (
	MaximumManifestBytes = 1 << 20
	maximumJSONDepth     = 32
	maximumJSONValues    = 100_000
	maximumReferenceSize = 16 << 10
)

var forbiddenManifestKeys = map[string]struct{}{
	"$ref":       {},
	"code":       {},
	"executable": {},
	"javascript": {},
	"module":     {},
	"plugin":     {},
	"script":     {},
	"template":   {},
	"wasm":       {},
}

func DecodeReference(data []byte) (Reference, error) {
	if err := inspectJSON(data, maximumReferenceSize, 8, nil); err != nil {
		return Reference{}, fmt.Errorf("%w: %v", ErrInvalidReference, err)
	}
	var wire referenceWire
	if err := strictDecode(data, &wire); err != nil {
		return Reference{}, fmt.Errorf("%w: decode JSON: %v", ErrInvalidReference, err)
	}
	return NewReference(wire.Repository, wire.Commit, wire.Digest)
}

func DecodeManifest(data []byte) (*Manifest, error) {
	if err := inspectJSON(data, MaximumManifestBytes, maximumJSONDepth, forbiddenManifestKeys); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidManifest, err)
	}
	var spec ManifestSpec
	if err := strictDecode(data, &spec); err != nil {
		return nil, fmt.Errorf("%w: decode JSON: %v", ErrInvalidManifest, err)
	}
	return NewManifest(spec)
}

// CanonicalJSON returns the deterministic byte representation covered by a
// manifest digest. Struct field order and array order are preserved; Go's JSON
// encoder orders map keys, including enum maps, lexicographically.
func (manifest *Manifest) CanonicalJSON() ([]byte, error) {
	if err := manifest.Validate(); err != nil {
		return nil, err
	}
	canonical := cloneManifestSpec(manifest.spec)
	for index := range canonical.Transforms {
		if canonical.Transforms[index].When != nil {
			canonical.Transforms[index].When.Equals = normalizedScalar(canonical.Transforms[index].When.Equals)
		}
	}
	for index := range canonical.UI {
		if canonical.UI[index].VisibleWhen != nil {
			canonical.UI[index].VisibleWhen.Equals = normalizedScalar(canonical.UI[index].VisibleWhen.Equals)
		}
	}
	return json.Marshal(canonical)
}

func (manifest *Manifest) Digest() (coreartifact.SHA256, error) {
	canonical, err := manifest.CanonicalJSON()
	if err != nil {
		return coreartifact.SHA256{}, err
	}
	return coreartifact.NewSHA256(sha256.Sum256(canonical)), nil
}

func inspectJSON(data []byte, maximumBytes, maximumDepth int, forbidden map[string]struct{}) error {
	if len(data) == 0 {
		return fmt.Errorf("empty JSON document")
	}
	if len(data) > maximumBytes {
		return fmt.Errorf("JSON document exceeds %d bytes", maximumBytes)
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	values := 0
	if err := inspectJSONValue(decoder, 0, maximumDepth, &values, forbidden); err != nil {
		return err
	}
	if _, err := decoder.Token(); err != io.EOF {
		if err == nil {
			return fmt.Errorf("JSON document contains more than one value")
		}
		return fmt.Errorf("read trailing JSON: %w", err)
	}
	return nil
}

func inspectJSONValue(
	decoder *json.Decoder,
	depth, maximumDepth int,
	values *int,
	forbidden map[string]struct{},
) error {
	if depth > maximumDepth {
		return fmt.Errorf("JSON nesting exceeds depth %d", maximumDepth)
	}
	*values++
	if *values > maximumJSONValues {
		return fmt.Errorf("JSON document exceeds %d values", maximumJSONValues)
	}
	token, err := decoder.Token()
	if err != nil {
		return fmt.Errorf("decode JSON token: %w", err)
	}
	delimiter, composite := token.(json.Delim)
	if !composite {
		return nil
	}

	switch delimiter {
	case '{':
		seen := make(map[string]struct{})
		for decoder.More() {
			nameToken, err := decoder.Token()
			if err != nil {
				return fmt.Errorf("decode JSON object name: %w", err)
			}
			name, ok := nameToken.(string)
			if !ok {
				return fmt.Errorf("JSON object name is not a string")
			}
			if _, exists := seen[name]; exists {
				return fmt.Errorf("duplicate JSON object key %q", name)
			}
			seen[name] = struct{}{}
			if _, blocked := forbidden[strings.ToLower(name)]; blocked {
				return fmt.Errorf("manifest key %q is forbidden", name)
			}
			if err := inspectJSONValue(decoder, depth+1, maximumDepth, values, forbidden); err != nil {
				return err
			}
		}
		closing, err := decoder.Token()
		if err != nil || closing != json.Delim('}') {
			return fmt.Errorf("invalid JSON object close")
		}
	case '[':
		for decoder.More() {
			if err := inspectJSONValue(decoder, depth+1, maximumDepth, values, forbidden); err != nil {
				return err
			}
		}
		closing, err := decoder.Token()
		if err != nil || closing != json.Delim(']') {
			return fmt.Errorf("invalid JSON array close")
		}
	default:
		return fmt.Errorf("unexpected JSON delimiter %q", delimiter)
	}
	return nil
}

func strictDecode(data []byte, destination any) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	decoder.UseNumber()
	if err := decoder.Decode(destination); err != nil {
		return err
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		if err == nil {
			return fmt.Errorf("JSON document contains more than one value")
		}
		return err
	}
	return nil
}
