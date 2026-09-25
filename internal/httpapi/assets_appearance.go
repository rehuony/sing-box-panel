// SPDX-License-Identifier: GPL-3.0-or-later

package httpapi

import (
	"fmt"
	"math"
	"strconv"

	"github.com/rehuony/sing-box-panel/internal/settings"
)

// initialAppearanceCSS paints the saved background before any JavaScript loads.
// Keep these blends aligned with web/src/theme/appearance.ts. The selector stops
// applying as soon as ThemeProvider takes ownership of the document.
func initialAppearanceCSS(appearance settings.Appearance) string {
	if (settings.Panel{Language: "en", Appearance: appearance}).Validate() != nil {
		appearance = settings.DefaultPanel().Appearance
	}
	color, _ := strconv.ParseUint(appearance.Color[1:], 16, 32)
	base := [3]uint64{color >> 16, color >> 8 & 255, color & 255}
	palette := func(dark bool) string {
		surface, scheme, soft := [3]uint64{255, 255, 255}, "light", 0.91
		canvas, canvasDeep := "var(--color-paper-2)", "var(--color-paper-3)"
		if dark {
			surface, scheme, soft = [3]uint64{24, 23, 30}, "dark", 0.84
			canvas, canvasDeep = "var(--color-paper)", "oklch(13% 0.014 274)"
		}
		blend := func(weight float64) string {
			var channels [3]int
			for i := range channels {
				channels[i] = int(math.Round(float64(base[i])*(1-weight) + float64(surface[i])*weight))
			}
			return fmt.Sprintf("#%02x%02x%02x", channels[0], channels[1], channels[2])
		}
		// Canvas aliases also mirror the light/dark rules in styles/global.css.
		return fmt.Sprintf(":root:not([data-resolved-theme]){color-scheme:%s;--color-paper:%s;--color-paper-2:%s;--color-paper-3:%s;--color-accent-soft:%s;--color-canvas:%s;--color-canvas-deep:%s;}",
			scheme, blend(0.98), blend(0.96), blend(0.92), blend(soft), canvas, canvasDeep)
	}
	if appearance.Theme == "system" {
		return palette(false) + "@media(prefers-color-scheme:dark){" + palette(true) + "}"
	}
	return palette(appearance.Theme == "dark")
}
