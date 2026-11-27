// SPDX-License-Identifier: GPL-3.0-or-later

package subscription

import (
	"crypto/sha256"
	"encoding/json"
	"sync"

	"github.com/rehuony/sing-box-panel/internal/sing-box-panel/store"
)

// The bounds of the render cache: renderings kept, and their content bytes
// together.
const (
	maximumCachedRenders     = 64
	maximumCachedRenderBytes = 64 << 20
)

// renderKey digests everything a rendering is made from.
type renderKey [sha256.Size]byte

// renderCache keeps renderings by a digest of everything they were rendered
// from, so a channel renders once however many clients fetch it while it
// stays the same, and a change, which changes the digest, is never answered
// from the cache. It drops the oldest renderings past its bounds. The zero
// value is ready to use.
type renderCache struct {
	mu      sync.Mutex
	entries map[renderKey]cachedRender
	order   []renderKey
	bytes   int
}

// cachedRender is a rendering and the fleet nodes it carries. Readers share
// it and must not change it.
type cachedRender struct {
	result  RenderResult
	nodeIDs []string
}

func (cache *renderCache) get(key renderKey) (cachedRender, bool) {
	cache.mu.Lock()
	defer cache.mu.Unlock()
	value, found := cache.entries[key]
	return value, found
}

func (cache *renderCache) put(key renderKey, value cachedRender) {
	size := len(value.result.Content)
	if size > maximumCachedRenderBytes {
		return
	}
	cache.mu.Lock()
	defer cache.mu.Unlock()
	if cache.entries == nil {
		cache.entries = map[renderKey]cachedRender{}
	}
	if _, found := cache.entries[key]; found {
		return
	}
	for len(cache.order) > 0 && (len(cache.order) >= maximumCachedRenders || cache.bytes+size > maximumCachedRenderBytes) {
		oldest := cache.order[0]
		cache.order = cache.order[1:]
		cache.bytes -= len(cache.entries[oldest].result.Content)
		delete(cache.entries, oldest)
	}
	cache.entries[key] = value
	cache.order = append(cache.order, key)
	cache.bytes += size
}

// renderDigest digests what RenderChannelNodes reads for a channel, and the
// fleet node each pool node comes from, which the rendering's traffic
// counts.
func renderDigest(pool []store.SubscriptionPoolNode, format store.SubscriptionFormat, config ChannelConfig, ruleSets map[string]ChannelRuleSet) (renderKey, error) {
	digest := sha256.New()
	encoder := json.NewEncoder(digest)
	if err := encoder.Encode([]any{format, config, ruleSets}); err != nil {
		return renderKey{}, err
	}
	for _, node := range pool {
		if err := encoder.Encode([]string{node.ID, node.NodeID, node.SourceID, node.DisplayName, node.OriginName, node.Protocol}); err != nil {
			return renderKey{}, err
		}
		// The outbound is a JSON object, so it cannot run into what follows.
		digest.Write(node.Outbound)
	}
	return renderKey(digest.Sum(nil)), nil
}
