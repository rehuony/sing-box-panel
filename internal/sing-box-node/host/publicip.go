// SPDX-License-Identifier: GPL-3.0-or-later

// The public IP detector resolves a bounded, cached public endpoint hint. The
// hint is never used to change a listener or an explicit operator override.

package host

import (
	"context"
	"io"
	"net/http"
	"net/netip"
	"strings"
	"sync"
	"time"

	"github.com/rehuony/sing-box-panel/internal/netguard"
)

// publicIPRefreshInterval is how often Run refreshes the hint, and how long
// before the hint expires it replaces it, so Cached has no gap between
// refreshes.
const publicIPRefreshInterval = 30 * time.Second

type PublicIPDetector struct {
	mu       sync.Mutex
	client   *http.Client
	now      func() time.Time
	value    string
	expires  time.Time
	inflight chan struct{}
}

func NewPublicIPDetector() *PublicIPDetector {
	return &PublicIPDetector{client: &http.Client{
		Timeout:       3 * time.Second,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}, now: time.Now}
}

// Resolve coalesces simultaneous requests and negatively caches failures. No
// credentials, panel URL, node information or user-supplied URL are sent.
func (d *PublicIPDetector) Resolve(ctx context.Context) string {
	return d.resolve(ctx, 0)
}

// resolve detects again once the hint expires within margin. A detection
// that fails while the hint has not expired keeps it, to be retried later.
func (d *PublicIPDetector) resolve(ctx context.Context, margin time.Duration) string {
	for {
		d.mu.Lock()
		if ctx.Err() != nil {
			d.mu.Unlock()
			return ""
		}
		if d.now().Add(margin).Before(d.expires) {
			value := d.value
			d.mu.Unlock()
			return value
		}
		if pending := d.inflight; pending != nil {
			d.mu.Unlock()
			select {
			case <-ctx.Done():
				return ""
			case <-pending:
				continue
			}
		}
		d.inflight = make(chan struct{})
		d.mu.Unlock()
		value := d.detect(ctx)
		d.mu.Lock()
		if value != "" || !d.now().Before(d.expires) {
			d.value = value
			ttl := 30 * time.Second
			if value != "" {
				ttl = 5 * time.Minute
			}
			d.expires = d.now().Add(ttl)
		}
		value = d.value
		close(d.inflight)
		d.inflight = nil
		d.mu.Unlock()
		return value
	}
}

// Cached never waits for network I/O and never returns expired evidence.
func (d *PublicIPDetector) Cached(context.Context) string {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.now().Before(d.expires) {
		return d.value
	}
	return ""
}

// Run owns proactive refreshes for the server lifetime, replacing the hint
// before it expires so a node never reports an empty one between refreshes.
func (d *PublicIPDetector) Run(ctx context.Context) {
	ticker := time.NewTicker(publicIPRefreshInterval)
	defer ticker.Stop()
	for {
		d.resolve(ctx, publicIPRefreshInterval)
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func (d *PublicIPDetector) detect(ctx context.Context) string {
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
	if !netguard.Public(address) {
		return ""
	}
	return address.String()
}
