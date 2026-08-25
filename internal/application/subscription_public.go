// SPDX-License-Identifier: GPL-3.0-or-later

package application

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"unicode"
	"unicode/utf8"

	"github.com/rehuony/sing-box-panel/internal/coreartifact"
	"github.com/rehuony/sing-box-panel/internal/jsonstrict"
	"github.com/rehuony/sing-box-panel/internal/manualjson"
	"github.com/rehuony/sing-box-panel/internal/store"
	subscriptionrender "github.com/rehuony/sing-box-panel/internal/subscription"
)

const (
	publicSubscriptionSnapshotSchema = 1
	maximumFrozenChannels            = 256
	maximumFrozenSources             = 256
	maximumPublicSubscriptionBody    = 8 << 20
)

var (
	ErrPublicSubscriptionAccessDenied       = errors.New("public subscription access denied")
	ErrPublicSubscriptionUnknownFormat      = errors.New("unknown public subscription format")
	ErrPublicSubscriptionFormatRequired     = errors.New("public subscription format is required")
	ErrPublicSubscriptionFormatMismatch     = errors.New("public subscription format does not match token channel")
	ErrPublicSubscriptionChannelUnavailable = errors.New("public subscription channel is not in the applied bundle")
	ErrPublicSubscriptionFormatAmbiguous    = errors.New("multiple applied channels use the requested format")
	ErrPublicSubscriptionSnapshotInvalid    = errors.New("applied public subscription snapshot is invalid")
	ErrSubscriptionSnapshotTooLarge         = errors.New("subscription snapshot exceeds its size limit")
)

// PublicSubscriptionResult owns caller-safe response bytes from one applied
// immutable subscription snapshot. Diagnostics contain stable positions and
// codes only; neither the plaintext token nor source/configuration secrets are
// retained in this value.
type PublicSubscriptionResult struct {
	Format      store.SubscriptionFormat        `json:"format"`
	MediaType   string                          `json:"media_type"`
	Body        []byte                          `json:"-"`
	NodeCount   int                             `json:"node_count"`
	Diagnostics []subscriptionrender.Diagnostic `json:"diagnostics"`
}

type subscriptionSnapshotWire struct {
	SchemaVersion int                             `json:"schema_version"`
	Channels      []frozenSubscriptionChannelWire `json:"channels"`
}

type frozenSubscriptionChannelWire struct {
	ChannelID   string                          `json:"channel_id"`
	Format      store.SubscriptionFormat        `json:"format"`
	MediaType   string                          `json:"media_type"`
	Body        []byte                          `json:"body_base64"`
	BodySHA256  string                          `json:"body_sha256"`
	NodeCount   int                             `json:"node_count"`
	Diagnostics []subscriptionrender.Diagnostic `json:"diagnostics"`
}

type frozenSubscriptionSourceWire struct {
	SourceID       string                       `json:"source_id"`
	SourceKind     store.SubscriptionSourceKind `json:"source_kind"`
	Snapshot       json.RawMessage              `json:"snapshot"`
	SnapshotSHA256 string                       `json:"snapshot_sha256"`
}

