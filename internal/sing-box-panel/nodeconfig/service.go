// SPDX-License-Identifier: GPL-3.0-or-later

package nodeconfig

import (
	"crypto/rand"
	"time"

	"github.com/rehuony/sing-box-panel/internal/sing-box-panel/store"
)

// Service reads and writes the editable configuration file.
type Service struct {
	database *store.Store
	now      func() time.Time
	random   func([]byte) (int, error)
}

func New(database *store.Store) *Service {
	return &Service{database: database, now: time.Now, random: rand.Read}
}
