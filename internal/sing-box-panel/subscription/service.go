// SPDX-License-Identifier: GPL-3.0-or-later

package subscription

import (
	"crypto/rand"
	"sync"
	"time"

	"github.com/rehuony/sing-box-panel/internal/sing-box-panel/settings"
	"github.com/rehuony/sing-box-panel/internal/sing-box-panel/store"
	"github.com/rehuony/sing-box-panel/internal/sing-box-panel/telemetry"
)

// Options supplies a Service's collaborators.
type Options struct {
	// Now and Random default to the system clock and crypto/rand.
	Now    func() time.Time
	Random func([]byte) (int, error)
}

// Service owns the subscription node pool, sources, channels, keys and
// users, and renders subscriptions for preview and public delivery.
type Service struct {
	database     *store.Store
	settings     *settings.Service
	telemetry    *telemetry.Service
	refreshSlots chan struct{}
	now          func() time.Time
	random       func([]byte) (int, error)
	// poolMu runs pool synchronizations one at a time.
	poolMu sync.Mutex
}

func New(database *store.Store, panelSettings *settings.Service, records *telemetry.Service, options Options) *Service {
	service := &Service{
		database: database, settings: panelSettings, telemetry: records,
		refreshSlots: make(chan struct{}, 3), now: time.Now, random: rand.Read,
	}
	if options.Now != nil {
		service.now = options.Now
	}
	if options.Random != nil {
		service.random = options.Random
	}
	return service
}
