// SPDX-License-Identifier: GPL-3.0-or-later
package application

import (
	"encoding/json"
	"errors"
	"fmt"
	"testing"
)

func TestPrivateRuntimeErrorsPreserveClassification(t *testing.T) {
	for _, kind := range runtimeErrorKinds {
		t.Run(kind.code, func(t *testing.T) {
			original := fmt.Errorf("operation detail: %w", kind.cause)
			encoded, err := json.Marshal(NewRuntimeControlError(original))
			if err != nil {
				t.Fatal(err)
			}
			var decoded RuntimeControlError
			if err := json.Unmarshal(encoded, &decoded); err != nil {
				t.Fatal(err)
			}
			if !errors.Is(&decoded, kind.cause) || decoded.Error() != original.Error() {
				t.Fatalf("lost classification: %s", encoded)
			}
		})
	}
}
