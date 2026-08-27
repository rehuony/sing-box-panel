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
	ErrPublicSubscriptionChannelUnavailable = errors.New("public subscription channel is not in the applied bundle")
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
// state, then selects one channel solely from the activation bundle that one
// SQL statement observed as applied.
func (application *Application) PublicSubscription(
	ctx context.Context,
	plaintextToken string,
	channelID string,
) (PublicSubscriptionResult, error) {
	if plaintextToken == "" || len(plaintextToken) > 512 || !validFrozenID(channelID) {
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
	selected, err := selectFrozenSubscriptionChannel(snapshot.Channels, channelID)
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
	inputs, err := application.database.LoadSubscriptionPreparationInputs(ctx, store.SubscriptionPreparationLimits{
		MaximumChannels:   maximumFrozenChannels,
		MaximumSources:    maximumFrozenSources,
		MaximumInputBytes: store.MaximumSubscriptionInputBytes,
	})
	if err != nil {
		if errors.Is(err, store.ErrSubscriptionLimitExceeded) {
			return nil, nil, ErrSubscriptionSnapshotTooLarge
		}
		return nil, nil, err
	}
	if len(inputs.Channels) > maximumFrozenChannels {
		return nil, nil, fmt.Errorf("%w: too many enabled channels", ErrSubscriptionSnapshotTooLarge)
	}
	if len(inputs.Sources) > maximumFrozenSources {
		return nil, nil, fmt.Errorf("%w: too many enabled sources", ErrSubscriptionSnapshotTooLarge)
	}

	sourceDocuments := make([][]byte, 0, len(inputs.Sources))
	sourceSnapshots := bytes.NewBuffer(make([]byte, 0, 1024))
	if err := appendBoundedSubscriptionBytes(sourceSnapshots, []byte{'['}); err != nil {
		return nil, nil, err
	}
	for _, source := range inputs.Sources {
		snapshot := source.LatestSnapshot
		if len(snapshot) == 0 {
			snapshot = []byte("null")
		} else {
			sourceDocuments = append(sourceDocuments, snapshot)
		}
		digest := sha256.Sum256(snapshot)
		encoded, encodeErr := json.Marshal(frozenSubscriptionSourceWire{
			SourceID: source.ID, SourceKind: source.SourceKind,
			Snapshot: snapshot, SnapshotSHA256: hex.EncodeToString(digest[:]),
		})
		if encodeErr != nil {
			return nil, nil, fmt.Errorf("encode subscription source snapshot: %w", encodeErr)
		}
		if sourceSnapshots.Len() > 1 {
			if err := appendBoundedSubscriptionBytes(sourceSnapshots, []byte{','}); err != nil {
				return nil, nil, fmt.Errorf("%w: source snapshots", err)
			}
		}
		if err := appendBoundedSubscriptionBytes(sourceSnapshots, encoded); err != nil {
			return nil, nil, fmt.Errorf("%w: source snapshots", err)
		}
	}
	if err := appendBoundedSubscriptionBytes(sourceSnapshots, []byte{']'}); err != nil {
		return nil, nil, fmt.Errorf("%w: source snapshots", err)
	}
	publicationJSON, err := subscriptionrender.MergeSourceSnapshots(finalStartupJSON, sourceDocuments)
	if err != nil {
		return nil, nil, fmt.Errorf("merge enabled subscription sources: %w", err)
	}

	content := bytes.NewBuffer(make([]byte, 0, 1024))
	if err := appendBoundedSubscriptionBytes(
		content,
		[]byte(`{"schema_version":1,"channels":[`),
	); err != nil {
		return nil, nil, err
	}
	for index, channel := range inputs.Channels {
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
		encoded, encodeErr := json.Marshal(frozenSubscriptionChannelWire{
			ChannelID: channel.ID, Format: channel.Format, MediaType: result.MediaType,
			Body: result.Content, BodySHA256: hex.EncodeToString(bodyDigest[:]),
			NodeCount: result.NodeCount, Diagnostics: result.Diagnostics,
		})
		if encodeErr != nil {
			return nil, nil, fmt.Errorf("encode subscription channel %q: %w", channel.ID, encodeErr)
		}
		if index != 0 {
			if err := appendBoundedSubscriptionBytes(content, []byte{','}); err != nil {
				return nil, nil, err
			}
		}
		if err := appendBoundedSubscriptionBytes(content, encoded); err != nil {
			return nil, nil, err
		}
	}
	if err := appendBoundedSubscriptionBytes(content, []byte(`]}`)); err != nil {
		return nil, nil, err
	}
	return json.RawMessage(content.Bytes()), json.RawMessage(sourceSnapshots.Bytes()), nil
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
	channelID string,
) (frozenSubscriptionChannelWire, error) {
	for _, channel := range channels {
		if channel.ChannelID == channelID {
			return channel, nil
		}
	}
	return frozenSubscriptionChannelWire{}, ErrPublicSubscriptionChannelUnavailable
}

func appendBoundedSubscriptionBytes(buffer *bytes.Buffer, value []byte) error {
	if int64(len(value)) > store.MaximumSubscriptionSnapshotBytes-int64(buffer.Len()) {
		return ErrSubscriptionSnapshotTooLarge
	}
	_, _ = buffer.Write(value)
	return nil
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
