// SPDX-License-Identifier: GPL-3.0-or-later

package cli

import (
	"encoding/json"
	"fmt"

	"github.com/rehuony/sing-box-panel/internal/application"
	"github.com/rehuony/sing-box-panel/internal/canonical"
	"github.com/rehuony/sing-box-panel/internal/jsonstrict"
	"github.com/spf13/cobra"
)

func newNodeCheckCommand(state *options, open openApplicationFunc) *cobra.Command {
	return &cobra.Command{
		Use:   "check NODE_ID",
		Short: "Check one canonical node's stored structure and references",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			instance, err := openApplication(cmd.Context(), state.settingsPath, open)
			if err != nil {
				return err
			}
			defer instance.Close()
			revision, node, err := instance.GetEntity(cmd.Context(), canonical.CollectionNodes, args[0])
			if err != nil {
				return classifyCanonicalError("node_check_failed", err)
			}
			result := struct {
				Revision            application.CanonicalSnapshot `json:"revision"`
				Node                map[string]any                `json:"node"`
				StructurallyValid   bool                          `json:"structurally_valid"`
				NetworkProbeApplied bool                          `json:"network_probe_applied"`
			}{Revision: revision, Node: node, StructurallyValid: true, NetworkProbeApplied: false}
			return writeResult(
				cmd.OutOrStdout(), state.format, result,
				fmt.Sprintf("node %s is structurally valid; no network probe was performed", args[0]),
			)
		},
	}
}

func newNodeImportCommand(state *options, open openApplicationFunc) *cobra.Command {
	var filePath, baseRevision string
	command := &cobra.Command{
		Use:   "import",
		Short: "Replace the canonical node collection from a JSON array",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			expectedHead, err := requiredConfigBaseRevision(cmd, baseRevision)
			if err != nil {
				return err
			}
			if filePath == "" {
				return &Error{Kind: ErrorUsage, Code: "file_required", Message: "--file is required; use - for stdin"}
			}
			raw, err := readInputFile(cmd.InOrStdin(), filePath, canonical.MaximumBytes, "node import")
			if err != nil {
				return &Error{Kind: ErrorValidation, Code: "node_import_input_failed", Message: err.Error(), Cause: err}
			}
			var nodes []map[string]any
			if err := jsonstrict.Decode(raw, canonical.MaximumBytes, &nodes); err != nil || nodes == nil {
				if err == nil {
					err = fmt.Errorf("node import must be a JSON array")
				}
				return &Error{Kind: ErrorValidation, Code: "node_import_invalid", Message: err.Error(), Cause: err}
			}
			canonicalValue, err := json.Marshal(nodes)
			if err != nil {
				return &Error{Kind: ErrorValidation, Code: "node_import_invalid", Message: err.Error(), Cause: err}
			}
			instance, err := openApplication(cmd.Context(), state.settingsPath, open)
			if err != nil {
				return err
			}
			defer instance.Close()
			result, err := instance.SetCanonicalValue(cmd.Context(), expectedHead, "/nodes", canonicalValue)
			if err != nil {
				return classifyCanonicalError("node_import_failed", err)
			}
			return writeResult(cmd.OutOrStdout(), state.format, result, canonicalSaveText(result))
		},
	}
	command.Flags().StringVar(&filePath, "file", "", "node JSON array file, or - for stdin")
	command.Flags().StringVar(&baseRevision, "base-revision", "", "revision ID used as the compare-and-swap base")
	return command
}
