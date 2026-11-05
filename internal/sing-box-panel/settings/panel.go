// SPDX-License-Identifier: GPL-3.0-or-later

package settings

import (
	"errors"
	"regexp"
)

// Panel contains preferences shared by the file, Web UI, and CLI.
type Panel struct {
	Language   string     `json:"language"`
	Appearance Appearance `json:"appearance"`
}

type Appearance struct {
	Theme  string `json:"theme"`
	Color  string `json:"color"`
	Radius int    `json:"radius"`
}

func DefaultPanel() Panel {
	return Panel{Language: "zh-CN", Appearance: Appearance{Theme: "system", Color: "#6D4ED1", Radius: 12}}
}

var panelColor = regexp.MustCompile(`^#[a-fA-F0-9]{6}$`)

func (value Panel) Validate() error {
	if !panelColor.MatchString(value.Appearance.Color) || value.Appearance.Radius < 0 || value.Appearance.Radius > 32 ||
		(value.Appearance.Theme != "light" && value.Appearance.Theme != "dark" && value.Appearance.Theme != "system") ||
		(value.Language != "zh-CN" && value.Language != "en") {
		return errors.New("panel preferences are invalid")
	}
	return nil
}
