// SPDX-License-Identifier: GPL-3.0-or-later

package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/rehuony/sing-box-panel/internal/application"
	"github.com/rehuony/sing-box-panel/internal/jsonstrict"
	"github.com/rehuony/sing-box-panel/internal/store"
	"github.com/spf13/cobra"
)

const (
	maximumSubscriptionCLIInputBytes    int64 = 5 << 20
	maximumSubscriptionCLIConfigBytes   int64 = 64 << 10
	maximumSubscriptionCLISnapshotBytes int64 = 4 << 20
)

type subscriptionChannelWriteInput struct {
	Name    *string                   `json:"name"`
	Format  *store.SubscriptionFormat `json:"format"`
	Config  json.RawMessage           `json:"config,omitempty"`
	Enabled *bool                     `json:"enabled"`
}

type subscriptionSourceCreateInput struct {
	Name           *string                       `json:"name"`
	SourceKind     *store.SubscriptionSourceKind `json:"source_kind"`
	Config         json.RawMessage               `json:"config,omitempty"`
	LatestSnapshot json.RawMessage               `json:"latest_snapshot,omitempty"`
	Enabled        *bool                         `json:"enabled"`
}

type subscriptionSourceWriteInput struct {
	Name       *string                       `json:"name"`
	SourceKind *store.SubscriptionSourceKind `json:"source_kind"`
	Config     json.RawMessage               `json:"config,omitempty"`
	Enabled    *bool                         `json:"enabled"`
}

func newSubscriptionCommand(state *options, open openApplicationFunc) *cobra.Command {
	root := group("subscription", "Manage subscription channels, sources, and tokens")
	root.AddCommand(
		newSubscriptionChannelCommand(state, open),
		newSubscriptionSourceCommand(state, open),
		newSubscriptionTokenCommand(state, open),
	)
	return root
}

func newSubscriptionChannelCommand(state *options, open openApplicationFunc) *cobra.Command {
	root := group("channel", "Manage public subscription channels")
	root.AddCommand(
		newSubscriptionChannelListCommand(state, open),
		newSubscriptionChannelShowCommand(state, open),
		newSubscriptionChannelCreateCommand(state, open),
		newSubscriptionChannelUpdateCommand(state, open),
		newSubscriptionChannelDeleteCommand(state, open),
		newSubscriptionChannelRenderCommand(state, open),
	)
	return root
}

func newSubscriptionChannelListCommand(state *options, open openApplicationFunc) *cobra.Command {
	var beforeTime, beforeID string
	var limit int
	command := &cobra.Command{
		Use:   "list",
		Short: "List subscription channels",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			cursor, err := parseSubscriptionCursor(beforeTime, beforeID)
			if err != nil {
				return err
			}
			instance, err := openApplication(cmd.Context(), state.settingsPath, open)
			if err != nil {
				return err
			}
			defer instance.Close()
			page, err := instance.ListSubscriptionChannels(cmd.Context(), application.SubscriptionListRequest{
				Cursor: cursor, Limit: limit,
			})
			if err != nil {
				return classifySubscriptionError("subscription_channel_list_failed", err)
			}
			if state.format == outputJSONL {
				return writeSubscriptionJSONLines(cmd.OutOrStdout(), page.Items)
			}
			return writeResult(cmd.OutOrStdout(), state.format, page, subscriptionPageText(
				subscriptionChannelListText(page.Items), page.Next,
			))
		},
	}
	addSubscriptionPageFlags(command, &beforeTime, &beforeID, &limit)
	return command
}

func newSubscriptionChannelShowCommand(state *options, open openApplicationFunc) *cobra.Command {
	return &cobra.Command{
		Use:   "show CHANNEL_ID",
		Short: "Show one subscription channel",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			channelID, err := requiredSubscriptionID(args[0], "channel")
			if err != nil {
				return err
			}
			instance, err := openApplication(cmd.Context(), state.settingsPath, open)
			if err != nil {
				return err
			}
			defer instance.Close()
			channel, err := instance.SubscriptionChannel(cmd.Context(), channelID)
			if err != nil {
				return classifySubscriptionError("subscription_channel_read_failed", err)
			}
			return writeResult(cmd.OutOrStdout(), state.format, channel, prettyJSON(channel))
		},
	}
}

