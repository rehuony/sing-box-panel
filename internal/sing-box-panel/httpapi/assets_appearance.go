// SPDX-License-Identifier: GPL-3.0-or-later

package httpapi

import (
	"fmt"
	"math"
	"strconv"

	"github.com/rehuony/sing-box-panel/internal/sing-box-panel/settings"
)

// initialAppearanceCSS paints the saved canvas before any JavaScript loads.
// Keep the blend aligned with web/apps/panel/src/theme/appearance.ts. The selector stops
// applying as soon as ThemeProvider takes ownership of the document.
func initialAppearanceCSS(appearance settings.Appearance) string {
	if (settings.Panel{Language: "en", Appearance: appearance}).Validate() != nil {
		appearance = settings.DefaultPanel().Appearance
	}
	color, _ := strconv.ParseUint(appearance.Color[1:], 16, 32)
	base := [3]uint64{color >> 16, color >> 8 & 255, color & 255}
	palette := func(dark bool) string {
		surface, scheme, canvas := [3]uint64{255, 255, 255}, "light", 0.96
		if dark {
			surface, scheme, canvas = [3]uint64{24, 23, 30}, "dark", 0.98
		}
		var channels [3]int
		for i := range channels {
			channels[i] = int(math.Round(float64(base[i])*(1-canvas) + float64(surface[i])*canvas))
		}
		return fmt.Sprintf(":root:not([data-resolved-theme]){color-scheme:%s;--color-canvas:#%02x%02x%02x;}",
			scheme, channels[0], channels[1], channels[2])
	}
	if appearance.Theme == "system" {
		return palette(false) + "@media(prefers-color-scheme:dark){" + palette(true) + "}"
	}
	return palette(appearance.Theme == "dark")
}
