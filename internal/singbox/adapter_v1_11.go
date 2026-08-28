// SPDX-License-Identifier: GPL-3.0-or-later

// The 1.11 projector owns the reviewed sing-box 1.11 configuration behavior.
package singbox

import (
	"encoding/json"
	"fmt"

	"github.com/rehuony/sing-box-panel/internal/configuration"
)

func projectV111(exactVersion string, request configuration.ProjectionRequest) (configuration.ProjectionResult, error) {
	document, err := configuration.ParseV2(request.CanonicalJSON)
	if err != nil {
		return configuration.ProjectionResult{}, fmt.Errorf("%w: %v", configuration.ErrProjection, err)
	}
	projected := document.Configuration()
	diagnostics := make([]configuration.ProjectionDiagnostic, 0, 2)
	for _, field := range []string{"certificate", "services"} {
		if _, present := projected[field]; !present {
			continue
		}
		delete(projected, field)
		diagnostics = append(diagnostics, configuration.ProjectionDiagnostic{
			Class: configuration.DiagnosticIgnored, Code: "unsupported_top_level_field",
			Path:    "/configuration/" + field,
			Message: "the field is retained globally but is unavailable in sing-box " + exactVersion,
		})
	}
	projected, err = configuration.StripPanelMetadata(projected)
	if err != nil {
		return configuration.ProjectionResult{}, fmt.Errorf("%w: %v", configuration.ErrProjection, err)
	}
	configJSON, err := json.Marshal(projected)
	if err != nil {
		return configuration.ProjectionResult{}, fmt.Errorf("%w: encode projected configuration: %v", configuration.ErrProjection, err)
	}
	return configuration.FinalizeProjection(configJSON, diagnostics)
}
