// SPDX-License-Identifier: GPL-3.0-or-later

package nodeconfig

import (
	"context"
	"crypto/rand"
	"time"

	"github.com/rehuony/sing-box-panel/internal/sing-box-panel/corecache"
	"github.com/rehuony/sing-box-panel/internal/sing-box-panel/store"
)

// Service reads and writes each node's editable configuration file.
type Service struct {
	database      *store.Store
	cores         *corecache.Service
	coreSchema    CoreSchemaSource
	nativeSchemas nativeSchemaCache
	now           func() time.Time
	random        func([]byte) (int, error)
}

// keptRevisions is how many of each node's newest configuration revisions
// stay; revisions still in use stay as well.
const keptRevisions = 50

// PruneRevisions removes each node's configuration revisions beyond the
// newest keptRevisions that nothing uses: not the saved text's, not the
// latest that parsed and not the one the node reports applied.
func (service *Service) PruneRevisions(ctx context.Context) (int64, error) {
	return service.database.PruneCanonicalRevisions(ctx, keptRevisions)
}

// New serves the configuration files in database. Configuration support
// follows the release lines of the cores the panel cached in cores; a core of
// a line with its own schema command is asked for its schema through
// coreSchema.
func New(database *store.Store, cores *corecache.Service, coreSchema CoreSchemaSource) *Service {
	return &Service{database: database, cores: cores, coreSchema: coreSchema, now: time.Now, random: rand.Read}
}
