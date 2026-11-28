// SPDX-License-Identifier: GPL-3.0-or-later

// Package telemetry owns the panel's operational records: the panel log and
// operation outcomes, metrics and runtime history, traffic periods and quota,
// and the dashboard projection.
package telemetry

import (
	"context"
	"crypto/rand"
	"sync"
	"time"

	"github.com/rehuony/sing-box-panel/internal/sing-box-panel/settings"
	"github.com/rehuony/sing-box-panel/internal/sing-box-panel/store"
)

// Service records and reads panel telemetry.
type Service struct {
	database    *store.Store
	now         func() time.Time
	random      func([]byte) (int, error)
	logObserver func(store.LogEntry)
	classify    Classifier
	settings    *settings.Service
	live        liveNodes

	dashboardsMu sync.Mutex
	root         context.Context
	dashboards   map[string]*dashboard

	// logsMu guards logsRecorded, which the next recorded log entry closes.
	logsMu       sync.Mutex
	logsRecorded chan struct{}
}

// Options supplies a Service's collaborators.
type Options struct {
	// Classify names failures that other services know to be safe to record.
	Classify Classifier
	// Observe receives every recorded log entry, such as for console output.
	Observe func(store.LogEntry)
	// Now and Random default to the system clock and crypto/rand.
	Now    func() time.Time
	Random func([]byte) (int, error)
}

// New records telemetry in database. Traffic accounting reads its quota,
// period and sample retention from panelSettings.
func New(database *store.Store, panelSettings *settings.Service, options Options) *Service {
	service := &Service{
		database:     database,
		now:          time.Now,
		random:       rand.Read,
		logObserver:  options.Observe,
		classify:     options.Classify,
		settings:     panelSettings,
		live:         liveNodes{nodes: map[string]*liveNode{}},
		root:         context.Background(),
		dashboards:   map[string]*dashboard{},
		logsRecorded: make(chan struct{}),
	}
	if options.Now != nil {
		service.now = options.Now
	}
	if options.Random != nil {
		service.random = options.Random
	}
	return service
}

// Bind sets the lifetime of shared reads before requests are served.
func (service *Service) Bind(ctx context.Context) {
	service.dashboardsMu.Lock()
	defer service.dashboardsMu.Unlock()
	service.root = ctx
	for _, entry := range service.dashboards {
		entry.feed.Bind(ctx)
	}
}
