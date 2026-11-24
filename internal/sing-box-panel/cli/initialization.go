// SPDX-License-Identifier: GPL-3.0-or-later

package cli

import (
	"io"
	"net"
	"strconv"

	"github.com/rehuony/sing-box-panel/internal/cliui"
	"github.com/rehuony/sing-box-panel/internal/sing-box-panel/settings"
)

func initialPanelURL(value settings.Settings) string {
	return "http://" + net.JoinHostPort(value.Server.Host, strconv.Itoa(value.Server.Port)) + value.Server.BasePath + "/"
}

func initializationText(writer io.Writer, format cliui.Format, value settings.Settings) string {
	style := cliui.NewStyle(writer, format)
	return cliui.Notice(cliui.Section(style.Success("sing-box-panel settings created"), cliui.Fields(
		cliui.Field{Label: "Panel URL", Value: style.Accent(initialPanelURL(value))},
		cliui.Field{Label: "Email", Value: value.Auth.Email},
		cliui.Field{Label: "Initial password", Value: value.InitialPassword},
		cliui.Field{Label: "Settings", Value: style.Path(value.Path())},
		cliui.Field{Label: "Data directory", Value: style.Path(value.DataDir)},
	), "Save this password now; only its hash is stored."))
}
