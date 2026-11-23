// SPDX-License-Identifier: GPL-3.0-or-later

package netguard

import (
	"net/netip"
	"testing"
)

func TestPublic(t *testing.T) {
	for address, want := range map[string]bool{
		"1.1.1.1":            true,
		"2606:4700::1111":    true,
		"::ffff:8.8.8.8":     true,
		"10.0.0.1":           false,
		"100.100.100.200":    false, // shared address space, used for cloud metadata
		"169.254.169.254":    false,
		"198.18.0.1":         false,
		"203.0.113.9":        false,
		"240.0.0.1":          false,
		"0.1.2.3":            false,
		"::ffff:127.0.0.1":   false,
		"64:ff9b::a9fe:a9fe": false, // NAT64 of 169.254.169.254
		"2002:a9fe:a9fe::1":  false, // 6to4 of 169.254.169.254
		"fd00::1":            false,
		"2001:db8::1":        false,
		"4000::1":            false,
	} {
		if got := Public(netip.MustParseAddr(address)); got != want {
			t.Errorf("Public(%s) = %t, want %t", address, got, want)
		}
	}
}