func newSubscriptionChannelCreateCommand(state *options, open openApplicationFunc) *cobra.Command {
	var filePath string
	command := &cobra.Command{
		Use:   "create",
		Short: "Create a subscription channel from a JSON file or stdin",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			var input subscriptionChannelWriteInput
			if err := readSubscriptionControlInput(cmd.InOrStdin(), filePath, "subscription channel", &input); err != nil {
				return err
			}
			request, err := input.createRequest()
			if err != nil {
				return subscriptionInputError("subscription_channel_input_invalid", err)
			}
			instance, err := openApplication(cmd.Context(), state.settingsPath, open)
			if err != nil {
				return err
			}
			defer instance.Close()
			channel, err := instance.CreateSubscriptionChannel(cmd.Context(), request)
			if err != nil {
				return classifySubscriptionError("subscription_channel_create_failed", err)
			}
			text := fmt.Sprintf("created subscription channel %s (updated_at %s)", channel.ID, formatSubscriptionTime(channel.UpdatedAt))
			return writeResult(cmd.OutOrStdout(), state.format, channel, text)
		},
	}
	command.Flags().StringVar(&filePath, "file", "", "strict channel JSON file, or - for stdin")
	return command
}

func newSubscriptionChannelUpdateCommand(state *options, open openApplicationFunc) *cobra.Command {
	var filePath, expectedRaw string
	command := &cobra.Command{
		Use:   "update CHANNEL_ID",
		Short: "Replace a subscription channel using updated_at compare-and-swap",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			channelID, err := requiredSubscriptionID(args[0], "channel")
			if err != nil {
				return err
			}
			expected, err := requiredSubscriptionTime(cmd, expectedRaw)
			if err != nil {
				return err
			}
			var input subscriptionChannelWriteInput
			if err := readSubscriptionControlInput(cmd.InOrStdin(), filePath, "subscription channel", &input); err != nil {
				return err
			}
			request, err := input.updateRequest(expected)
			if err != nil {
				return subscriptionInputError("subscription_channel_input_invalid", err)
			}
			instance, err := openApplication(cmd.Context(), state.settingsPath, open)
			if err != nil {
				return err
			}
			defer instance.Close()
			channel, err := instance.UpdateSubscriptionChannel(cmd.Context(), channelID, request)
			if err != nil {
				return classifySubscriptionError("subscription_channel_update_failed", err)
			}
			text := fmt.Sprintf("updated subscription channel %s (updated_at %s)", channel.ID, formatSubscriptionTime(channel.UpdatedAt))
			return writeResult(cmd.OutOrStdout(), state.format, channel, text)
		},
	}
	command.Flags().StringVar(&filePath, "file", "", "strict replacement channel JSON file, or - for stdin")
	command.Flags().StringVar(&expectedRaw, "updated-at", "", "current updated_at value used for compare-and-swap")
	return command
}

func newSubscriptionChannelDeleteCommand(state *options, open openApplicationFunc) *cobra.Command {
	var expectedRaw string
	command := &cobra.Command{
		Use:   "delete CHANNEL_ID",
		Short: "Delete a subscription channel using updated_at compare-and-swap",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			channelID, err := requiredSubscriptionID(args[0], "channel")
			if err != nil {
				return err
			}
			expected, err := requiredSubscriptionTime(cmd, expectedRaw)
			if err != nil {
				return err
			}
			instance, err := openApplication(cmd.Context(), state.settingsPath, open)
			if err != nil {
				return err
			}
			defer instance.Close()
			if err := instance.DeleteSubscriptionChannel(cmd.Context(), channelID, expected); err != nil {
				return classifySubscriptionError("subscription_channel_delete_failed", err)
			}
			result := map[string]any{"id": channelID, "deleted": true, "expected_updated_at": expected}
			return writeResult(cmd.OutOrStdout(), state.format, result, "deleted subscription channel "+channelID)
		},
	}
	command.Flags().StringVar(&expectedRaw, "updated-at", "", "current updated_at value used for compare-and-swap")
	return command
}

func newSubscriptionChannelRenderCommand(state *options, open openApplicationFunc) *cobra.Command {
	var startupArtifactID string
	command := &cobra.Command{
		Use:   "render CHANNEL_ID",
		Short: "Preview one channel from a checked immutable startup artifact",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			channelID, err := requiredSubscriptionID(args[0], "channel")
			if err != nil {
				return err
			}
			if strings.TrimSpace(startupArtifactID) == "" {
				return &Error{Kind: ErrorUsage, Code: "startup_artifact_required", Message: "--artifact is required"}
			}
			startupArtifactID, err = requiredSubscriptionIdentifier(startupArtifactID, "startup artifact", 256)
			if err != nil {
				return err
			}
			instance, err := openApplication(cmd.Context(), state.settingsPath, open)
			if err != nil {
				return err
			}
			defer instance.Close()
			preview, err := instance.RenderSubscriptionPreview(cmd.Context(), startupArtifactID, channelID)
			if err != nil {
				return classifySubscriptionError("subscription_render_failed", err)
			}
			return writeResult(cmd.OutOrStdout(), state.format, preview, string(preview.Result.Content))
		},
	}
	command.Flags().StringVar(&startupArtifactID, "artifact", "", "ready or stale startup artifact ID")
	return command
}

