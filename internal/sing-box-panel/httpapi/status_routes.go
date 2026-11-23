// SPDX-License-Identifier: GPL-3.0-or-later

package httpapi

import (
	"net/http"
	"runtime"
)

func (handler *Handler) systemStatus(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, SystemStatus{Platform: SystemPlatform{OS: runtime.GOOS, Arch: runtime.GOARCH}, PanelVersion: handler.build.Version})
}
