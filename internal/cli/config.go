// SPDX-License-Identifier: GPL-3.0-or-later

package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/rehuony/sing-box-panel/internal/application"
	"github.com/rehuony/sing-box-panel/internal/canonical"
	"github.com/spf13/cobra"
)

type openApplicationFunc func(context.Context, string) (*application.Application, error)

func newConfigShowCommand(state *options, open openApplicationFunc) *cobra.Command {
	return &cobra.Command{
		Use:   "show",
		Short: "Show the current global canonical revision",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			instance, err := openApplication(cmd.Context(), state.settingsPath, open)
			if err != nil {
				return err
			}
			defer instance.Close()
			head, err := instance.CanonicalHead(cmd.Context())
			if err != nil {
				return &Error{Kind: ErrorDomain, Code: "canonical_read_failed", Message: err.Error(), Cause: err}
			}
			if head == nil {
				return &Error{Kind: ErrorDomain, Code: "canonical_not_initialized", Message: "no canonical revision has been saved"}
			}
			var pretty bytes.Buffer
			if err := json.Indent(&pretty, head.Document, "", "  "); err != nil {
				return &Error{Kind: ErrorDomain, Code: "canonical_invalid", Message: err.Error(), Cause: err}
			}
			return writeResult(cmd.OutOrStdout(), state.format, head, pretty.String())
		},
	}
}

func newConfigReplaceCommand(state *options, open openApplicationFunc) *cobra.Command {
	var filePath string
	var baseRevision string
	command := &cobra.Command{
		Use:   "replace",
		Short: "Replace the complete canonical document using a base revision",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if filePath == "" {
				return &Error{Kind: ErrorUsage, Code: "file_required", Message: "--file is required; use - for stdin"}
			}
			if !cmd.Flags().Changed("base-revision") {
				return &Error{Kind: ErrorUsage, Code: "base_revision_required", Message: "--base-revision is required; use none for the first revision"}
			}
			expectedHead := strings.TrimSpace(baseRevision)
			if expectedHead == "none" {
				expectedHead = ""
			} else if expectedHead == "" {
				return &Error{Kind: ErrorUsage, Code: "base_revision_invalid", Message: "--base-revision must be a revision ID or none"}
			}
			document, err := readCanonicalInput(cmd.InOrStdin(), filePath)
			if err != nil {
				return &Error{Kind: ErrorValidation, Code: "canonical_input_failed", Message: err.Error(), Cause: err}
			}
			instance, err := openApplication(cmd.Context(), state.settingsPath, open)
			if err != nil {
				return err
			}
			defer instance.Close()
			result, err := instance.ReplaceCanonical(cmd.Context(), expectedHead, document)
			if err != nil {
				if application.IsRevisionConflict(err) {
					return &Error{Kind: ErrorConflict, Code: "canonical_revision_conflict", Message: err.Error(), Cause: err}
				}
				if errors.Is(err, canonical.ErrInvalidDocument) {
					return &Error{Kind: ErrorValidation, Code: "canonical_invalid", Message: err.Error(), Cause: err}
				}
				return &Error{Kind: ErrorDomain, Code: "canonical_save_failed", Message: err.Error(), Cause: err}
			}
			text := fmt.Sprintf("saved canonical revision #%d %s", result.Revision.Sequence, result.Revision.ID)
			if result.NoChange {
				text = fmt.Sprintf("canonical revision #%d %s is unchanged", result.Revision.Sequence, result.Revision.ID)
			} else if result.TaskID != "" {
				text += " (task " + result.TaskID + ")"
			}
			return writeResult(cmd.OutOrStdout(), state.format, result, text)
		},
	}
	command.Flags().StringVar(&filePath, "file", "", "canonical JSON file, or - for stdin")
	command.Flags().StringVar(&baseRevision, "base-revision", "", "revision ID used as the compare-and-swap base, or none")
	return command
}

func openApplication(
	ctx context.Context,
	settingsPath string,
	open openApplicationFunc,
) (*application.Application, error) {
	if open == nil {
		return nil, &Error{Kind: ErrorUnavailable, Code: "application_unavailable", Message: "application services are unavailable"}
	}
	instance, err := open(ctx, settingsPath)
	if err != nil {
		return nil, &Error{Kind: ErrorValidation, Code: "application_open_failed", Message: err.Error(), Cause: err}
	}
	return instance, nil
}

func readCanonicalInput(stdin io.Reader, filePath string) ([]byte, error) {
	var reader io.Reader
	var closeFile func() error
	if filePath == "-" {
		reader = stdin
	} else {
		file, err := os.Open(filePath)
		if err != nil {
			return nil, fmt.Errorf("open canonical input: %w", err)
		}
		reader = file
		closeFile = file.Close
	}
	if closeFile != nil {
		defer closeFile()
	}
	data, err := io.ReadAll(io.LimitReader(reader, canonical.MaximumBytes+1))
	if err != nil {
		return nil, fmt.Errorf("read canonical input: %w", err)
	}
	if len(data) > canonical.MaximumBytes {
		return nil, fmt.Errorf("canonical input exceeds %d bytes", canonical.MaximumBytes)
	}
	return data, nil
}
