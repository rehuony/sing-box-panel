// SPDX-License-Identifier: GPL-3.0-or-later

package application

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"strings"
	"time"

	"github.com/rehuony/sing-box-panel/internal/jsonstrict"
	"github.com/rehuony/sing-box-panel/internal/settings"
	"github.com/rehuony/sing-box-panel/internal/store"
)

// The journal is temporary recovery material, never a second settings source.
// A database marker decides whether an interrupted file/identity save committed.
type settingsJournal struct {
	ID     string `json:"id"`
	Before []byte `json:"before"`
	After  []byte `json:"after"`
}

func panelFields(p PanelPreferences, key string) settings.Panel {
	return settings.Panel{PublicNodeHost: p.PublicNodeHost, IdentityName: p.IdentityName, IdentityKey: key, Language: p.Language, Appearance: p.Appearance}
}

func panelValues(value settings.Settings) storedPanelSettings {
	return storedPanelSettings{
		Preferences: PanelPreferences{ListenHost: value.Server.Host, ListenPort: value.Server.Port, ExternalOrigin: value.Server.ExternalOrigin,
			PublicNodeHost: value.Panel.PublicNodeHost, IdentityName: value.Panel.IdentityName, TrafficQuotaGiB: value.Traffic.QuotaGiB,
			Language: value.Panel.Language, Appearance: value.Panel.Appearance},
		GitHubToken: value.GitHub.Token, IdentityKey: value.Panel.IdentityKey, ManagementToken: value.Auth.Token,
	}
}

func applyPanelValues(value *settings.Settings, panel storedPanelSettings) {
	value.Server.Host, value.Server.Port, value.Server.ExternalOrigin = panel.Preferences.ListenHost, panel.Preferences.ListenPort, panel.Preferences.ExternalOrigin
	value.Auth.Token, value.Auth.SecureCookie = panel.ManagementToken, strings.HasPrefix(value.Server.ExternalOrigin, "https://")
	value.GitHub.Token = panel.GitHubToken
	value.Traffic.QuotaGiB = panel.Preferences.TrafficQuotaGiB
	value.Panel = panelFields(panel.Preferences, panel.IdentityKey)
}

// encodeSettings retains a relative data_dir instead of rewriting it as the
// absolute runtime location when unrelated Web preferences are saved.
func encodeSettings(value settings.Settings, before []byte) ([]byte, error) {
	var location struct {
		DataDir string `json:"data_dir"`
	}
	if err := json.Unmarshal(before, &location); err != nil {
		return nil, err
	}
	value.DataDir = location.DataDir
	raw, err := json.MarshalIndent(value, "", "  ")
	return append(raw, '\n'), err
}

func (app *Application) currentSettings(ctx context.Context) (settings.Settings, int64, error) {
	if app.settingsPath == "" {
		// Read-only embedded services can have no file. Production composition always
		// supplies the selected path; saving without one is an error.
		value := app.settings
		if value.Panel.Language == "" {
			value.Panel = settings.DefaultPanel()
		}
		if value.Server.Host == "" {
			value.Server.Host = "127.0.0.1"
		}
		if value.Server.Port == 0 {
			value.Server.Port = 3000
		}
		return value, 0, nil
	}
	lock, err := settings.Lock(ctx, app.settingsPath)
	if err != nil {
		return settings.Settings{}, 0, err
	}
	defer lock.Close()
	if err := app.recoverSettingsFile(ctx); err != nil {
		return settings.Settings{}, 0, err
	}
	raw, err := settings.Read(app.settingsPath)
	if err != nil {
		return settings.Settings{}, 0, err
	}
	value, err := settings.Parse(app.settingsPath, raw)
	return value, settings.Revision(raw), err
}

