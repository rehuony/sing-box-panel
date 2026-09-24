// SPDX-License-Identifier: GPL-3.0-or-later

package settings

import (
	"errors"
	"net/netip"
	"regexp"
	"strings"
)

// Panel contains preferences shared by the file, Web UI, and CLI.
type Panel struct {
	PublicNodeHost string     `json:"public_node_host"`
	Language       string     `json:"language"`
	Appearance     Appearance `json:"appearance"`
}

type Appearance struct {
	Theme  string `json:"theme"`
	Color  string `json:"color"`
	Radius int    `json:"radius"`
}

func DefaultPanel() Panel {
	return Panel{Language: "zh-CN", Appearance: Appearance{Theme: "light", Color: "#6D4ED1", Radius: 12}}
}

var panelColor = regexp.MustCompile(`^#[a-fA-F0-9]{6}$`)
var hostnameLabel = regexp.MustCompile(`^[a-zA-Z0-9](?:[a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?$`)

func (value Panel) Validate() error {
	if !panelColor.MatchString(value.Appearance.Color) || value.Appearance.Radius < 0 || value.Appearance.Radius > 32 ||
		(value.Appearance.Theme != "light" && value.Appearance.Theme != "dark" && value.Appearance.Theme != "system") ||
		(value.Language != "zh-CN" && value.Language != "en") {
		return errors.New("panel preferences are invalid")
	}
	if value.PublicNodeHost != "" && !ValidPublishedHost(value.PublicNodeHost) {
		return errors.New("panel.public_node_host must be a public IP or domain")
	}
	return nil
}

func ValidPublishedHost(host string) bool {
	if address, err := netip.ParseAddr(strings.Trim(host, "[]")); err == nil {
		return address.Zone() == "" && address.IsGlobalUnicast() && !address.IsLoopback() && !address.IsPrivate()
	}
	if len(host) > 253 || strings.EqualFold(host, "localhost") {
		return false
	}
	for _, label := range strings.Split(strings.TrimSuffix(host, "."), ".") {
		if !hostnameLabel.MatchString(label) {
			return false
		}
	}
	return strings.Contains(host, ".")
}
