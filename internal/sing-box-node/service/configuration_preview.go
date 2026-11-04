// SPDX-License-Identifier: GPL-3.0-or-later

package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"

	"github.com/rehuony/sing-box-panel/internal/configuration"
	"github.com/rehuony/sing-box-panel/internal/sing-box-node/store"
)

var (
	// ErrNoConfiguration rejects runtime work before the panel sent a
	// configuration.
	ErrNoConfiguration = errors.New("the panel has not sent a configuration to this node")
	// ErrConfigurationDigest rejects configuration bytes that do not match
	// the digest the panel sent with them.
	ErrConfigurationDigest = errors.New("configuration content does not match its digest")
)

// configurationPreview pairs the latest configuration the panel sent with a
// core, as the input of a check or activation.
type configurationPreview struct {
	CanonicalRevision store.CanonicalRevision
	CoreArtifact      store.CoreArtifact
	Config            json.RawMessage
}

func (application *Node) preview(ctx context.Context, coreID string) (configurationPreview, error) {
	core, err := application.database.GetCoreArtifact(ctx, strings.TrimSpace(coreID))
	if err != nil {
		return configurationPreview{}, err
	}
	head, err := application.database.Head(ctx)
	if err != nil {
		return configurationPreview{}, err
	}
	if head == nil {
		return configurationPreview{}, ErrNoConfiguration
	}
	document, err := configuration.Parse(head.Document)
	if err != nil {
		return configurationPreview{}, err
	}
	return configurationPreview{CanonicalRevision: *head, CoreArtifact: core, Config: document.CanonicalJSON()}, nil
}

// ReceiveConfiguration records a configuration the panel sent under its
// revision ID and makes it the one that checks and activations use.
func (application *Node) ReceiveConfiguration(ctx context.Context, revisionID string, content []byte, digest string) (store.CanonicalRevision, error) {
	sum := sha256.Sum256(content)
	if hex.EncodeToString(sum[:]) != strings.ToLower(digest) {
		return store.CanonicalRevision{}, ErrConfigurationDigest
	}
	document, err := configuration.Parse(content)
	if err != nil {
		return store.CanonicalRevision{}, err
	}
	return application.database.RecordRevision(ctx, revisionID, document.CanonicalJSON(), application.now().UTC())
}
