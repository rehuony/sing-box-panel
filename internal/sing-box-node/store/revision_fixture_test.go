// SPDX-License-Identifier: GPL-3.0-or-later

package store

import "context"

// recordTestRevision records revision as if the panel had sent it.
func recordTestRevision(ctx context.Context, database *Store, revision NewCanonicalRevision) (CanonicalRevision, error) {
	return database.RecordRevision(ctx, revision.ID, revision.Document, revision.CreatedAt)
}
