// SPDX-License-Identifier: GPL-3.0-or-later

package application

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"unicode"
	"unicode/utf8"

	"github.com/rehuony/sing-box-panel/internal/store"
	"github.com/rehuony/sing-box-panel/internal/subscription"
)

var (
	ErrPublicSubscriptionAccessDenied       = errors.New("public subscription access denied")
	ErrPublicSubscriptionChannelUnavailable = errors.New("public subscription channel is not in the applied bundle")
)

// PublicSubscriptionResult owns caller-safe response bytes rendered from one
// consistent read of applied local state and live authorization/source
// pointers. Diagnostics contain stable positions and codes only; neither the
// plaintext token nor source/configuration secrets are retained in this value.
type PublicSubscriptionResult struct {
	TokenID     string                          `json:"-"`
	Format      store.SubscriptionFormat        `json:"format"`
	MediaType   string                          `json:"media_type"`
	Body        []byte                          `json:"-"`
	ETag        string                          `json:"etag"`
	NodeCount   int                             `json:"node_count"`
	Diagnostics []subscription.RenderDiagnostic `json:"diagnostics"`
}

// PublicSubscription authenticates the current token/user state and renders
// the current channel from only explicitly granted nodes. The store supplies
// all mutable pointers and the applied local artifact in one consistent read.
func (application *Application) PublicSubscription(
	ctx context.Context,
	plaintextToken string,
	channelID string,
) (PublicSubscriptionResult, error) {
	if plaintextToken == "" || len(plaintextToken) > 512 || !validPublicID(channelID) {
		return PublicSubscriptionResult{}, ErrPublicSubscriptionAccessDenied
	}
	digest := sha256.Sum256([]byte(plaintextToken))
	state, err := application.database.LoadPublicSubscriptionState(
		ctx,
		hex.EncodeToString(digest[:]),
		channelID,
		application.now().UTC(),
	)
	if err != nil {
		switch {
		case errors.Is(err, store.ErrSubscriptionTokenNotFound),
			errors.Is(err, store.ErrSubscriptionTokenInactive):
			return PublicSubscriptionResult{}, ErrPublicSubscriptionAccessDenied
		case errors.Is(err, store.ErrSubscriptionChannelNotFound):
			return PublicSubscriptionResult{}, ErrPublicSubscriptionChannelUnavailable
		default:
			return PublicSubscriptionResult{}, err
		}
	}

	return application.renderSubscriptionState(state)
}

func (application *Application) renderSubscriptionState(state store.PublicSubscriptionState) (PublicSubscriptionResult, error) {
	resolved, err := application.configurationAdapters.Resolve(coreArtifactProfile(state.Core))
	if err != nil || resolved.ID() != state.Startup.AdapterID || resolved.Revision() != state.Startup.AdapterRevision {
		return PublicSubscriptionResult{}, fmt.Errorf("applied startup adapter is unavailable: %w", err)
	}
	startupJSON, err := application.subscriptionStartupJSONWithCore(state.Startup, state.Core)
	if err != nil {
		return PublicSubscriptionResult{}, fmt.Errorf("prepare applied local subscription version: %w", err)
	}
	conversion, err := application.convertInboundNodes(
		state.Startup.ExactCoreVersion, startupJSON, state.Channel.PublicHost,
	)
	if err != nil {
		return PublicSubscriptionResult{}, err
	}
	allNodes := append([]subscription.Node(nil), conversion.Nodes...)
	for _, source := range state.Sources {
		nodes, decodeErr := subscription.DecodeNodes(source.NormalizedNodes)
		if decodeErr != nil {
			return PublicSubscriptionResult{}, fmt.Errorf("decode current source version %q: %w", source.VersionID, decodeErr)
		}
		allNodes = append(allNodes, nodes...)
	}
	granted := make(map[string]struct{}, len(state.Grants))
	for _, key := range state.Grants {
		granted[key] = struct{}{}
	}
	selectedNodes := make([]subscription.Node, 0, len(allNodes))
	for _, node := range allNodes {
		if _, allowed := granted[node.Key]; allowed {
			selectedNodes = append(selectedNodes, node)
		}
	}
	config, err := store.DecodeSubscriptionChannelConfig(state.Channel.Config)
	if err != nil {
		return PublicSubscriptionResult{}, err
	}
	rendered, err := subscription.RenderNodes(selectedNodes, subscription.RenderChannel{
		Format:       subscription.RenderFormat(state.Channel.Format),
		ExcludeTags:  append([]string(nil), config.ExcludeTags...),
		ExcludeTypes: append([]string(nil), config.ExcludeTypes...),
	})
	if err != nil {
		return PublicSubscriptionResult{}, err
	}
	diagnostics := make([]subscription.RenderDiagnostic, 0, len(conversion.Diagnostics)+len(rendered.Diagnostics))
	for _, diagnostic := range conversion.Diagnostics {
		diagnostics = append(diagnostics, subscription.RenderDiagnostic{
			Collection: diagnostic.Collection, ItemIndex: diagnostic.ItemIndex,
			Format: subscription.RenderFormat(state.Channel.Format), Code: diagnostic.Code,
		})
	}
	diagnostics = append(diagnostics, rendered.Diagnostics...)
	bodyDigest := sha256.Sum256(rendered.Content)
	return PublicSubscriptionResult{
		TokenID: state.TokenID, Format: state.Channel.Format, MediaType: rendered.MediaType,
		Body: bytes.Clone(rendered.Content), ETag: hex.EncodeToString(bodyDigest[:]),
		NodeCount: rendered.NodeCount, Diagnostics: diagnostics,
	}, nil
}

func (application *Application) RecordPublicSubscriptionUse(
	ctx context.Context,
	tokenID string,
	bodyBytes int64,
) error {
	return application.database.RecordSubscriptionTokenUse(ctx, tokenID, application.now().UTC(), bodyBytes)
}

func (application *Application) subscriptionStartupJSONWithCore(
	startup store.StartupArtifact,
	_ store.CoreArtifact,
) ([]byte, error) {
	if _, err := subscription.DecodeDocumentObject(startup.ConfigBytes); err != nil {
		return nil, fmt.Errorf("%w: compiled startup is not one strict JSON object", subscription.ErrInvalidStartup)
	}
	return bytes.Clone(startup.ConfigBytes), nil
}

func validPublicID(value string) bool {
	if value == "" || len(value) > 128 || !utf8.ValidString(value) {
		return false
	}
	for _, character := range value {
		if unicode.IsControl(character) || unicode.IsSpace(character) {
			return false
		}
	}
	return true
}
