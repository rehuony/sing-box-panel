// SPDX-License-Identifier: GPL-3.0-or-later

package subscription

import (
	"encoding/json"
	"testing"

	"github.com/rehuony/sing-box-panel/internal/sing-box-panel/store"
)

func TestRenderCacheKeysOnEverythingARenderingReadsAndStaysBounded(t *testing.T) {
	pool := []store.SubscriptionPoolNode{{ID: "sn_a", SourceID: "src", DisplayName: "A", OriginName: "A", Protocol: "socks", Outbound: json.RawMessage(`{"type":"socks"}`)}}
	config := ChannelConfig{IncompatibleNodes: "skip", DefaultExit: RouteExit{Kind: "direct"}}
	key, err := renderDigest(pool, store.SubscriptionFormatSingBox, config, nil)
	if err != nil {
		t.Fatal(err)
	}
	renamed := append([]store.SubscriptionPoolNode{}, pool...)
	renamed[0].DisplayName = "B"
	for _, other := range []func() (renderKey, error){
		func() (renderKey, error) { return renderDigest(renamed, store.SubscriptionFormatSingBox, config, nil) },
		func() (renderKey, error) { return renderDigest(pool, store.SubscriptionFormatMihomo, config, nil) },
		func() (renderKey, error) {
			return renderDigest(pool, store.SubscriptionFormatSingBox, config, map[string]ChannelRuleSet{"set": {Client: RenderFormatSingBox}})
		},
	} {
		if changed, err := other(); err != nil || changed == key {
			t.Fatalf("a changed input kept the key: %v", err)
		}
	}

	var cache renderCache
	for index := range maximumCachedRenders + 1 {
		cache.put(renderKey{byte(index)}, cachedRender{result: RenderResult{Content: []byte("x")}})
	}
	if _, found := cache.get(renderKey{0}); found || len(cache.entries) != maximumCachedRenders {
		t.Fatalf("cache holds %d renderings, the oldest kept: %v", len(cache.entries), found)
	}
	cache.put(renderKey{0xff}, cachedRender{result: RenderResult{Content: make([]byte, maximumCachedRenderBytes)}})
	if _, found := cache.get(renderKey{0xff}); !found || len(cache.entries) != 1 || cache.bytes != maximumCachedRenderBytes {
		t.Fatalf("cache = %d renderings, %d bytes", len(cache.entries), cache.bytes)
	}
}
