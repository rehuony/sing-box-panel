// SPDX-License-Identifier: GPL-3.0-or-later

package singbox11115

import (
	"encoding/json"
	"fmt"

	"github.com/rehuony/sing-box-panel/internal/canonical"
	"github.com/rehuony/sing-box-panel/internal/configuration/adapter"
)

const Version = "1.11.15"

var officialFeatures = []string{
	"with_acme", "with_clash_api", "with_dhcp", "with_ech", "with_gvisor",
	"with_quic", "with_reality_server", "with_utls", "with_wireguard",
}

type compiledAdapter struct{}

func New() adapter.Adapter { return compiledAdapter{} }

func (compiledAdapter) ID() string           { return "sing-box/v1_11_15/official-linux-plain" }
func (compiledAdapter) Revision() string     { return "2" }
func (compiledAdapter) ExactVersion() string { return Version }
func (compiledAdapter) Provenance() adapter.Provenance {
	return adapter.Provenance{
		UpstreamTag: "v1.11.15", UpstreamCommit: "bc35aca01704497c179da1a03e45ad8e32f1a51b",
		Source: "github.com/SagerNet/sing-box/option at the exact upstream tag",
	}
}
func (compiledAdapter) Supports(profile adapter.Profile) bool {
	return adapter.MatchesOfficialLinuxPlain(profile, Version, officialFeatures)
}

func (compiledAdapter) Project(request adapter.Request) (adapter.Result, error) {
	document, err := canonical.ParseV2(request.CanonicalJSON)
	if err != nil {
		return adapter.Result{}, fmt.Errorf("%w: %v", adapter.ErrProjection, err)
	}
	configuration := document.Configuration()
	diagnostics := make([]adapter.Diagnostic, 0, 2)
	for _, field := range []string{"certificate", "services"} {
		if _, present := configuration[field]; !present {
			continue
		}
		delete(configuration, field)
		diagnostics = append(diagnostics, adapter.Diagnostic{
			Class: adapter.DiagnosticIgnored, Code: "unsupported_top_level_field",
			Path:    "/configuration/" + field,
			Message: "the field is retained globally but is unavailable in sing-box 1.11.15",
		})
	}
	configuration, err = canonical.StripPanelMetadata(configuration)
	if err != nil {
		return adapter.Result{}, fmt.Errorf("%w: %v", adapter.ErrProjection, err)
	}
	configJSON, err := json.Marshal(configuration)
	if err != nil {
		return adapter.Result{}, fmt.Errorf("%w: encode projected configuration: %v", adapter.ErrProjection, err)
	}
	return adapter.FinalizeResult(configJSON, diagnostics)
}
