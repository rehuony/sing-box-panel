// SPDX-License-Identifier: GPL-3.0-or-later

package nodeconfig

import (
	"sync"

	"github.com/rehuony/sing-box-panel/internal/sing-box-panel/schema"
)

// nativeSchemaCapacity bounds the native schemas kept in memory; past it the
// oldest goes first.
const nativeSchemaCapacity = 16

// nativeSchemaCache keeps the schemas cores printed, by archive digest.
type nativeSchemaCache struct {
	mu        sync.Mutex
	contracts map[string]*schema.Contract
	order     []string
}

func (cache *nativeSchemaCache) get(archiveSHA256 string) (*schema.Contract, bool) {
	cache.mu.Lock()
	defer cache.mu.Unlock()
	contract, found := cache.contracts[archiveSHA256]
	return contract, found
}

func (cache *nativeSchemaCache) put(archiveSHA256 string, contract *schema.Contract) {
	cache.mu.Lock()
	defer cache.mu.Unlock()
	if cache.contracts == nil {
		cache.contracts = map[string]*schema.Contract{}
	}
	if _, found := cache.contracts[archiveSHA256]; found {
		return
	}
	if len(cache.order) == nativeSchemaCapacity {
		delete(cache.contracts, cache.order[0])
		cache.order = cache.order[1:]
	}
	cache.contracts[archiveSHA256] = contract
	cache.order = append(cache.order, archiveSHA256)
}
