// SPDX-License-Identifier: GPL-3.0-or-later

package cli

import (
	"fmt"
	"strings"

	"github.com/rehuony/sing-box-panel/internal/application"
	"github.com/spf13/cobra"
)

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
	var userID string
	command := &cobra.Command{
		Use:   "render CHANNEL_ID",
		Short: "Preview one channel for an enabled subscription user",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			channelID, err := requiredSubscriptionID(args[0], "channel")
			if err != nil {
				return err
			}
			if strings.TrimSpace(userID) == "" {
				return &Error{Kind: ErrorUsage, Code: "subscription_user_required", Message: "--user is required"}
			}
			userID, err = requiredSubscriptionID(userID, "user")
			if err != nil {
				return err
			}
			instance, err := openApplication(cmd.Context(), state.settingsPath, open)
			if err != nil {
				return err
			}
			defer instance.Close()
			preview, err := instance.RenderSubscriptionPreview(cmd.Context(), userID, channelID)
			if err != nil {
				return classifySubscriptionError("subscription_render_failed", err)
			}
			return writeResult(cmd.OutOrStdout(), state.format, preview, string(preview.Result.Content))
		},
	}
	command.Flags().StringVar(&userID, "user", "", "enabled subscription user ID")
	return command
}
