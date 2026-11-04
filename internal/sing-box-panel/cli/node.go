// SPDX-License-Identifier: GPL-3.0-or-later

package cli

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"

	"github.com/rehuony/sing-box-panel/internal/sing-box-panel/app"
	"github.com/rehuony/sing-box-panel/internal/sing-box-panel/fleet"
	"github.com/rehuony/sing-box-panel/internal/sing-box-panel/settings"
	"github.com/rehuony/sing-box-panel/internal/sing-box-panel/store"
)

func newNodeCommand(state *options) *cobra.Command {
	command := group("node", "Manage the nodes this panel controls")
	command.AddCommand(
		&cobra.Command{
			Use:   "add NAME",
			Short: "Register a node and print its one-time enrollment command",
			Args:  cobra.ExactArgs(1),
			RunE: func(cmd *cobra.Command, args []string) error {
				return withFleet(cmd, state, func(nodes *fleet.Service) error {
					enrollment, err := nodes.AddNode(cmd.Context(), args[0])
					if err != nil {
						return err
					}
					return writeResult(cmd.OutOrStdout(), state.format, enrollment, enrollmentText("Added node", enrollment))
				})
			},
		},
		&cobra.Command{
			Use:   "list",
			Short: "List nodes and their state",
			Args:  cobra.NoArgs,
			RunE: func(cmd *cobra.Command, _ []string) error {
				return withFleet(cmd, state, func(nodes *fleet.Service) error {
					list, err := nodes.Nodes(cmd.Context())
					if err != nil {
						return err
					}
					return writeResult(cmd.OutOrStdout(), state.format, list, nodeTable(list))
				})
			},
		},
		&cobra.Command{
			Use:   "enroll NODE",
			Short: "Issue a new enrollment code for a node, which replaces its key when used",
			Args:  cobra.ExactArgs(1),
			RunE: func(cmd *cobra.Command, args []string) error {
				return withFleet(cmd, state, func(nodes *fleet.Service) error {
					node, err := nodes.FindNode(cmd.Context(), args[0])
					if err != nil {
						return err
					}
					enrollment, err := nodes.IssueEnrollment(cmd.Context(), node.ID)
					if err != nil {
						return err
					}
					return writeResult(cmd.OutOrStdout(), state.format, enrollment, enrollmentText("Issued a new code for node", enrollment))
				})
			},
		},
		&cobra.Command{
			Use:   "rename NODE NAME",
			Short: "Rename a node",
			Args:  cobra.ExactArgs(2),
			RunE: func(cmd *cobra.Command, args []string) error {
				return withFleet(cmd, state, func(nodes *fleet.Service) error {
					node, err := nodes.FindNode(cmd.Context(), args[0])
					if err != nil {
						return err
					}
					renamed, err := nodes.RenameNode(cmd.Context(), node.ID, args[1])
					if err != nil {
						return err
					}
					return writeResult(cmd.OutOrStdout(), state.format, renamed, fmt.Sprintf("Renamed node %s to %s", renamed.ID, renamed.Name))
				})
			},
		},
		&cobra.Command{
			Use:   "remove NODE",
			Short: "Remove a node and its key; a running node loses its session",
			Args:  cobra.ExactArgs(1),
			RunE: func(cmd *cobra.Command, args []string) error {
				return withFleet(cmd, state, func(nodes *fleet.Service) error {
					node, err := nodes.FindNode(cmd.Context(), args[0])
					if err != nil {
						return err
					}
					if err := nodes.DeleteNode(cmd.Context(), node.ID); err != nil {
						return err
					}
					return writeResult(cmd.OutOrStdout(), state.format, node, fmt.Sprintf("Removed node %s (%s)", node.Name, node.ID))
				})
			},
		},
	)
	return command
}

// withFleet runs action over the node registry of the panel whose settings
// state selects. The panel server may be running.
func withFleet(cmd *cobra.Command, state *options, action func(*fleet.Service) error) error {
	value, err := settings.Load(state.settingsPath)
	if err != nil {
		return &Error{Kind: ErrorValidation, Code: "settings_invalid", Message: err.Error(), Cause: err}
	}
	database, err := store.Open(cmd.Context(), filepath.Join(value.DataDir, "panel.db"))
	if err != nil {
		return &Error{Kind: ErrorUnavailable, Code: "database_unavailable", Message: err.Error(), Cause: err}
	}
	defer database.Close()
	services := app.New(database, value, app.Options{})
	err = action(services.Fleet)
	switch {
	case err == nil:
		return nil
	case errors.Is(err, store.ErrNodeNotFound):
		return &Error{Kind: ErrorValidation, Code: "node_not_found", Message: "no node has this ID or name", Cause: err}
	case errors.Is(err, store.ErrNodeNameConflict):
		return &Error{Kind: ErrorConflict, Code: "node_name_conflict", Message: err.Error(), Cause: err}
	case errors.Is(err, fleet.ErrNameInvalid), errors.Is(err, fleet.ErrEndpointRequired):
		return &Error{Kind: ErrorValidation, Code: "node_invalid", Message: err.Error(), Cause: err}
	}
	return err
}

func enrollmentText(action string, enrollment fleet.Enrollment) string {
	return fmt.Sprintf("%s %s (%s). Before %s, run on the node:\n  %s",
		action, enrollment.Node.Name, enrollment.Node.ID, enrollment.ExpiresAt.Local().Format(time.DateTime), enrollment.Command)
}

func nodeTable(nodes []fleet.Node) string {
	if len(nodes) == 0 {
		return "No nodes. Add one with: sing-box-panel node add NAME"
	}
	var text strings.Builder
	table := tabwriter.NewWriter(&text, 0, 0, 2, ' ', 0)
	fmt.Fprintln(table, "NAME\tSTATE\tID\tLAST SEEN")
	for _, node := range nodes {
		lastSeen := "—"
		if node.LastSeenAt != nil {
			lastSeen = node.LastSeenAt.Local().Format(time.DateTime)
		}
		fmt.Fprintf(table, "%s\t%s\t%s\t%s\n", node.Name, node.State, node.ID, lastSeen)
	}
	_ = table.Flush()
	return strings.TrimSuffix(text.String(), "\n")
}