// PublicSubscription authenticates the token against current revoke/expiry
// state, then selects bytes solely from the activation bundle that one SQL
// statement observed as applied. An empty requested format is accepted only
// for a channel-bound token.
func (application *Application) PublicSubscription(
	ctx context.Context,
	plaintextToken string,
	requestedFormat string,
) (PublicSubscriptionResult, error) {
	format, err := parseRequestedSubscriptionFormat(requestedFormat)
	if err != nil {
		return PublicSubscriptionResult{}, err
	}
	if plaintextToken == "" || len(plaintextToken) > 512 {
		return PublicSubscriptionResult{}, ErrPublicSubscriptionAccessDenied
	}
	digest := sha256.Sum256([]byte(plaintextToken))
	state, err := application.database.LoadPublicSubscriptionState(
		ctx,
		hex.EncodeToString(digest[:]),
		application.now().UTC(),
	)
	if err != nil {
		switch {
		case errors.Is(err, store.ErrSubscriptionTokenNotFound),
			errors.Is(err, store.ErrSubscriptionTokenInactive):
			return PublicSubscriptionResult{}, ErrPublicSubscriptionAccessDenied
		default:
			return PublicSubscriptionResult{}, err
		}
	}

	var snapshot subscriptionSnapshotWire
	if err := jsonstrict.Decode(
		state.Content,
		store.MaximumSubscriptionSnapshotBytes,
		&snapshot,
	); err != nil {
		return PublicSubscriptionResult{}, fmt.Errorf("%w: decode wire", ErrPublicSubscriptionSnapshotInvalid)
	}
	if err := validateSubscriptionSnapshotWire(snapshot); err != nil {
		return PublicSubscriptionResult{}, err
	}
	selected, err := selectFrozenSubscriptionChannel(snapshot.Channels, state.TokenChannelID, format)
	if err != nil {
		return PublicSubscriptionResult{}, err
	}
	return PublicSubscriptionResult{
		Format: selected.Format, MediaType: selected.MediaType,
		Body: bytes.Clone(selected.Body), NodeCount: selected.NodeCount,
		Diagnostics: append([]subscriptionrender.Diagnostic(nil), selected.Diagnostics...),
	}, nil
}

func (application *Application) prepareSubscriptionFreeze(
	ctx context.Context,
	startup store.StartupArtifact,
) (json.RawMessage, json.RawMessage, error) {
	finalStartupJSON, err := application.subscriptionStartupJSON(ctx, startup)
	if err != nil {
		return nil, nil, err
	}
	inputs, err := application.database.LoadSubscriptionPreparationInputs(ctx)
	if err != nil {
		return nil, nil, err
	}
	if len(inputs.Channels) > maximumFrozenChannels {
		return nil, nil, fmt.Errorf("%w: too many enabled channels", ErrSubscriptionSnapshotTooLarge)
	}
	if len(inputs.Sources) > maximumFrozenSources {
		return nil, nil, fmt.Errorf("%w: too many enabled sources", ErrSubscriptionSnapshotTooLarge)
	}

	sources := make([]frozenSubscriptionSourceWire, 0, len(inputs.Sources))
	sourceDocuments := make([][]byte, 0, len(inputs.Sources))
	for _, source := range inputs.Sources {
		snapshot := bytes.Clone(source.LatestSnapshot)
		if len(snapshot) == 0 {
			snapshot = []byte("null")
		} else {
			sourceDocuments = append(sourceDocuments, snapshot)
		}
		digest := sha256.Sum256(snapshot)
		sources = append(sources, frozenSubscriptionSourceWire{
			SourceID: source.ID, SourceKind: source.SourceKind,
			Snapshot: snapshot, SnapshotSHA256: hex.EncodeToString(digest[:]),
		})
	}
	publicationJSON, err := subscriptionrender.MergeSourceSnapshots(finalStartupJSON, sourceDocuments)
	if err != nil {
		return nil, nil, fmt.Errorf("merge enabled subscription sources: %w", err)
	}

	wire := subscriptionSnapshotWire{
		SchemaVersion: publicSubscriptionSnapshotSchema,
		Channels:      make([]frozenSubscriptionChannelWire, 0, len(inputs.Channels)),
	}
	for _, channel := range inputs.Channels {
		config, decodeErr := store.DecodeSubscriptionChannelConfig(channel.Config)
		if decodeErr != nil {
			return nil, nil, decodeErr
		}
		result, renderErr := subscriptionrender.Render(publicationJSON, subscriptionrender.Channel{
			Format:       subscriptionrender.Format(channel.Format),
			ExcludeTags:  append([]string(nil), config.ExcludeTags...),
			ExcludeTypes: append([]string(nil), config.ExcludeTypes...),
		})
		if renderErr != nil {
			return nil, nil, fmt.Errorf("render subscription channel %q: %w", channel.ID, renderErr)
		}
		if len(result.Content) > maximumPublicSubscriptionBody {
			return nil, nil, fmt.Errorf("%w: rendered channel body", ErrSubscriptionSnapshotTooLarge)
		}
		bodyDigest := sha256.Sum256(result.Content)
		wire.Channels = append(wire.Channels, frozenSubscriptionChannelWire{
			ChannelID: channel.ID, Format: channel.Format, MediaType: result.MediaType,
			Body: bytes.Clone(result.Content), BodySHA256: hex.EncodeToString(bodyDigest[:]),
			NodeCount:   result.NodeCount,
			Diagnostics: append([]subscriptionrender.Diagnostic(nil), result.Diagnostics...),
		})
	}
	content, err := json.Marshal(wire)
	if err != nil {
		return nil, nil, fmt.Errorf("encode subscription snapshot: %w", err)
	}
	if int64(len(content)) > store.MaximumSubscriptionSnapshotBytes {
		return nil, nil, ErrSubscriptionSnapshotTooLarge
	}

	sourceSnapshots, err := json.Marshal(sources)
	if err != nil {
		return nil, nil, fmt.Errorf("encode subscription source snapshots: %w", err)
	}
	if int64(len(sourceSnapshots)) > store.MaximumSubscriptionSnapshotBytes {
		return nil, nil, fmt.Errorf("%w: source snapshots", ErrSubscriptionSnapshotTooLarge)
	}
	return content, sourceSnapshots, nil
}

