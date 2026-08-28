// SPDX-License-Identifier: GPL-3.0-or-later

package render_test

import (
	"fmt"

	"github.com/rehuony/sing-box-panel/internal/subscription/render"
)

func ExampleRender() {
	result, err := render.Render(
		[]byte(`{"outbounds":[{"type":"shadowsocks","tag":"edge","server":"proxy.example","server_port":443,"method":"aes-128-gcm","password":"secret"}]}`),
		render.Channel{Format: render.FormatMihomo},
	)
	if err != nil {
		panic(err)
	}
	fmt.Printf("%s %d\n", result.Format, result.NodeCount)
	// Output:
	// mihomo 1
}
