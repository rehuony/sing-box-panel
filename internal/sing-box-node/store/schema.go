package store

import (
	"context"
	"database/sql"
	_ "embed"

	"github.com/rehuony/sing-box-panel/internal/database"
)

//go:embed schema.sql
var databaseSchema string

type queryRower interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

// SchemaInfo identifies the database format independently of table contents.
type SchemaInfo = database.Info

// SchemaInfo returns the persistent SQLite schema identity and version.
func (s *Store) SchemaInfo(ctx context.Context) (SchemaInfo, error) {
	return s.handle.Info(ctx)
}
