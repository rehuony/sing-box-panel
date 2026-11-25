// SPDX-License-Identifier: GPL-3.0-or-later

package subscription

import (
	"errors"
	"testing"
)

func TestInboundConversionNeedsAReleaseLine(t *testing.T) {
	t.Parallel()
	for _, version := range []string{"1.13.18", "1.14", "v1.14.4", ""} {
		if _, err := convertInbounds(version, InboundRequest{FinalStartupJSON: []byte(`{}`), PublicHost: "public.example"}); !errors.Is(err, ErrUnsupportedCoreVersion) {
			t.Fatalf("convertInbounds(%q) error = %v, want ErrUnsupportedCoreVersion", version, err)
		}
	}
}
