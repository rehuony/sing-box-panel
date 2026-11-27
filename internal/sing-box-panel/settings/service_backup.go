// SPDX-License-Identifier: GPL-3.0-or-later
package settings

import (
	"context"
	"errors"
	"reflect"

	"github.com/rehuony/sing-box-panel/internal/settingsfile"
)

// SavedDocument reads the saved settings document under the settings lock,
// after recovering an interrupted save, as a backup holds it.
func (app *Service) SavedDocument(ctx context.Context) ([]byte, error) {
	if app.settingsPath == "" {
		return nil, errors.New("the panel has no settings file")
	}
	lock, err := settingsfile.Lock(ctx, app.settingsPath)
	if err != nil {
		return nil, err
	}
	defer lock.Close()
	if err := app.Recover(ctx); err != nil {
		return nil, err
	}
	raw, err := Read(app.settingsPath)
	if err != nil {
		return nil, err
	}
	if _, err := Parse(app.settingsPath, raw); err != nil {
		return nil, err
	}
	return raw, nil
}

// RestoredDocument is the settings document a restore writes at path: the
// restored document with the data directory of local, the document there
// now, which a backup never moves, and unless takeService the listener,
// base path and external origin of local too, so that a backup from another
// host serves where this one does. It validates the result.
func RestoredDocument(path string, local, restored []byte, takeService bool) ([]byte, Settings, error) {
	keep := []string{"data_dir"}
	if !takeService {
		keep = append(keep, "server")
	}
	document := upgradeDocument(append(append([]byte(nil), restored...), '\n'))
	for _, name := range keep {
		field, err := settingsfile.FieldOf(reflect.TypeFor[Settings](), name)
		if err != nil {
			return nil, Settings{}, err
		}
		value, err := field.Get(local)
		if err != nil {
			return nil, Settings{}, err
		}
		if document, err = field.Set(document, value); err != nil {
			return nil, Settings{}, err
		}
	}
	if len(document) > MaximumBytes {
		return nil, Settings{}, errors.New("restored settings exceed the settings size limit")
	}
	value, err := Parse(path, document)
	if err != nil {
		return nil, Settings{}, err
	}
	return document, value, nil
}