func (application *Application) subscriptionStartupJSON(
	ctx context.Context,
	startup store.StartupArtifact,
) ([]byte, error) {
	switch startup.Kind {
	case store.StartupArtifactStructured:
		var object map[string]any
		if err := jsonstrict.Decode(
			startup.ConfigBytes,
			subscriptionrender.MaximumStartupBytes,
			&object,
		); err != nil || object == nil {
			return nil, fmt.Errorf("%w: structured startup is not one strict JSON object", subscriptionrender.ErrInvalidStartup)
		}
		return bytes.Clone(startup.ConfigBytes), nil
	case store.StartupArtifactManual:
		core, err := application.database.GetCoreArtifact(ctx, startup.CoreArtifactID)
		if err != nil {
			return nil, err
		}
		version, err := coreartifact.ParseExactVersion(startup.ExactCoreVersion)
		if err != nil {
			return nil, err
		}
		binaryDigest, err := coreartifact.ParseSHA256(core.BinarySHA256)
		if err != nil {
			return nil, err
		}
		document, err := manualjson.Parse(startup.ConfigBytes, manualjson.Binding{
			CoreVersion: version, ArtifactDigest: binaryDigest,
			BaseRevisionID: startup.CanonicalRevisionID,
		})
		if err != nil {
			return nil, err
		}
		return document.StandardJSON(), nil
	default:
		return nil, fmt.Errorf("unsupported startup artifact kind %q", startup.Kind)
	}
}

func parseRequestedSubscriptionFormat(value string) (store.SubscriptionFormat, error) {
	if value == "" {
		return "", nil
	}
	format := store.SubscriptionFormat(value)
	if !validPublicSubscriptionFormat(format) {
		return "", ErrPublicSubscriptionUnknownFormat
	}
	return format, nil
}

