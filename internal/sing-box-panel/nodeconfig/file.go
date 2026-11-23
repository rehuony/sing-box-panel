// SPDX-License-Identifier: GPL-3.0-or-later

package nodeconfig

import (
	"context"
	"errors"
	"regexp"
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

func (application *Service) ConfigurationFile(ctx context.Context, nodeID string) (ConfigurationFile, error) {
	file, err := application.database.ConfigurationFile(ctx, nodeID)
	if err != nil {
		return ConfigurationFile{}, err
	}
	return configurationFileView(file), nil
}

func (application *Service) SaveConfigurationFile(ctx context.Context, nodeID string, input ConfigurationFileWrite) (ConfigurationFile, error) {
	update, err := store.NewConfigurationFileUpdate(application.random, application.now(), input.Revision, input.Content)
	if err != nil {
		return ConfigurationFile{}, err
	}
	file, err := application.database.SaveConfigurationFile(ctx, nodeID, update.ExpectedRevision, update.Content, update.Revision)
	if err != nil {
		return ConfigurationFile{}, err
	}
	return configurationFileView(file), nil
}

// reportedRevisionID matches the revision IDs the panel issues.
var reportedRevisionID = regexp.MustCompile(`^rev_[0-9a-f]{32}$`)

// SeedConfigurationFile saves the configuration nodeID already runs as its
// file, under the revision ID the node reports for it, so the panel holds the
// revision the node applies and publishes its inbounds without a restart. A
// malformed ID, or one the panel already uses, is replaced by a new one.
func (application *Service) SeedConfigurationFile(ctx context.Context, nodeID string, expected int64, revisionID, content string) (ConfigurationFile, error) {
	update, err := store.NewConfigurationFileUpdate(application.random, application.now(), expected, content)
	if err != nil {
		return ConfigurationFile{}, err
	}
	if reportedRevisionID.MatchString(revisionID) {
		_, err := application.database.GetCanonicalRevision(ctx, revisionID)
		switch {
		case errors.Is(err, store.ErrCanonicalRevisionNotFound):
			update.Revision.ID = revisionID
		case err != nil:
			return ConfigurationFile{}, err
		}
	}
	file, err := application.database.SaveConfigurationFile(ctx, nodeID, update.ExpectedRevision, update.Content, update.Revision)
	if err != nil {
		return ConfigurationFile{}, err
	}
	return configurationFileView(file), nil
}

// CurrentRevision returns the revision of nodeID's saved file, which must
// parse, and the file revision it belongs to.
func (application *Service) CurrentRevision(ctx context.Context, nodeID string) (store.CanonicalRevision, int64, error) {
	file, err := application.database.ConfigurationFile(ctx, nodeID)
	if err != nil {
		return store.CanonicalRevision{}, 0, err
	}
	if file.Revision == 0 {
		return store.CanonicalRevision{}, 0, ErrConfigurationNotSaved
	}
	if file.CanonicalRevisionID == "" {
		return store.CanonicalRevision{}, 0, store.ErrConfigurationFileUnparsed
	}
	revision, err := application.database.GetCanonicalRevision(ctx, file.CanonicalRevisionID)
	return revision, file.Revision, err
}
