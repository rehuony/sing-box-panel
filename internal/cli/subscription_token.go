// SPDX-License-Identifier: GPL-3.0-or-later

package cli

import (
	"fmt"

	"github.com/rehuony/sing-box-panel/internal/application"
	"github.com/spf13/cobra"
)

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
	var expiryRaw, userID, label string
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
				UserID: userID, Label: label, ExpiresAt: expiresAt,
			})
			if err != nil {
				return classifySubscriptionError("subscription_token_create_failed", err)
			}
			text := fmt.Sprintf("created subscription token %s\ntoken\t%s", created.Metadata.ID, created.Token)
			return writeResult(cmd.OutOrStdout(), state.format, created, text)
		},
	}
	command.Flags().StringVar(&expiryRaw, "expires-at", "", "optional exclusive RFC3339 expiry")
	command.Flags().StringVar(&userID, "user-id", "", "required subscription user ID")
	command.Flags().StringVar(&label, "label", "", "required token label")
	_ = command.MarkFlagRequired("user-id")
	_ = command.MarkFlagRequired("label")
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