func validateSubscriptionSnapshotWire(snapshot subscriptionSnapshotWire) error {
	if snapshot.SchemaVersion != publicSubscriptionSnapshotSchema || snapshot.Channels == nil ||
		len(snapshot.Channels) > maximumFrozenChannels {
		return ErrPublicSubscriptionSnapshotInvalid
	}
	seen := make(map[string]struct{}, len(snapshot.Channels))
	for _, channel := range snapshot.Channels {
		if !validFrozenID(channel.ChannelID) || !validPublicSubscriptionFormat(channel.Format) ||
			channel.MediaType != mediaTypeForSubscriptionFormat(channel.Format) ||
			len(channel.Body) > maximumPublicSubscriptionBody || channel.NodeCount < 0 ||
			channel.NodeCount > 10_000 || len(channel.Diagnostics) > 20_000 {
			return ErrPublicSubscriptionSnapshotInvalid
		}
		if _, exists := seen[channel.ChannelID]; exists {
			return ErrPublicSubscriptionSnapshotInvalid
		}
		seen[channel.ChannelID] = struct{}{}
		digest, err := coreartifact.ParseSHA256(channel.BodySHA256)
		if err != nil {
			return ErrPublicSubscriptionSnapshotInvalid
		}
		actual := sha256.Sum256(channel.Body)
		if digest.String() != hex.EncodeToString(actual[:]) {
			return ErrPublicSubscriptionSnapshotInvalid
		}
		for _, diagnostic := range channel.Diagnostics {
			if diagnostic.Format != subscriptionrender.Format(channel.Format) || diagnostic.ItemIndex < 0 ||
				!validSubscriptionCollection(diagnostic.Collection) ||
				!validSubscriptionDiagnosticCode(diagnostic.Code) {
				return ErrPublicSubscriptionSnapshotInvalid
			}
		}
	}
	return nil
}

func selectFrozenSubscriptionChannel(
	channels []frozenSubscriptionChannelWire,
	tokenChannelID string,
	requested store.SubscriptionFormat,
) (frozenSubscriptionChannelWire, error) {
	if tokenChannelID != "" {
		for _, channel := range channels {
			if channel.ChannelID != tokenChannelID {
				continue
			}
			if requested != "" && requested != channel.Format {
				return frozenSubscriptionChannelWire{}, ErrPublicSubscriptionFormatMismatch
			}
			return channel, nil
		}
		return frozenSubscriptionChannelWire{}, ErrPublicSubscriptionChannelUnavailable
	}
	if requested == "" {
		return frozenSubscriptionChannelWire{}, ErrPublicSubscriptionFormatRequired
	}
	var selected *frozenSubscriptionChannelWire
	for index := range channels {
		if channels[index].Format != requested {
			continue
		}
		if selected != nil {
			return frozenSubscriptionChannelWire{}, ErrPublicSubscriptionFormatAmbiguous
		}
		selected = &channels[index]
	}
	if selected == nil {
		return frozenSubscriptionChannelWire{}, ErrPublicSubscriptionChannelUnavailable
	}
	return *selected, nil
}

func validPublicSubscriptionFormat(format store.SubscriptionFormat) bool {
	return format == store.SubscriptionFormatSingBox ||
		format == store.SubscriptionFormatMihomo ||
		format == store.SubscriptionFormatLoon
}

func mediaTypeForSubscriptionFormat(format store.SubscriptionFormat) string {
	switch format {
	case store.SubscriptionFormatSingBox:
		return "application/json"
	case store.SubscriptionFormatMihomo:
		return "application/yaml; charset=utf-8"
	case store.SubscriptionFormatLoon:
		return "text/plain; charset=utf-8"
	default:
		return ""
	}
}

func validFrozenID(value string) bool {
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

func validSubscriptionCollection(value subscriptionrender.Collection) bool {
	return value == subscriptionrender.CollectionOutbounds || value == subscriptionrender.CollectionEndpoints
}

func validSubscriptionDiagnosticCode(value subscriptionrender.DiagnosticCode) bool {
	switch value {
	case subscriptionrender.DiagnosticInvalidOutbound,
		subscriptionrender.DiagnosticInvalidMetadata,
		subscriptionrender.DiagnosticDuplicateTag,
		subscriptionrender.DiagnosticUnsupportedType,
		subscriptionrender.DiagnosticUnsupportedOption,
		subscriptionrender.DiagnosticUnsupportedTransport,
		subscriptionrender.DiagnosticUnsupportedTLS,
		subscriptionrender.DiagnosticUnsupportedNetwork,
		subscriptionrender.DiagnosticUnresolvedDependency,
		subscriptionrender.DiagnosticInvalidRequiredField:
		return true
	default:
		return false
	}
}
