// SPDX-License-Identifier: GPL-3.0-or-later

package subscription_test

import (
	"fmt"

	"github.com/rehuony/sing-box-panel/internal/subscription"
)

func ExampleRender() {
	result, err := subscription.Render(
		[]byte(`{"outbounds":[{"type":"shadowsocks","tag":"edge","server":"proxy.example","server_port":443,"method":"aes-128-gcm","password":"secret"}]}`),
		subscription.RenderChannel{Format: subscription.RenderFormatMihomo},
	)
	if err != nil {
		panic(err)
	}
	fmt.Printf("%s %d\n", result.Format, result.NodeCount)
	// Output:
	// mihomo 1
}