func newSubscriptionSourceCommand(state *options, open openApplicationFunc) *cobra.Command {
	root := group("source", "Manage attached third-party subscription sources")
	root.AddCommand(
		newSubscriptionSourceListCommand(state, open),
		newSubscriptionSourceShowCommand(state, open),
		newSubscriptionSourceCreateCommand(state, open),
		newSubscriptionSourceUpdateCommand(state, open),
		newSubscriptionSourceRefreshCommand(state, open),
		newSubscriptionSourceDeleteCommand(state, open),
	)
	return root
}

func newSubscriptionSourceListCommand(state *options, open openApplicationFunc) *cobra.Command {
	var beforeTime, beforeID string
	var limit int
	command := &cobra.Command{
		Use:   "list",
		Short: "List attached subscription sources",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			cursor, err := parseSubscriptionCursor(beforeTime, beforeID)
			if err != nil {
				return err
			}
			instance, err := openApplication(cmd.Context(), state.settingsPath, open)
			if err != nil {
				return err
			}
			defer instance.Close()
			page, err := instance.ListSubscriptionSources(cmd.Context(), application.SubscriptionListRequest{
				Cursor: cursor, Limit: limit,
			})
			if err != nil {
				return classifySubscriptionError("subscription_source_list_failed", err)
			}
			if state.format == outputJSONL {
				return writeSubscriptionJSONLines(cmd.OutOrStdout(), page.Items)
			}
			return writeResult(cmd.OutOrStdout(), state.format, page, subscriptionPageText(
				subscriptionSourceListText(page.Items), page.Next,
			))
		},
	}
	addSubscriptionPageFlags(command, &beforeTime, &beforeID, &limit)
	return command
}

func newSubscriptionSourceShowCommand(state *options, open openApplicationFunc) *cobra.Command {
	return &cobra.Command{
		Use:   "show SOURCE_ID",
		Short: "Show one attached subscription source",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			sourceID, err := requiredSubscriptionID(args[0], "source")
			if err != nil {
				return err
			}
			instance, err := openApplication(cmd.Context(), state.settingsPath, open)
			if err != nil {
				return err
			}
			defer instance.Close()
			source, err := instance.SubscriptionSource(cmd.Context(), sourceID)
			if err != nil {
				return classifySubscriptionError("subscription_source_read_failed", err)
			}
			return writeResult(cmd.OutOrStdout(), state.format, source, prettyJSON(source))
		},
	}
}

func newSubscriptionSourceCreateCommand(state *options, open openApplicationFunc) *cobra.Command {
	var filePath string
	command := &cobra.Command{
		Use:   "create",
		Short: "Attach a subscription source from a JSON file or stdin",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			var input subscriptionSourceCreateInput
			if err := readSubscriptionControlInput(cmd.InOrStdin(), filePath, "subscription source", &input); err != nil {
				return err
			}
			request, err := input.request()
			if err != nil {
				return subscriptionInputError("subscription_source_input_invalid", err)
			}
			instance, err := openApplication(cmd.Context(), state.settingsPath, open)
			if err != nil {
				return err
			}
			defer instance.Close()
			source, err := instance.CreateSubscriptionSource(cmd.Context(), request)
			if err != nil {
				return classifySubscriptionError("subscription_source_create_failed", err)
			}
			text := fmt.Sprintf("created subscription source %s (updated_at %s)", source.ID, formatSubscriptionTime(source.UpdatedAt))
			return writeResult(cmd.OutOrStdout(), state.format, source, text)
		},
	}
	command.Flags().StringVar(&filePath, "file", "", "strict source JSON file, or - for stdin")
	return command
}

