// SPDX-License-Identifier: GPL-3.0-or-later

// The 1.12 projector owns the reviewed sing-box 1.12 configuration behavior.
package singbox

import (
	"encoding/json"
	"fmt"

	"github.com/rehuony/sing-box-panel/internal/configuration"
)

func projectV112(_ string, request configuration.ProjectionRequest) (configuration.ProjectionResult, error) {
	document, err := configuration.Parse(request.CanonicalJSON)
	if err != nil {
		return configuration.ProjectionResult{}, fmt.Errorf("%w: %v", configuration.ErrProjection, err)
	}
	projected, err := configuration.StripPanelMetadata(document.Configuration())
	if err != nil {
		return configuration.ProjectionResult{}, fmt.Errorf("%w: %v", configuration.ErrProjection, err)
	}
	configJSON, err := json.Marshal(projected)
	if err != nil {
		return configuration.ProjectionResult{}, fmt.Errorf("%w: encode projected configuration: %v", configuration.ErrProjection, err)
	}
	return configuration.FinalizeProjection(configJSON, nil)
}
