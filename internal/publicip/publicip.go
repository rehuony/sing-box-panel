// SPDX-License-Identifier: GPL-3.0-or-later

// Package publicip resolves a bounded, cached public endpoint hint. The hint is
// never used to change a listener or an explicit operator override.
package publicip

import (
	"context"
	"io"
	"net/http"
	"net/netip"
	"strings"
	"sync"
	"time"
)

type Detector struct {
	mu      sync.Mutex
	client  *http.Client
	now     func() time.Time
	value   string
	expires time.Time
}

func New() *Detector {
	return &Detector{client: &http.Client{
		Timeout:       3 * time.Second,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}, now: time.Now}
}

// Resolve coalesces simultaneous requests and negatively caches failures. No
// credentials, panel URL, node information or user-supplied URL are sent.
func (d *Detector) Resolve(ctx context.Context) string {
	d.mu.Lock()
	defer d.mu.Unlock()
	if ctx.Err() != nil {
		return ""
	}
	if d.now().Before(d.expires) {
		return d.value
	}
	d.value = ""
	d.expires = d.now().Add(30 * time.Second)
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://api.ipify.org", nil)
	if err != nil {
		return ""
	}
	request.Header.Set("Accept", "text/plain")
	response, err := d.client.Do(request)
	if err != nil {
		return ""
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return ""
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, 129))
	if err != nil || len(body) > 128 {
		return ""
	}
	address, err := netip.ParseAddr(strings.TrimSpace(string(body)))
	if err != nil {
		return ""
	}
	address = address.Unmap()
	if !address.IsGlobalUnicast() || address.IsPrivate() || address.IsLoopback() || address.IsLinkLocalUnicast() {
		return ""
	}
	for _, reserved := range []string{"100.64.0.0/10", "192.0.0.0/24", "192.0.2.0/24", "198.18.0.0/15", "198.51.100.0/24", "203.0.113.0/24", "240.0.0.0/4", "2001:db8::/32"} {
		if netip.MustParsePrefix(reserved).Contains(address) {
			return ""
		}
	}
	d.value = address.String()
	d.expires = d.now().Add(5 * time.Minute)
	return d.value
}