func newSubscriptionSourceUpdateCommand(state *options, open openApplicationFunc) *cobra.Command {
	var filePath, expectedRaw string
	command := &cobra.Command{
		Use:   "update SOURCE_ID",
		Short: "Replace source metadata using updated_at compare-and-swap",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			sourceID, err := requiredSubscriptionID(args[0], "source")
			if err != nil {
				return err
			}
			expected, err := requiredSubscriptionTime(cmd, expectedRaw)
			if err != nil {
				return err
			}
			var input subscriptionSourceWriteInput
			if err := readSubscriptionControlInput(cmd.InOrStdin(), filePath, "subscription source", &input); err != nil {
				return err
			}
			request, err := input.request(expected)
			if err != nil {
				return subscriptionInputError("subscription_source_input_invalid", err)
			}
			instance, err := openApplication(cmd.Context(), state.settingsPath, open)
			if err != nil {
				return err
			}
			defer instance.Close()
			source, err := instance.UpdateSubscriptionSource(cmd.Context(), sourceID, request)
			if err != nil {
				return classifySubscriptionError("subscription_source_update_failed", err)
			}
			text := fmt.Sprintf("updated subscription source %s (updated_at %s)", source.ID, formatSubscriptionTime(source.UpdatedAt))
			return writeResult(cmd.OutOrStdout(), state.format, source, text)
		},
	}
	command.Flags().StringVar(&filePath, "file", "", "strict replacement source JSON file, or - for stdin")
	command.Flags().StringVar(&expectedRaw, "updated-at", "", "current updated_at value used for compare-and-swap")
	return command
}

func newSubscriptionSourceRefreshCommand(state *options, open openApplicationFunc) *cobra.Command {
	var filePath, expectedRaw string
	command := &cobra.Command{
		Use:   "refresh SOURCE_ID",
		Short: "Store a strict source snapshot for the next successful apply",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			sourceID, err := requiredSubscriptionID(args[0], "source")
			if err != nil {
				return err
			}
			expected, err := requiredSubscriptionTime(cmd, expectedRaw)
			if err != nil {
				return err
			}
			if strings.TrimSpace(filePath) == "" {
				return &Error{Kind: ErrorUsage, Code: "subscription_source_snapshot_file_required", Message: "--file is required; use - for stdin"}
			}
			snapshot, err := readInputFile(cmd.InOrStdin(), filePath, maximumSubscriptionCLISnapshotBytes, "subscription source snapshot")
			if err != nil {
				return subscriptionInputError("subscription_source_snapshot_input_failed", err)
			}
			if err := validateSubscriptionSnapshot(snapshot); err != nil {
				return subscriptionInputError("subscription_source_snapshot_invalid", err)
			}
			instance, err := openApplication(cmd.Context(), state.settingsPath, open)
			if err != nil {
				return err
			}
			defer instance.Close()
			source, err := instance.UpdateSubscriptionSourceSnapshot(cmd.Context(), sourceID, application.UpdateSubscriptionSourceSnapshotRequest{
				LatestSnapshot: snapshot, ExpectedUpdatedAt: expected,
			})
			if err != nil {
				return classifySubscriptionError("subscription_source_refresh_failed", err)
			}
			text := fmt.Sprintf("refreshed subscription source %s (updated_at %s)", source.ID, formatSubscriptionTime(source.UpdatedAt))
			return writeResult(cmd.OutOrStdout(), state.format, source, text)
		},
	}
	command.Flags().StringVar(&filePath, "file", "", "strict source snapshot JSON file, or - for stdin")
	command.Flags().StringVar(&expectedRaw, "updated-at", "", "current updated_at value used for compare-and-swap")
	return command
}

func newSubscriptionSourceDeleteCommand(state *options, open openApplicationFunc) *cobra.Command {
	var expectedRaw string
	command := &cobra.Command{
		Use:   "delete SOURCE_ID",
		Short: "Delete a subscription source using updated_at compare-and-swap",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			sourceID, err := requiredSubscriptionID(args[0], "source")
			if err != nil {
				return err
			}
			expected, err := requiredSubscriptionTime(cmd, expectedRaw)
			if err != nil {
				return err
			}
			instance, err := openApplication(cmd.Context(), state.settingsPath, open)
			if err != nil {
				return err
			}
			defer instance.Close()
			if err := instance.DeleteSubscriptionSource(cmd.Context(), sourceID, expected); err != nil {
				return classifySubscriptionError("subscription_source_delete_failed", err)
			}
			result := map[string]any{"id": sourceID, "deleted": true, "expected_updated_at": expected}
			return writeResult(cmd.OutOrStdout(), state.format, result, "deleted subscription source "+sourceID)
		},
	}
	command.Flags().StringVar(&expectedRaw, "updated-at", "", "current updated_at value used for compare-and-swap")
	return command
}

func newSubscriptionTokenCommand(state *options, open openApplicationFunc) *cobra.Command {
	root := group("token", "Manage public subscription tokens")
	root.AddCommand(
		newSubscriptionTokenListCommand(state, open),
		newSubscriptionTokenCreateCommand(state, open),
		newSubscriptionTokenRotateCommand(state, open),
		newSubscriptionTokenRevokeCommand(state, open),
	)
	return root
}

