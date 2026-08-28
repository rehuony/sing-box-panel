// SPDX-License-Identifier: GPL-3.0-or-later

package cli

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/rehuony/sing-box-panel/internal/application"
	"github.com/spf13/cobra"
)

func newConfigRevisionCommand(state *options, open openApplicationFunc) *cobra.Command {
	root := group("revision", "Inspect immutable canonical revisions")
	root.AddCommand(
		newRevisionListCommand(state, open),
		newRevisionShowCommand(state, open),
		newRevisionDiffCommand(state, open),
		newRevisionRestoreCommand(state, open),
	)
	return root
}

func newRevisionListCommand(state *options, open openApplicationFunc) *cobra.Command {
	var beforeSequence int64
	var limit int
	command := &cobra.Command{
		Use:   "list",
		Short: "List immutable canonical revisions newest first",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			instance, err := openApplication(cmd.Context(), state.settingsPath, open)
			if err != nil {
				return err
			}
			defer instance.Close()
			page, err := instance.ListCanonicalRevisions(cmd.Context(), beforeSequence, limit)
			if err != nil {
				return &Error{Kind: ErrorValidation, Code: "revision_filter_invalid", Message: err.Error(), Cause: err}
			}
			return writeResult(cmd.OutOrStdout(), state.format, page, revisionPageText(page))
		},
	}
	command.Flags().Int64Var(&beforeSequence, "before-sequence", 0, "exclusive sequence cursor")
	command.Flags().IntVar(&limit, "limit", 50, "maximum revisions to return (1-200)")
	return command
}

func newRevisionShowCommand(state *options, open openApplicationFunc) *cobra.Command {
	return &cobra.Command{
		Use:   "show REVISION",
		Short: "Show a revision by ID or #sequence",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			instance, err := openApplication(cmd.Context(), state.settingsPath, open)
			if err != nil {
				return err
			}
			defer instance.Close()
			revision, err := instance.CanonicalRevision(cmd.Context(), args[0])
			if err != nil {
				return classifyRevisionError("revision_read_failed", err)
			}
			return writeResult(cmd.OutOrStdout(), state.format, revision, prettyRawJSON(revision.Document))
		},
	}
}

func newRevisionDiffCommand(state *options, open openApplicationFunc) *cobra.Command {
	return &cobra.Command{
		Use:   "diff FROM TO",
		Short: "Show deterministic JSON-pointer changes between two revisions",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			instance, err := openApplication(cmd.Context(), state.settingsPath, open)
			if err != nil {
				return err
			}
			defer instance.Close()
			diff, err := instance.DiffCanonicalRevisions(cmd.Context(), args[0], args[1])
			if err != nil {
				return classifyRevisionError("revision_diff_failed", err)
			}
			return writeResult(cmd.OutOrStdout(), state.format, diff, revisionDiffText(diff))
		},
	}
}

func newRevisionRestoreCommand(state *options, open openApplicationFunc) *cobra.Command {
	var baseRevision string
	command := &cobra.Command{
		Use:   "restore REVISION",
		Short: "Restore an old snapshot by creating a new immutable revision",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			expectedHead, err := requiredConfigBaseRevision(cmd, baseRevision)
			if err != nil {
				return err
			}
			instance, err := openApplication(cmd.Context(), state.settingsPath, open)
			if err != nil {
				return err
			}
			defer instance.Close()
			result, err := instance.RestoreCanonicalRevision(cmd.Context(), expectedHead, args[0])
			if err != nil {
				if application.IsRevisionConflict(err) {
					return &Error{Kind: ErrorConflict, Code: "canonical_revision_conflict", Message: err.Error(), Cause: err}
				}
				return classifyRevisionError("revision_restore_failed", err)
			}
			return writeResult(cmd.OutOrStdout(), state.format, result, canonicalSaveText(result))
		},
	}
	command.Flags().StringVar(&baseRevision, "base-revision", "", "current head revision ID used as the compare-and-swap base")
	return command
}

func classifyRevisionError(code string, err error) error {
	if application.IsRevisionNotFound(err) {
		return &Error{Kind: ErrorDomain, Code: "revision_not_found", Message: err.Error(), Cause: err}
	}
	return &Error{Kind: ErrorDomain, Code: code, Message: err.Error(), Cause: err}
}

func revisionPageText(page application.CanonicalRevisionPage) string {
	if len(page.Items) == 0 {
		return "no canonical revisions"
	}
	var output strings.Builder
	for _, revision := range page.Items {
		fmt.Fprintf(&output, "#%d\t%s\t%s\t%s\n", revision.Sequence, revision.ID, revision.SHA256, revision.CreatedAt.Format("2006-01-02T15:04:05.999999999Z07:00"))
	}
	return strings.TrimSuffix(output.String(), "\n")
}

func revisionDiffText(diff application.CanonicalRevisionDiff) string {
	if len(diff.Changes) == 0 {
		return fmt.Sprintf("no changes between #%d and #%d", diff.From.Sequence, diff.To.Sequence)
	}
	var output strings.Builder
	fmt.Fprintf(&output, "FROM\t#%d\t%s\nTO\t#%d\t%s\n", diff.From.Sequence, diff.From.ID, diff.To.Sequence, diff.To.ID)
	for _, change := range diff.Changes {
		fmt.Fprintf(&output, "%s\t%s -> %s\n", change.Path, diffValueText(change.From), diffValueText(change.To))
	}
	return strings.TrimSuffix(output.String(), "\n")
}

func diffValueText(value application.DiffValue) string {
	if !value.Present {
		return "<absent>"
	}
	return strings.ReplaceAll(prettyJSON(value.Value), "\n", " ")
}

func prettyRawJSON(raw []byte) string {
	var value any
	if err := json.Unmarshal(raw, &value); err != nil {
		return string(raw)
	}
	return prettyJSON(value)
}
