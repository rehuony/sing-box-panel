package store

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	"io/fs"
	"sort"
	"strconv"
	"strings"
	"time"
)

//go:embed migrations/*.sql
var migrationFiles embed.FS

type migration struct {
	version int
	name    string
	sql     string
}

type queryRower interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

// SchemaInfo identifies the database format independently of table contents.
type SchemaInfo struct {
	ApplicationID   int
	Version         int
	LatestMigration int
}

func (s *Store) migrate(ctx context.Context) error {
	migrations, err := loadMigrations()
	if err != nil {
		return err
	}
	if len(migrations) == 0 || migrations[len(migrations)-1].version != CurrentSchemaVersion {
		return fmt.Errorf(
			"%w: embedded latest version is not %d",
			ErrSchemaInconsistent,
			CurrentSchemaVersion,
		)
	}

	appliedAt := time.Now().UTC().Format(time.RFC3339Nano)
	return s.WithTx(ctx, func(tx *sql.Tx) error {
		applicationID, err := pragmaInt(ctx, tx, "application_id")
		if err != nil {
			return err
		}
		version, err := pragmaInt(ctx, tx, "user_version")
		if err != nil {
			return err
		}

		if applicationID != 0 && applicationID != ApplicationID {
			return fmt.Errorf(
				"%w: got %#x, want %#x",
				ErrUnexpectedApplicationID,
				applicationID,
				ApplicationID,
			)
		}
		if version > CurrentSchemaVersion {
			return fmt.Errorf(
				"%w: got %d, maximum supported is %d",
				ErrSchemaTooNew,
				version,
				CurrentSchemaVersion,
			)
		}

		if applicationID == 0 {
			hasObjects, err := hasUserSchemaObjects(ctx, tx)
			if err != nil {
				return err
			}
			if version != 0 || hasObjects {
				return fmt.Errorf(
					"%w: refusing to adopt an unidentified non-empty database",
					ErrUnexpectedApplicationID,
				)
			}
		}

		for _, migration := range migrations {
			if migration.version <= version {
				continue
			}
			if migration.version != version+1 {
				return fmt.Errorf(
					"%w: migration jumps from %d to %d",
					ErrSchemaInconsistent,
					version,
					migration.version,
				)
			}
			if _, err := tx.ExecContext(ctx, migration.sql); err != nil {
				return fmt.Errorf("apply SQLite migration %s: %w", migration.name, err)
			}
			if _, err := tx.ExecContext(
				ctx,
				`INSERT INTO schema_migrations(version, name, applied_at) VALUES (?, ?, ?)`,
				migration.version,
				migration.name,
				appliedAt,
			); err != nil {
				return fmt.Errorf("record SQLite migration %s: %w", migration.name, err)
			}
			if _, err := tx.ExecContext(
				ctx,
				fmt.Sprintf("PRAGMA user_version = %d", migration.version),
			); err != nil {
				return fmt.Errorf("set SQLite user_version: %w", err)
			}
			version = migration.version
		}

		if applicationID == 0 {
			if _, err := tx.ExecContext(
				ctx,
				fmt.Sprintf("PRAGMA application_id = %d", ApplicationID),
			); err != nil {
				return fmt.Errorf("set SQLite application_id: %w", err)
			}
		}
		return nil
	})
}

func loadMigrations() ([]migration, error) {
	names, err := fs.Glob(migrationFiles, "migrations/*.sql")
	if err != nil {
		return nil, fmt.Errorf("list embedded SQLite migrations: %w", err)
	}
	sort.Strings(names)

	migrations := make([]migration, 0, len(names))
	for _, name := range names {
		base := strings.TrimSuffix(strings.TrimPrefix(name, "migrations/"), ".sql")
		prefix, _, ok := strings.Cut(base, "_")
		if !ok {
			return nil, fmt.Errorf("%w: invalid migration name %q", ErrSchemaInconsistent, name)
		}
		version, err := strconv.Atoi(prefix)
		if err != nil || version < 1 {
			return nil, fmt.Errorf("%w: invalid migration version in %q", ErrSchemaInconsistent, name)
		}
		body, err := migrationFiles.ReadFile(name)
		if err != nil {
			return nil, fmt.Errorf("read embedded SQLite migration %q: %w", name, err)
		}
		if len(migrations) > 0 && version <= migrations[len(migrations)-1].version {
			return nil, fmt.Errorf("%w: duplicate or unordered migration %q", ErrSchemaInconsistent, name)
		}
		migrations = append(migrations, migration{version: version, name: base, sql: string(body)})
	}
	return migrations, nil
}

func hasUserSchemaObjects(ctx context.Context, q queryRower) (bool, error) {
	var count int
	err := q.QueryRowContext(
		ctx,
		`SELECT count(*) FROM sqlite_schema WHERE name NOT LIKE 'sqlite_%'`,
	).Scan(&count)
	if err != nil {
		return false, fmt.Errorf("inspect SQLite schema: %w", err)
	}
	return count != 0, nil
}

func pragmaInt(ctx context.Context, q queryRower, name string) (int, error) {
	var value int
	if err := q.QueryRowContext(ctx, "PRAGMA "+name).Scan(&value); err != nil {
		return 0, fmt.Errorf("read SQLite PRAGMA %s: %w", name, err)
	}
	return value, nil
}

// SchemaInfo returns the persistent SQLite schema identity and version.
func (s *Store) SchemaInfo(ctx context.Context) (SchemaInfo, error) {
	applicationID, err := pragmaInt(ctx, s.db, "application_id")
	if err != nil {
		return SchemaInfo{}, err
	}
	if applicationID != ApplicationID {
		return SchemaInfo{}, fmt.Errorf(
			"%w: got %#x, want %#x",
			ErrUnexpectedApplicationID,
			applicationID,
			ApplicationID,
		)
	}
	version, err := pragmaInt(ctx, s.db, "user_version")
	if err != nil {
		return SchemaInfo{}, err
	}
	if version > CurrentSchemaVersion {
		return SchemaInfo{}, fmt.Errorf(
			"%w: got %d, maximum supported is %d",
			ErrSchemaTooNew,
			version,
			CurrentSchemaVersion,
		)
	}

	var latestMigration int
	if err := s.db.QueryRowContext(
		ctx,
		`SELECT coalesce(max(version), 0) FROM schema_migrations`,
	).Scan(&latestMigration); err != nil {
		return SchemaInfo{}, fmt.Errorf("read SQLite migration ledger: %w", err)
	}
	if latestMigration != version || version != CurrentSchemaVersion {
		return SchemaInfo{}, fmt.Errorf(
			"%w: user_version=%d, latest migration=%d, expected=%d",
			ErrSchemaInconsistent,
			version,
			latestMigration,
			CurrentSchemaVersion,
		)
	}

	return SchemaInfo{
		ApplicationID:   applicationID,
		Version:         version,
		LatestMigration: latestMigration,
	}, nil
}