func newSubscriptionTokenListCommand(state *options, open openApplicationFunc) *cobra.Command {
	var beforeTime, beforeID string
	var limit int
	command := &cobra.Command{
		Use:   "list",
		Short: "List subscription token metadata without plaintext or digests",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			cursor, err := parseSubscriptionCursor(beforeTime, beforeID)
			if err != nil {
				return err
			}
			instance, err := openApplication(cmd.Context(), state.settingsPath, open)
			if err != nil {
				return err
			}
			defer instance.Close()
			page, err := instance.ListSubscriptionTokens(cmd.Context(), application.SubscriptionListRequest{
				Cursor: cursor, Limit: limit,
			})
			if err != nil {
				return classifySubscriptionError("subscription_token_list_failed", err)
			}
			if state.format == outputJSONL {
				return writeSubscriptionJSONLines(cmd.OutOrStdout(), page.Items)
			}
			return writeResult(cmd.OutOrStdout(), state.format, page, subscriptionPageText(
				subscriptionTokenListText(page.Items), page.Next,
			))
		},
	}
	addSubscriptionPageFlags(command, &beforeTime, &beforeID, &limit)
	return command
}

func newSubscriptionTokenCreateCommand(state *options, open openApplicationFunc) *cobra.Command {
	var expiryRaw string
	command := &cobra.Command{
		Use:   "create",
		Short: "Create a subscription token and print its plaintext once",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			expiresAt, err := optionalSubscriptionExpiry(expiryRaw)
			if err != nil {
				return err
			}
			instance, err := openApplication(cmd.Context(), state.settingsPath, open)
			if err != nil {
				return err
			}
			defer instance.Close()
			created, err := instance.CreateSubscriptionToken(cmd.Context(), application.CreateSubscriptionTokenRequest{
				ExpiresAt: expiresAt,
			})
			if err != nil {
				return classifySubscriptionError("subscription_token_create_failed", err)
			}
			text := fmt.Sprintf("created subscription token %s\ntoken\t%s", created.Metadata.ID, created.Token)
			return writeResult(cmd.OutOrStdout(), state.format, created, text)
		},
	}
	command.Flags().StringVar(&expiryRaw, "expires-at", "", "optional exclusive RFC3339 expiry")
	return command
}

func newSubscriptionTokenRotateCommand(state *options, open openApplicationFunc) *cobra.Command {
	var expiryRaw string
	command := &cobra.Command{
		Use:   "rotate TOKEN_ID",
		Short: "Atomically revoke a token and print one replacement plaintext once",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			tokenID, err := requiredSubscriptionID(args[0], "token")
			if err != nil {
				return err
			}
			expiresAt, err := optionalSubscriptionExpiry(expiryRaw)
			if err != nil {
				return err
			}
			instance, err := openApplication(cmd.Context(), state.settingsPath, open)
			if err != nil {
				return err
			}
			defer instance.Close()
			rotation, err := instance.RotateSubscriptionToken(cmd.Context(), tokenID, expiresAt)
			if err != nil {
				return classifySubscriptionError("subscription_token_rotate_failed", err)
			}
			text := fmt.Sprintf("rotated subscription token %s to %s\ntoken\t%s", rotation.Revoked.ID, rotation.Created.ID, rotation.Token)
			return writeResult(cmd.OutOrStdout(), state.format, rotation, text)
		},
	}
	command.Flags().StringVar(&expiryRaw, "expires-at", "", "optional exclusive RFC3339 expiry for the replacement")
	return command
}

func newSubscriptionTokenRevokeCommand(state *options, open openApplicationFunc) *cobra.Command {
	return &cobra.Command{
		Use:   "revoke TOKEN_ID",
		Short: "Revoke a subscription token immediately",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			tokenID, err := requiredSubscriptionID(args[0], "token")
			if err != nil {
				return err
			}
			instance, err := openApplication(cmd.Context(), state.settingsPath, open)
			if err != nil {
				return err
			}
			defer instance.Close()
			token, err := instance.RevokeSubscriptionToken(cmd.Context(), tokenID)
			if err != nil {
				return classifySubscriptionError("subscription_token_revoke_failed", err)
			}
			return writeResult(cmd.OutOrStdout(), state.format, token, "revoked subscription token "+token.ID)
		},
	}
}

