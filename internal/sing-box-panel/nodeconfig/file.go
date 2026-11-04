// SPDX-License-Identifier: GPL-3.0-or-later

package nodeconfig

import (
	"context"
	"errors"
	"time"

	"github.com/rehuony/sing-box-panel/internal/configuration"
	"github.com/rehuony/sing-box-panel/internal/sing-box-panel/store"
)

var ErrConfigurationNotSaved = errors.New("no sing-box configuration has been saved; save one in the Web UI before enabling or checking a core")

// Content is always text, so invalid JSON and large numbers cross the browser
// boundary without alteration. SyntaxValid does not claim binary validation.
type ConfigurationFile struct {
	Revision            int64      `json:"revision"`
	Content             string     `json:"content"`
	CanonicalRevisionID string     `json:"canonical_revision_id,omitempty"`
	SyntaxValid         bool       `json:"syntax_valid"`
	UpdatedAt           *time.Time `json:"updated_at,omitempty"`
}

type ConfigurationFileWrite struct {
	Revision int64  `json:"revision"`
	Content  string `json:"content"`
}

func configurationFileView(file store.ConfigurationFile) ConfigurationFile {
	_, err := configuration.Parse([]byte(file.Content))
	result := ConfigurationFile{Revision: file.Revision, Content: file.Content, CanonicalRevisionID: file.CanonicalRevisionID, SyntaxValid: err == nil}
	if !file.UpdatedAt.IsZero() {
		result.UpdatedAt = &file.UpdatedAt
	}
	return result
}

func (application *Service) ConfigurationFile(ctx context.Context) (ConfigurationFile, error) {
	file, err := application.database.ConfigurationFile(ctx)
	if err != nil {
		return ConfigurationFile{}, err
	}
	return configurationFileView(file), nil
}

// InitializeConfigurationFile saves the empty document before the server accepts
// requests. Existing saved text, including incomplete JSON, is left untouched.
func (application *Service) InitializeConfigurationFile(ctx context.Context) error {
	file, err := application.database.ConfigurationFile(ctx)
	if err != nil {
		return err
	}
	if file.Revision > 0 {
		return nil
	}
	_, err = application.SaveConfigurationFile(ctx, ConfigurationFileWrite{Content: string(configuration.Empty().CanonicalJSON())})
	if errors.Is(err, store.ErrConfigurationFileConflict) {
		// Another writer created the document after the read; preserve its save.
		return nil
	}
	return err
}

func (application *Service) SaveConfigurationFile(ctx context.Context, input ConfigurationFileWrite) (ConfigurationFile, error) {
	update, err := store.NewConfigurationFileUpdate(application.random, application.now(), input.Revision, input.Content)
	if err != nil {
		return ConfigurationFile{}, err
	}
	file, err := application.database.SaveConfigurationFile(ctx, update.ExpectedRevision, update.Content, update.Revision)
	if err != nil {
		return ConfigurationFile{}, err
	}
	return configurationFileView(file), nil
}

// RequireParsedFile rejects a saved file that is not valid configuration.
func (application *Service) RequireParsedFile(ctx context.Context) error {
	file, err := application.database.ConfigurationFile(ctx)
	if err != nil {
		return err
	}
	if file.Revision > 0 && file.CanonicalRevisionID == "" {
		return store.ErrConfigurationFileUnparsed
	}
	return nil
}
