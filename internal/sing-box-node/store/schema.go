package store

import (
	"context"
	"database/sql"
	"embed"

	"github.com/rehuony/sing-box-panel/internal/database"
)

// migrationFiles are the store's schema migrations, numbered from the
// baseline. A shipped migration is never edited; a change adds the next one.
//
//go:embed migrations/*.sql
var migrationFiles embed.FS

// Schema is the store's database identity and its migrations in order.
func Schema() (database.Schema, error) {
	migrations, err := database.LoadMigrations(migrationFiles, "migrations")
	if err != nil {
		return database.Schema{}, err
	}
	return database.Schema{ApplicationID: ApplicationID, Migrations: migrations}, nil
}

type queryRower interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

// SchemaInfo identifies the database format independently of table contents.
type SchemaInfo = database.Info

// SchemaInfo returns the persistent SQLite schema identity and version.
func (s *Store) SchemaInfo(ctx context.Context) (SchemaInfo, error) {
	return s.handle.Info(ctx)
}