func (input subscriptionChannelWriteInput) createRequest() (application.CreateSubscriptionChannelRequest, error) {
	if input.Name == nil || input.Format == nil || input.Enabled == nil {
		return application.CreateSubscriptionChannelRequest{}, errors.New("name, format, and enabled are required")
	}
	if !validSubscriptionCLIName(*input.Name) {
		return application.CreateSubscriptionChannelRequest{}, errors.New("name must be normalized non-empty UTF-8 of at most 128 bytes")
	}
	switch *input.Format {
	case store.SubscriptionFormatSingBox, store.SubscriptionFormatMihomo, store.SubscriptionFormatLoon:
	default:
		return application.CreateSubscriptionChannelRequest{}, errors.New("format must be sing-box, mihomo, or loon")
	}
	if _, err := store.DecodeSubscriptionChannelConfig(input.Config); err != nil {
		return application.CreateSubscriptionChannelRequest{}, err
	}
	return application.CreateSubscriptionChannelRequest{
		Name: *input.Name, Format: *input.Format,
		Config: append(json.RawMessage(nil), input.Config...), Enabled: *input.Enabled,
	}, nil
}

func (input subscriptionChannelWriteInput) updateRequest(expected time.Time) (application.UpdateSubscriptionChannelRequest, error) {
	created, err := input.createRequest()
	if err != nil {
		return application.UpdateSubscriptionChannelRequest{}, err
	}
	return application.UpdateSubscriptionChannelRequest{
		Name: created.Name, Format: created.Format, Config: created.Config,
		Enabled: created.Enabled, ExpectedUpdatedAt: expected,
	}, nil
}

func (input subscriptionSourceCreateInput) request() (application.CreateSubscriptionSourceRequest, error) {
	if input.Name == nil || input.SourceKind == nil || input.Enabled == nil {
		return application.CreateSubscriptionSourceRequest{}, errors.New("name, source_kind, and enabled are required")
	}
	if err := validateSubscriptionSourceFields(*input.Name, *input.SourceKind, input.Config); err != nil {
		return application.CreateSubscriptionSourceRequest{}, err
	}
	if len(input.LatestSnapshot) != 0 {
		if err := validateSubscriptionSnapshot(input.LatestSnapshot); err != nil {
			return application.CreateSubscriptionSourceRequest{}, err
		}
	}
	return application.CreateSubscriptionSourceRequest{
		Name: *input.Name, SourceKind: *input.SourceKind,
		Config:         append(json.RawMessage(nil), input.Config...),
		LatestSnapshot: append(json.RawMessage(nil), input.LatestSnapshot...),
		Enabled:        *input.Enabled,
	}, nil
}

func (input subscriptionSourceWriteInput) request(expected time.Time) (application.UpdateSubscriptionSourceRequest, error) {
	if input.Name == nil || input.SourceKind == nil || input.Enabled == nil {
		return application.UpdateSubscriptionSourceRequest{}, errors.New("name, source_kind, and enabled are required")
	}
	if err := validateSubscriptionSourceFields(*input.Name, *input.SourceKind, input.Config); err != nil {
		return application.UpdateSubscriptionSourceRequest{}, err
	}
	return application.UpdateSubscriptionSourceRequest{
		Name: *input.Name, SourceKind: *input.SourceKind,
		Config: append(json.RawMessage(nil), input.Config...), Enabled: *input.Enabled,
		ExpectedUpdatedAt: expected,
	}, nil
}

func readSubscriptionControlInput(stdin io.Reader, filePath, label string, target any) error {
	if strings.TrimSpace(filePath) == "" {
		return &Error{Kind: ErrorUsage, Code: "subscription_file_required", Message: "--file is required; use - for stdin"}
	}
	data, err := readInputFile(stdin, filePath, maximumSubscriptionCLIInputBytes, label)
	if err != nil {
		return subscriptionInputError("subscription_input_failed", err)
	}
	if err := jsonstrict.Decode(data, maximumSubscriptionCLIInputBytes, target); err != nil {
		return subscriptionInputError("subscription_input_invalid", err)
	}
	return nil
}

func requiredSubscriptionID(raw string, kind string) (string, error) {
	value, err := requiredSubscriptionIdentifier(raw, "subscription "+kind+" ID", 128)
	if err != nil {
		return "", &Error{Kind: ErrorUsage, Code: "subscription_" + kind + "_id_invalid", Message: err.Error(), Cause: err}
	}
	return value, nil
}

func requiredSubscriptionIdentifier(raw, label string, maximum int) (string, error) {
	if raw == "" || raw != strings.TrimSpace(raw) || len(raw) > maximum || strings.ContainsRune(raw, '\x00') {
		return "", fmt.Errorf("%s must be normalized, non-empty, and at most %d bytes", label, maximum)
	}
	return raw, nil
}

