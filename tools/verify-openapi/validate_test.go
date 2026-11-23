// SPDX-License-Identifier: GPL-3.0-or-later

package main

import (
	"fmt"
	"strings"
	"testing"
)

const validDocument = `openapi: 3.1.0
paths:
  /ok:
    get:
      operationId: getOk
      responses:
        "200":
          description: ok
          content:
            application/json:
              schema:
                $ref: "#/components/schemas/Result~1Value"
components:
  schemas:
    Result/Value:
      type: object
`

func TestValidateAcceptsRepositoryContract(t *testing.T) {
	t.Parallel()
	if findings := validateOpenAPI([]byte(validDocument)); len(findings) != 0 {
		t.Fatalf("validateOpenAPI(): %s", formatFindings(findings))
	}
}

func TestValidateRejectsInvalidContracts(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		source  string
		message string
	}{
		{
			name:    "duplicate key",
			source:  strings.Replace(validDocument, "paths:\n", "paths:\npaths:\n", 1),
			message: "duplicate key",
		},
		{
			name:    "missing operation ID",
			source:  strings.Replace(validDocument, "operationId: getOk", "description: missing id", 1),
			message: "missing non-empty operationId",
		},
		{
			name: "generic successful response",
			source: strings.Replace(validDocument,
				`$ref: "#/components/schemas/Result~1Value"`,
				`type: object
                additionalProperties: true`, 1),
			message: "must not use a generic object schema",
		},
		{
			name:    "unresolved reference",
			source:  strings.Replace(validDocument, "#/components/schemas/Result~1Value", "#/components/schemas/Missing", 1),
			message: "unresolved reference",
		},
		{
			name: "duplicate operation ID",
			source: strings.Replace(validDocument, "components:\n",
				"  /other:\n    post:\n      operationId: getOk\n      responses: {}\ncomponents:\n", 1),
			message: "duplicate operationId",
		},
		{
			name:    "external reference",
			source:  strings.Replace(validDocument, "#/components/schemas/Result~1Value", "other.yaml#/Result", 1),
			message: "cannot be validated offline",
		},
		{
			name:    "alias",
			source:  "openapi: 3.1.0\npaths: &paths {}\ncomponents:\n  schemas: *paths\n",
			message: "YAML aliases are not allowed",
		},
		{
			name:    "multiple documents",
			source:  "openapi: 3.1.0\npaths: {}\n---\nopenapi: 3.1.0\npaths: {}\n",
			message: "exactly one YAML document",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			findings := validateOpenAPI([]byte(test.source))
			if !containsFinding(findings, test.message) {
				t.Fatalf("validateOpenAPI() findings = %s, want a finding containing %q", formatFindings(findings), test.message)
			}
		})
	}
}

func TestValidateIsDeterministic(t *testing.T) {
	t.Parallel()
	source := []byte("openapi: 3.1.0\npaths:\n  /z:\n    get: {}\n  /a:\n    post: {}\n")
	first := formatFindings(validateOpenAPI(source))
	second := formatFindings(validateOpenAPI(source))
	if first != second {
		t.Fatalf("validateOpenAPI() changed between identical calls:\nfirst:  %s\nsecond: %s", first, second)
	}
}

func containsFinding(findings []error, fragment string) bool {
	for _, finding := range findings {
		if strings.Contains(finding.Error(), fragment) {
			return true
		}
	}
	return false
}

func formatFindings(findings []error) string {
	if len(findings) == 0 {
		return "<none>"
	}
	formatted := make([]string, len(findings))
	for index, finding := range findings {
		formatted[index] = finding.Error()
	}
	return fmt.Sprintf("%q", formatted)
}
