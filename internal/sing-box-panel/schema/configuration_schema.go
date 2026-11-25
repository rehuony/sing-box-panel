// SPDX-License-Identifier: GPL-3.0-or-later

// Package schema prepares the sing-box configuration JSON Schemas that the
// panel serves to its editor and checks configurations with: the reviewed
// 1.13 schema embedded here, and the schema a node's core prints for the
// lines that have `sing-box schema`.
package schema

import (
	"bytes"
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"

	"github.com/rehuony/sing-box-panel/internal/nodeapi"
	jsonschema "github.com/santhosh-tekuri/jsonschema/v6"
)

// ErrConfigurationSchemaUnavailable reports a core without a configuration
// schema.
var ErrConfigurationSchemaUnavailable = errors.New("configuration schema is unavailable")

//go:embed 1.13.json
var reviewed113Source []byte

var reviewed113 = sync.OnceValues(func() (*Contract, error) { return Prepare(reviewed113Source) })

// Reviewed113 returns the contract of the reviewed schema of the 1.13 line.
func Reviewed113() (*Contract, error) { return reviewed113() }

// Contract is a prepared configuration schema. It is immutable.
type Contract struct {
	sha256    string
	schema    []byte
	validator *jsonschema.Schema
}

// SHA256 is the lowercase hexadecimal SHA-256 of the schema, its identity
// for API and Web consumers.
func (contract *Contract) SHA256() string { return contract.sha256 }

// Schema returns a copy of the schema: canonical JSON with the panel's
// presentation overlay.
func (contract *Contract) Schema() json.RawMessage { return bytes.Clone(contract.schema) }

// Validate checks a configuration document against the schema. The core's
// own `sing-box check` remains authoritative.
func (contract *Contract) Validate(document []byte) error {
	var decoded any
	if err := decodeJSONPreservingNumbers(document, &decoded); err != nil {
		return fmt.Errorf("decode sing-box configuration: %w", err)
	}
	if err := contract.validator.Validate(decoded); err != nil {
		return fmt.Errorf("configuration violates the core's schema: %w", err)
	}
	return nil
}

// Prepare turns a raw schema, as a core prints it or as the panel embeds
// it, into a contract: at most nodeapi.MaximumCoreSchemaBytes of one JSON
// object whose references stay inside it, written as canonical JSON with
// the presentation overlay and compiled as Draft 2020-12.
func Prepare(raw []byte) (*Contract, error) {
	if len(raw) > nodeapi.MaximumCoreSchemaBytes {
		return nil, fmt.Errorf("schema exceeds %d bytes", nodeapi.MaximumCoreSchemaBytes)
	}
	var document any
	if err := decodeJSONPreservingNumbers(raw, &document); err != nil {
		return nil, fmt.Errorf("decode schema: %w", err)
	}
	root, ok := document.(map[string]any)
	if !ok {
		return nil, errors.New("schema root is not an object")
	}
	if err := rejectExternalSchemaReferences(root); err != nil {
		return nil, err
	}
	if err := applyPresentationOverlay(root); err != nil {
		return nil, fmt.Errorf("apply presentation overlay: %w", err)
	}
	schema, err := json.Marshal(root)
	if err != nil {
		return nil, err
	}
	digest := sha256.Sum256(schema)
	identity := hex.EncodeToString(digest[:])
	resource := "urn:sing-box-panel:configuration-schema:" + identity
	compiler := jsonschema.NewCompiler()
	compiler.DefaultDraft(jsonschema.Draft2020)
	if err := compiler.AddResource(resource, root); err != nil {
		return nil, fmt.Errorf("add schema: %w", err)
	}
	validator, err := compiler.Compile(resource)
	if err != nil {
		return nil, fmt.Errorf("compile schema: %w", err)
	}
	return &Contract{sha256: identity, schema: schema, validator: validator}, nil
}

func rejectExternalSchemaReferences(value any) error {
	switch typed := value.(type) {
	case []any:
		for _, child := range typed {
			if err := rejectExternalSchemaReferences(child); err != nil {
				return err
			}
		}
	case map[string]any:
		for key, child := range typed {
			if key == "$ref" {
				reference, ok := child.(string)
				if !ok || !strings.HasPrefix(reference, "#") {
					return fmt.Errorf("external schema reference %#v is forbidden", child)
				}
			}
			if err := rejectExternalSchemaReferences(child); err != nil {
				return err
			}
		}
	}
	return nil
}

func decodeJSONPreservingNumbers(raw []byte, destination any) error {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	if err := decoder.Decode(destination); err != nil {
		return err
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		if err == nil {
			return errors.New("JSON contains multiple values")
		}
		return err
	}
	return nil
}