func validateSubscriptionSourceFields(name string, kind store.SubscriptionSourceKind, config json.RawMessage) error {
	if !validSubscriptionCLIName(name) {
		return errors.New("name must be normalized non-empty UTF-8 of at most 128 bytes")
	}
	if kind != store.SubscriptionSourceRemote && kind != store.SubscriptionSourceLocal {
		return errors.New("source_kind must be remote or local")
	}
	if len(config) == 0 {
		return nil
	}
	var object map[string]any
	if err := jsonstrict.Decode(config, maximumSubscriptionCLIConfigBytes, &object); err != nil || object == nil {
		return errors.New("config must be one strict non-null JSON object")
	}
	return nil
}

func validateSubscriptionSnapshot(snapshot json.RawMessage) error {
	var value any
	if err := jsonstrict.Decode(snapshot, maximumSubscriptionCLISnapshotBytes, &value); err != nil {
		return err
	}
	switch value.(type) {
	case map[string]any, []any:
		return nil
	default:
		return errors.New("latest_snapshot must be a JSON object or array")
	}
}

func validSubscriptionCLIName(value string) bool {
	if value == "" || value != strings.TrimSpace(value) || len(value) > 128 || !utf8.ValidString(value) {
		return false
	}
	for _, character := range value {
		if unicode.IsControl(character) {
			return false
		}
	}
	return true
}

func requiredSubscriptionTime(cmd *cobra.Command, raw string) (time.Time, error) {
	if !cmd.Flags().Changed("updated-at") || strings.TrimSpace(raw) == "" {
		return time.Time{}, &Error{
			Kind: ErrorUsage, Code: "subscription_updated_at_required",
			Message: "--updated-at is required and must match the current resource updated_at",
		}
	}
	parsed, err := time.Parse(time.RFC3339Nano, strings.TrimSpace(raw))
	if err != nil {
		return time.Time{}, &Error{
			Kind: ErrorUsage, Code: "subscription_updated_at_invalid",
			Message: "--updated-at must be an RFC3339 timestamp", Cause: err,
		}
	}
	return parsed.UTC(), nil
}

func optionalSubscriptionExpiry(raw string) (*time.Time, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	parsed, err := time.Parse(time.RFC3339Nano, raw)
	if err != nil {
		return nil, &Error{
			Kind: ErrorUsage, Code: "subscription_expiry_invalid",
			Message: "--expires-at must be an RFC3339 timestamp", Cause: err,
		}
	}
	parsed = parsed.UTC()
	return &parsed, nil
}

func addSubscriptionPageFlags(
	command *cobra.Command,
	beforeTime *string,
	beforeID *string,
	limit *int,
) {
	command.Flags().StringVar(beforeTime, "before-time", "", "exclusive next-page RFC3339 timestamp (requires --before-id)")
	command.Flags().StringVar(beforeID, "before-id", "", "exclusive next-page resource ID (requires --before-time)")
	command.Flags().IntVar(limit, "limit", 50, "maximum resources to return (1-200)")
}

func parseSubscriptionCursor(rawTime string, rawID string) (*application.SubscriptionCursor, error) {
	rawTime = strings.TrimSpace(rawTime)
	rawID = strings.TrimSpace(rawID)
	if rawTime == "" && rawID == "" {
		return nil, nil
	}
	if rawTime == "" || rawID == "" {
		return nil, &Error{
			Kind: ErrorUsage, Code: "subscription_cursor_invalid",
			Message: "--before-time and --before-id must be provided together",
		}
	}
	createdAt, err := time.Parse(time.RFC3339Nano, rawTime)
	if err != nil {
		return nil, &Error{
			Kind: ErrorUsage, Code: "subscription_cursor_invalid",
			Message: "--before-time must be an RFC3339 timestamp", Cause: err,
		}
	}
	identifier, err := requiredSubscriptionID(rawID, "cursor")
	if err != nil {
		return nil, err
	}
	return &application.SubscriptionCursor{CreatedAt: createdAt.UTC(), ID: identifier}, nil
}

func subscriptionPageText(value string, next *application.SubscriptionCursor) string {
	if next == nil {
		return value
	}
	return fmt.Sprintf(
		"%s\nnext\t--before-time=%s --before-id=%s",
		value,
		formatSubscriptionTime(next.CreatedAt),
		next.ID,
	)
}

func writeSubscriptionJSONLines[T any](writer io.Writer, items []T) error {
	encoder := json.NewEncoder(writer)
	encoder.SetEscapeHTML(false)
	for _, item := range items {
		if err := encoder.Encode(item); err != nil {
			return err
		}
	}
	return nil
}

func subscriptionInputError(code string, err error) error {
	return &Error{Kind: ErrorValidation, Code: code, Message: err.Error(), Cause: err}
}

