package store

import (
	"context"
	"database/sql"
	"embed"

	"github.com/rehuony/sing-box-panel/internal/database"
)

// migrationFiles are the schema migrations of the store's databases, one
// folder for each database file: panel for panel.db and logs for logs.db,
// each numbered from its baseline. A shipped migration is never edited; a
// change adds the next one.
//
//go:embed migrations/panel/*.sql migrations/logs/*.sql
var migrationFiles embed.FS

// Schema is the panel database's identity and its migrations in order.
func Schema() (database.Schema, error) {
	migrations, err := database.LoadMigrations(migrationFiles, "migrations/panel")
	if err != nil {
		return database.Schema{}, err
	}
	return database.Schema{ApplicationID: ApplicationID, Migrations: migrations}, nil
}

// LogSchema is the log database's identity and its migrations in order.
func LogSchema() (database.Schema, error) {
	migrations, err := database.LoadMigrations(migrationFiles, "migrations/logs")
	if err != nil {
		return database.Schema{}, err
	}
	return database.Schema{ApplicationID: LogsApplicationID, Migrations: migrations}, nil
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
