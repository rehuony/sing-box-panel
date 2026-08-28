// SPDX-License-Identifier: GPL-3.0-or-later

package singbox11319

import (
	"encoding/json"
	"fmt"

	"github.com/rehuony/sing-box-panel/internal/canonical"
	"github.com/rehuony/sing-box-panel/internal/configuration/adapter"
)

const Version = "1.13.19"

var officialFeatures = []string{
	"badlinkname", "tfogo_checklinkname0", "with_acme", "with_ccm", "with_clash_api",
	"with_dhcp", "with_gvisor", "with_ocm", "with_quic", "with_tailscale",
	"with_utls", "with_wireguard",
}

type compiledAdapter struct{}

func New() adapter.Adapter { return compiledAdapter{} }

func (compiledAdapter) ID() string           { return "sing-box/v1_13_19/official-linux-arm64" }
func (compiledAdapter) Revision() string     { return "1" }
func (compiledAdapter) ExactVersion() string { return Version }
func (compiledAdapter) Provenance() adapter.Provenance {
	return adapter.Provenance{
		UpstreamTag: "v1.13.19", UpstreamCommit: "b5ebaa1fc0f2b94256180b95468e73ef53caa27d",
		Source: "github.com/SagerNet/sing-box/option at the exact upstream tag",
	}
}
func (compiledAdapter) Supports(profile adapter.Profile) bool {
	return adapter.MatchesOfficialLinuxARM64(profile, Version, officialFeatures)
}

func (compiledAdapter) Project(request adapter.Request) (adapter.Result, error) {
	document, err := canonical.ParseV2(request.CanonicalJSON)
	if err != nil {
		return adapter.Result{}, fmt.Errorf("%w: %v", adapter.ErrProjection, err)
	}
	configuration, err := canonical.StripPanelMetadata(document.Configuration())
	if err != nil {
		return adapter.Result{}, fmt.Errorf("%w: %v", adapter.ErrProjection, err)
	}
	configJSON, err := json.Marshal(configuration)
	if err != nil {
		return adapter.Result{}, fmt.Errorf("%w: encode projected configuration: %v", adapter.ErrProjection, err)
	}
	return adapter.FinalizeResult(configJSON, nil)
}
