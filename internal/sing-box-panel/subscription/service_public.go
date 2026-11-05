// SPDX-License-Identifier: GPL-3.0-or-later

package subscription

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/rehuony/sing-box-panel/internal/sing-box-panel/store"
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
	TokenID     string                    `json:"-"`
	Format      store.SubscriptionFormat  `json:"format"`
	MediaType   string                    `json:"media_type"`
	Body        []byte                    `json:"-"`
	ETag        string                    `json:"etag"`
	NodeCount   int                       `json:"node_count"`
	Diagnostics []RenderDiagnostic        `json:"diagnostics"`
	Traffic     PublicSubscriptionTraffic `json:"-"`
}

// PublicSubscriptionTraffic reports the instance's recorded period usage,
// including incomplete or stale totals. A zero TotalBytes means unlimited.
type PublicSubscriptionTraffic struct {
	UploadBytes   int64
	DownloadBytes int64
	TotalBytes    int64
}

// PublicSubscription authenticates the current token/user state and renders
// the current channel from only explicitly granted nodes. The store supplies
// all mutable pointers and the applied local artifact in one consistent read.
func (application *Service) PublicSubscription(
	ctx context.Context,
	plaintextToken string,
	channelID string,
) (PublicSubscriptionResult, error) {
	if plaintextToken == "" || len(plaintextToken) > 512 || !validPublicID(channelID) {
		return PublicSubscriptionResult{}, ErrPublicSubscriptionAccessDenied
	}
	digest := sha256.Sum256([]byte(plaintextToken))
	now := application.now().UTC()
	state, err := application.database.LoadPublicSubscriptionState(
		ctx,
		hex.EncodeToString(digest[:]),
		channelID,
		now,
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

	result, err := application.renderSubscriptionState(ctx, state)
	if err != nil {
		return PublicSubscriptionResult{}, err
	}
	traffic, err := application.publicSubscriptionTraffic(ctx, now)
	if err != nil {
		return PublicSubscriptionResult{}, err
	}
	bodyDigest := sha256.Sum256(result.Content)
	return PublicSubscriptionResult{
		TokenID: state.TokenID, Format: store.SubscriptionFormat(result.Format), MediaType: result.MediaType,
		Body: bytes.Clone(result.Content), ETag: hex.EncodeToString(bodyDigest[:]),
		NodeCount: result.NodeCount, Diagnostics: result.Diagnostics, Traffic: traffic,
	}, nil
}

// publicSubscriptionTraffic is the usage the subscription header reports:
// every node's traffic in its own current period. Quotas belong to nodes, so
// the header reports no total.
func (application *Service) publicSubscriptionTraffic(ctx context.Context, at time.Time) (PublicSubscriptionTraffic, error) {
	status, err := application.telemetry.TrafficStatus(ctx, at)
	if err != nil {
		return PublicSubscriptionTraffic{}, err
	}
	return PublicSubscriptionTraffic{UploadBytes: status.UploadBytes, DownloadBytes: status.DownloadBytes}, nil
}

func (application *Service) renderSubscriptionState(ctx context.Context, state store.PublicSubscriptionState) (RenderResult, error) {
	host, err := application.publicationHost(ctx, state.SubscriptionNodeControls, state.Channel.PublicHost)
	if err != nil {
		return RenderResult{}, err
	}
	var conversion InboundResult
	var startupJSON []byte
	if state.AppliedBundleID != "" {
		startupJSON, err = subscriptionStartupJSON(state.Startup)
		if err != nil {
			return RenderResult{}, fmt.Errorf("prepare applied local subscription version: %w", err)
		}
		conversion, err = application.convertInboundNodes(state.Startup.ExactCoreVersion, startupJSON, host)
		if err != nil {
			return RenderResult{}, err
		}
	}
	manual, err := manualPublicationNodes(state.ManualNodes)
	if err != nil {
		return RenderResult{}, err
	}
	allNodes := append(append([]Node(nil), conversion.Nodes...), manual...)
	for _, source := range state.Sources {
		nodes, decodeErr := DecodeNodes(source.NormalizedNodes)
		if decodeErr != nil {
			return RenderResult{}, fmt.Errorf("decode current source version %q: %w", source.VersionID, decodeErr)
		}
		allNodes = append(allNodes, nodes...)
	}
	granted := make(map[string]struct{}, len(state.Grants))
	for _, key := range state.Grants {
		granted[key] = struct{}{}
	}
	selectedNodes := make([]Node, 0, len(allNodes))
	for _, node := range allNodes {
		if _, allowed := granted[node.Key]; (allowed || state.UserID == "") && !state.Visibility[PublicationID(node)].Hidden {
			selectedNodes = append(selectedNodes, node)
		}
	}
	config, err := DecodeChannelConfig(state.Channel.Config)
	if err != nil {
		return RenderResult{}, err
	}
	rendered, err := RenderPolicyNodes(selectedNodes, RenderChannel{
		Format:       RenderFormat(state.Channel.Format),
		ExcludeTags:  append([]string(nil), config.ExcludeTags...),
		ExcludeTypes: append([]string(nil), config.ExcludeTypes...),
		NodeOrder:    publicationNodeIDs(orderSubscriptionNodes(selectedNodes, state.Sources, subscriptionNodeOrders(state.NodeOrders))),
	}, config.Policy)
	if err != nil {
		return RenderResult{}, err
	}
	if config.Policy != nil && rendered.Format == RenderFormatSingBox {
		if err := validateSubscriptionNativeJSON(rendered.Content, "generated_configuration"); err != nil {
			return RenderResult{}, err
		}
	}
	diagnostics := make([]RenderDiagnostic, 0, len(conversion.Diagnostics)+len(rendered.Diagnostics))
	for _, diagnostic := range conversion.Diagnostics {
		diagnostics = append(diagnostics, RenderDiagnostic{
			Collection: diagnostic.Collection, ItemIndex: diagnostic.ItemIndex,
			Format: RenderFormat(state.Channel.Format), Code: diagnostic.Code,
		})
	}
	diagnostics = append(diagnostics, rendered.Diagnostics...)
	rendered.Diagnostics = diagnostics
	rendered.PreviewDiagnostics = append(inboundPreviewDiagnostics(startupJSON, conversion.Diagnostics, rendered.Format), rendered.PreviewDiagnostics...)
	return rendered, nil
}

func (application *Service) RecordPublicSubscriptionUse(
	ctx context.Context,
	tokenID string,
	bodyBytes int64,
) error {
	err := application.database.RecordSubscriptionTokenUse(ctx, tokenID, application.now().UTC(), bodyBytes)
	if errors.Is(err, store.ErrSubscriptionTokenInactive) {
		return ErrPublicSubscriptionAccessDenied
	}
	return err
}

func subscriptionStartupJSON(startup store.AppliedStartup) ([]byte, error) {
	if _, err := DecodeDocumentObject(startup.ConfigBytes); err != nil {
		return nil, fmt.Errorf("%w: compiled startup is not one strict JSON object", ErrInvalidStartup)
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
