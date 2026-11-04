// SPDX-License-Identifier: GPL-3.0-or-later

package nodeconfig

import (
	"crypto/rand"
	"time"

	"github.com/rehuony/sing-box-panel/internal/sing-box-panel/corecache"
	"github.com/rehuony/sing-box-panel/internal/sing-box-panel/store"
)

// Service reads and writes each node's editable configuration file.
type Service struct {
	database *store.Store
	cores    *corecache.Service
	now      func() time.Time
	random   func([]byte) (int, error)
}

// New serves the configuration files in database. Configuration support
// follows the exact versions of the cores the panel cached in cores.
func New(database *store.Store, cores *corecache.Service) *Service {
	return &Service{database: database, cores: cores, now: time.Now, random: rand.Read}
}
