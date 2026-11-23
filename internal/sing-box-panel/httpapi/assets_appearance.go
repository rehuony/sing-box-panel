// SPDX-License-Identifier: GPL-3.0-or-later

package httpapi

import (
	"fmt"

	"github.com/rehuony/sing-box-panel/internal/sing-box-panel/settings"
)

// initialAppearanceCSS paints the saved theme's canvas before any JavaScript
// loads. The canvas is the same under every accent; keep its values aligned
// with web/packages/shared/src/styles/tokens.css. The selector stops applying
// as soon as ThemeProvider takes ownership of the document.
func initialAppearanceCSS(appearance settings.Appearance) string {
	if (settings.Panel{Language: "en", Appearance: appearance}).Validate() != nil {
		appearance = settings.DefaultPanel().Appearance
	}
	palette := func(dark bool) string {
		scheme, canvas := "light", "#eeeef2"
		if dark {
			scheme, canvas = "dark", "#1c1d24"
		}
		return fmt.Sprintf(":root:not([data-resolved-theme]){color-scheme:%s;--color-canvas:%s;}", scheme, canvas)
	}
	if appearance.Theme == "system" {
		return palette(false) + "@media(prefers-color-scheme:dark){" + palette(true) + "}"
	}
	return palette(appearance.Theme == "dark")
}
