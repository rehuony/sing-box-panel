// SPDX-License-Identifier: GPL-3.0-or-later

package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

var ErrCatalogStateNotFound = errors.New("catalog state not found")

// CatalogState is the last complete, locally usable official catalog. The
// remote validator is meaningful only together with these exact JSON bytes.
type CatalogState struct {
	Validator   string
	Catalog     json.RawMessage
	Diagnostics json.RawMessage
	RefreshedAt time.Time
}

func (s *Store) SaveCatalogState(ctx context.Context, state CatalogState) (CatalogState, error) {
	prepared, err := prepareCatalogState(state)
	if err != nil {
		return CatalogState{}, err
	}
	_, err = s.db.ExecContext(
		ctx,
		`INSERT INTO catalog_state(singleton, validator, catalog_json, diagnostics_json, refreshed_at)
         VALUES (1, ?, ?, ?, ?)
         ON CONFLICT(singleton) DO UPDATE SET
            validator = excluded.validator,
            catalog_json = excluded.catalog_json,
            diagnostics_json = excluded.diagnostics_json,
            refreshed_at = excluded.refreshed_at`,
		prepared.Validator,
		string(prepared.Catalog),
		string(prepared.Diagnostics),
		formatTaskTime(prepared.RefreshedAt),
	)
	if err != nil {
		return CatalogState{}, fmt.Errorf("save catalog state: %w", err)
	}
	return s.CatalogState(ctx)
}

func (s *Store) CatalogState(ctx context.Context) (CatalogState, error) {
	var state CatalogState
	var catalogJSON, diagnosticsJSON, refreshedAt string
	err := s.db.QueryRowContext(
		ctx,
		`SELECT validator, catalog_json, diagnostics_json, refreshed_at
           FROM catalog_state WHERE singleton = 1`,
	).Scan(&state.Validator, &catalogJSON, &diagnosticsJSON, &refreshedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return CatalogState{}, ErrCatalogStateNotFound
	}
	if err != nil {
		return CatalogState{}, fmt.Errorf("read catalog state: %w", err)
	}
	state.Catalog = append(json.RawMessage(nil), catalogJSON...)
	state.Diagnostics = append(json.RawMessage(nil), diagnosticsJSON...)
	state.RefreshedAt, err = parseTaskTime(refreshedAt)
	if err != nil {
		return CatalogState{}, fmt.Errorf("parse catalog refreshed_at: %w", err)
	}
	return state, nil
}

func prepareCatalogState(state CatalogState) (CatalogState, error) {
	if len(state.Validator) > 16<<10 || strings.ContainsAny(state.Validator, "\x00\r\n") {
		return CatalogState{}, errors.New("catalog validator is invalid")
	}
	catalogJSON, err := compactJSON(state.Catalog, "")
	if err != nil || string(catalogJSON) == "null" {
		return CatalogState{}, errors.New("catalog JSON is invalid")
	}
	diagnosticsJSON, err := compactJSON(state.Diagnostics, "[]")
	if err != nil {
		return CatalogState{}, fmt.Errorf("catalog diagnostics: %w", err)
	}
	var diagnostics []any
	if err := json.Unmarshal(diagnosticsJSON, &diagnostics); err != nil {
		return CatalogState{}, errors.New("catalog diagnostics must be an array")
	}
	if state.RefreshedAt.IsZero() {
		state.RefreshedAt = time.Now().UTC()
	} else {
		state.RefreshedAt = state.RefreshedAt.UTC()
	}
	state.Catalog = catalogJSON
	state.Diagnostics = diagnosticsJSON
	return state, nil
}