func classifySubscriptionError(code string, err error) error {
	switch {
	case errors.Is(err, store.ErrSubscriptionConflict):
		return &Error{Kind: ErrorConflict, Code: "subscription_conflict", Message: err.Error(), Cause: err}
	case errors.Is(err, store.ErrSubscriptionChannelExists):
		return &Error{Kind: ErrorConflict, Code: "subscription_channel_exists", Message: err.Error(), Cause: err}
	case errors.Is(err, store.ErrSubscriptionLimitExceeded), errors.Is(err, application.ErrSubscriptionSnapshotTooLarge):
		return &Error{Kind: ErrorValidation, Code: "subscription_limit_exceeded", Message: err.Error(), Cause: err}
	case errors.Is(err, store.ErrSubscriptionSourceExists):
		return &Error{Kind: ErrorConflict, Code: "subscription_source_exists", Message: err.Error(), Cause: err}
	case errors.Is(err, store.ErrSubscriptionTokenExists):
		return &Error{Kind: ErrorConflict, Code: "subscription_token_exists", Message: err.Error(), Cause: err}
	case errors.Is(err, store.ErrSubscriptionTokenInactive):
		return &Error{Kind: ErrorConflict, Code: "subscription_token_inactive", Message: err.Error(), Cause: err}
	case errors.Is(err, application.ErrSubscriptionPreviewArtifactState), errors.Is(err, store.ErrStartupArtifactState):
		return &Error{Kind: ErrorConflict, Code: "subscription_preview_artifact_state", Message: err.Error(), Cause: err}
	case errors.Is(err, store.ErrSubscriptionChannelNotFound):
		return &Error{Kind: ErrorDomain, Code: "subscription_channel_not_found", Message: err.Error(), Cause: err}
	case errors.Is(err, store.ErrSubscriptionSourceNotFound):
		return &Error{Kind: ErrorDomain, Code: "subscription_source_not_found", Message: err.Error(), Cause: err}
	case errors.Is(err, store.ErrSubscriptionTokenNotFound):
		return &Error{Kind: ErrorDomain, Code: "subscription_token_not_found", Message: err.Error(), Cause: err}
	case errors.Is(err, store.ErrStartupArtifactNotFound):
		return &Error{Kind: ErrorDomain, Code: "startup_artifact_not_found", Message: err.Error(), Cause: err}
	default:
		return &Error{Kind: ErrorDomain, Code: code, Message: err.Error(), Cause: err}
	}
}

func subscriptionChannelListText(values []application.SubscriptionChannelSummary) string {
	if len(values) == 0 {
		return "no subscription channels"
	}
	var output strings.Builder
	output.WriteString("ID\tNAME\tFORMAT\tENABLED\tUPDATED_AT\n")
	for _, value := range values {
		fmt.Fprintf(&output, "%s\t%s\t%s\t%t\t%s\n", value.ID, value.Name, value.Format, value.Enabled, formatSubscriptionTime(value.UpdatedAt))
	}
	return strings.TrimSuffix(output.String(), "\n")
}

func subscriptionSourceListText(values []application.SubscriptionSourceSummary) string {
	if len(values) == 0 {
		return "no subscription sources"
	}
	var output strings.Builder
	output.WriteString("ID\tNAME\tKIND\tENABLED\tSNAPSHOT\tUPDATED_AT\n")
	for _, value := range values {
		fmt.Fprintf(&output, "%s\t%s\t%s\t%t\t%t\t%s\n", value.ID, value.Name, value.SourceKind, value.Enabled, value.HasSnapshot, formatSubscriptionTime(value.UpdatedAt))
	}
	return strings.TrimSuffix(output.String(), "\n")
}

func subscriptionTokenListText(values []application.SubscriptionToken) string {
	if len(values) == 0 {
		return "no subscription tokens"
	}
	var output strings.Builder
	output.WriteString("ID\tACTIVE\tEXPIRES_AT\tREVOKED_AT\tCREATED_AT\n")
	for _, value := range values {
		fmt.Fprintf(
			&output, "%s\t%t\t%s\t%s\t%s\n",
			value.ID, value.Active,
			formatOptionalSubscriptionTime(value.ExpiresAt), formatOptionalSubscriptionTime(value.RevokedAt),
			formatSubscriptionTime(value.CreatedAt),
		)
	}
	return strings.TrimSuffix(output.String(), "\n")
}

func formatSubscriptionTime(value time.Time) string {
	return value.UTC().Format(time.RFC3339Nano)
}

func formatOptionalSubscriptionTime(value *time.Time) string {
	if value == nil {
		return "-"
	}
	return formatSubscriptionTime(*value)
}
