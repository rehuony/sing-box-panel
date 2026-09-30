// SPDX-License-Identifier: GPL-3.0-or-later

package cli

import (
	"fmt"
	"io"
	"net"
	"strconv"

	"github.com/rehuony/sing-box-panel/internal/settings"
)

func initialPanelURL(value settings.Settings) string {
	return "http://" + net.JoinHostPort(value.Server.Host, strconv.Itoa(value.Server.Port)) + value.Server.BasePath + "/"
}

func initializationText(writer io.Writer, format outputFormat, value settings.Settings) string {
	style := newFileTreeStyle(writer, format)
	return fmt.Sprintf("\n%s\n\n  Panel URL         %s\n  Email             %s\n  Initial Password  %s\n  Settings          %s\n  Data Dir          %s\n\n  Save this password now; only its hash is stored.\n",
		style.paint("1;32", "sing-box-panel settings created"),
		style.paint("36", initialPanelURL(value)), value.Auth.Email, value.InitialPassword,
		style.path(value.Path()), style.path(value.DataDir))
}
