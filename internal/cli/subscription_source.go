// SPDX-License-Identifier: GPL-3.0-or-later

package cli

import (
	"fmt"

	"github.com/rehuony/sing-box-panel/internal/application"
	"github.com/spf13/cobra"
)

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
	command := &cobra.Command{
		Use:   "refresh SOURCE_ID",
		Short: "Queue a durable refresh of the subscription source",
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
			task, err := instance.QueueSubscriptionSourceRefresh(cmd.Context(), sourceID)
			if err != nil {
				return classifySubscriptionError("subscription_source_refresh_failed", err)
			}
			return writeResult(cmd.OutOrStdout(), state.format, task, "queued subscription source refresh "+task.ID)
		},
	}
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
