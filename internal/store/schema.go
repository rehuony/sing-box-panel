package store

import (
	"context"
	"database/sql"
	_ "embed"
	"fmt"
)

//go:embed schema.sql
var databaseSchema string

type queryRower interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

// SchemaInfo identifies the database format independently of table contents.
type SchemaInfo struct {
	ApplicationID int
	Version       int
}

// initializeSchema creates an empty database or transactionally upgrades versions
// 11 and 12 of this storage epoch. Unrelated and newer formats remain rejected.
func (s *Store) initializeSchema(ctx context.Context) error {
	return s.WithTx(ctx, func(tx *sql.Tx) error {
		applicationID, err := pragmaInt(ctx, tx, "application_id")
		if err != nil {
			return err
		}
		version, err := pragmaInt(ctx, tx, "user_version")
		if err != nil {
			return err
		}
		if applicationID == ApplicationID && version == 11 {
			if _, err := tx.ExecContext(ctx, trafficMonthsSchema); err != nil {
				return err
			}
			if err := seedTrafficMonths(ctx, tx); err != nil {
				return err
			}
			version = 12
		}
		if applicationID == ApplicationID && version == 12 {
			if _, err := tx.ExecContext(ctx, `UPDATE subscription_channels
				SET config_json = json_remove(config_json, '$.export_token_ids')
				WHERE json_type(config_json, '$.export_token_ids') IS NOT NULL`); err != nil {
				return fmt.Errorf("remove subscription export key bindings: %w", err)
			}
			_, err := tx.ExecContext(ctx, "PRAGMA user_version = 13")
			return err
		}
		if applicationID != 0 {
			return validateSchemaIdentity(applicationID, version)
		}
		hasObjects, err := hasUserSchemaObjects(ctx, tx)
		if err != nil {
			return err
		}
		if version != 0 || hasObjects {
			return fmt.Errorf("%w: refusing to adopt an unidentified non-empty database", ErrUnexpectedApplicationID)
		}
		if _, err := tx.ExecContext(ctx, databaseSchema+"\n"+trafficMonthsSchema); err != nil {
			return fmt.Errorf("initialize SQLite schema: %w", err)
		}
		if _, err := tx.ExecContext(ctx, fmt.Sprintf("PRAGMA application_id = %d; PRAGMA user_version = %d", ApplicationID, CurrentSchemaVersion)); err != nil {
			return fmt.Errorf("set SQLite schema identity: %w", err)
		}
		return nil
	})
}

func validateSchemaIdentity(applicationID, version int) error {
	if applicationID != ApplicationID {
		return fmt.Errorf("%w: got %#x, want %#x", ErrUnexpectedApplicationID, applicationID, ApplicationID)
	}
	if version > CurrentSchemaVersion {
		return fmt.Errorf("%w: got %d, supported %d", ErrSchemaTooNew, version, CurrentSchemaVersion)
	}
	if version != CurrentSchemaVersion {
		return fmt.Errorf("%w: got %d, supported %d", ErrSchemaUnsupported, version, CurrentSchemaVersion)
	}
	return nil
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
	version, err := pragmaInt(ctx, s.db, "user_version")
	if err != nil {
		return SchemaInfo{}, err
	}
	if err := validateSchemaIdentity(applicationID, version); err != nil {
		return SchemaInfo{}, err
	}
	return SchemaInfo{ApplicationID: applicationID, Version: version}, nil
}