func (app *Application) recoverSettingsFile(ctx context.Context) error {
	path := app.settingsPath + ".pending"
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return errors.New("settings recovery journal must be a regular file")
	}
	file, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	raw, err := io.ReadAll(io.LimitReader(file, 4*settings.MaximumBytes+4097))
	file.Close()
	if err != nil {
		return err
	}
	var journal settingsJournal
	if err := jsonstrict.Decode(raw, 4*settings.MaximumBytes+4096, &journal); err != nil || journal.ID == "" {
		return errors.New("invalid settings recovery journal")
	}
	if _, err := settings.Parse(app.settingsPath, journal.Before); err != nil {
		return errors.New("invalid previous settings in recovery journal")
	}
	if _, err := settings.Parse(app.settingsPath, journal.After); err != nil {
		return errors.New("invalid next settings in recovery journal")
	}
	current, err := settings.ReadRaw(app.settingsPath)
	if err != nil {
		return err
	}
	if !bytes.Equal(current, journal.Before) && !bytes.Equal(current, journal.After) {
		return errors.New("settings changed outside the interrupted update; recovery requires resolving the conflict")
	}
	committed, err := app.database.PanelSettingsFileCommitted(ctx, journal.ID)
	if err != nil {
		return err
	}
	target := journal.Before
	if committed {
		target = journal.After
	}
	if !bytes.Equal(current, target) {
		if err := settings.ReplaceLocked(app.settingsPath, target); err != nil {
			return err
		}
	}
	return settings.RemoveDurable(path)
}

// RecoverPanelSettingsFileLocked finishes an interrupted settings save before
// its database is relocated. The caller must hold settings.Lock and provide
// the database from the recorded current data directory.
func (app *Application) RecoverPanelSettingsFileLocked(ctx context.Context) error {
	return app.recoverSettingsFile(ctx)
}

// commitSettingsFile requires the file lock. The durable journal is published
// before either resource changes and cleared only after a known commit outcome.
func (app *Application) commitSettingsFile(ctx context.Context, before, after []byte, legacy *int64, configuration *store.ConfigurationFileUpdate) error {
	if _, err := settings.Parse(app.settingsPath, after); err != nil {
		return err
	}
	id, err := app.newID("settings")
	if err != nil {
		return err
	}
	journal, err := json.Marshal(settingsJournal{ID: id, Before: before, After: after})
	if err != nil {
		return err
	}
	if err := settings.WriteAtomic(app.settingsPath+".pending", journal); err != nil {
		return err
	}
	err = app.database.CommitPanelSettingsFile(ctx, app.settingsPath, id, legacy, configuration, func() error {
		current, err := settings.ReadRaw(app.settingsPath)
		if err != nil {
			return err
		}
		if !bytes.Equal(current, before) {
			return store.ErrPanelSettingsConflict
		}
		return settings.ReplaceLocked(app.settingsPath, after)
	})
	// Cancellation or a commit error can have an ambiguous outcome. Resolve it
	// using a separate bounded context before permitting another reader/writer.
	recoveryCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	recoveryErr := app.recoverSettingsFile(recoveryCtx)
	return errors.Join(err, recoveryErr)
}

// MigratePanelSettings runs at startup, when the selected file and database are
// both known. Legacy preferences override bootstrap values exactly once. The
// legacy row is removed in the same recoverable transaction as file publication.
func (app *Application) MigratePanelSettings(ctx context.Context) error {
	if app.settingsPath == "" {
		return errors.New("panel settings file path is required")
	}
	lock, err := settings.Lock(ctx, app.settingsPath)
	if err != nil {
		return err
	}
	defer lock.Close()
	if err := app.recoverSettingsFile(ctx); err != nil {
		return err
	}
	before, err := settings.Read(app.settingsPath)
	if err != nil {
		return err
	}
	value, err := settings.Parse(app.settingsPath, before)
	if err != nil {
		return err
	}
	legacy, revision, err := app.database.PanelSettings(ctx)
	if err != nil {
		return err
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(before, &fields); err != nil {
		return err
	}
	if len(legacy) == 0 && fields["panel"] != nil {
		return nil
	}
	if len(legacy) != 0 {
		panel := panelValues(value)
		if err := json.Unmarshal(legacy, &panel); err != nil {
			return errors.New("legacy panel settings are invalid")
		}
		applyPanelValues(&value, panel)
	}
	after, err := encodeSettings(value, before)
	if err != nil {
		return err
	}
	return app.commitSettingsFile(ctx, before, after, &revision, nil)
}
