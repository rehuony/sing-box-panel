// SPDX-License-Identifier: GPL-3.0-or-later

package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/rehuony/sing-box-panel/internal/application"
	"github.com/rehuony/sing-box-panel/internal/canonical"
	"github.com/rehuony/sing-box-panel/internal/jsonstrict"
	"github.com/spf13/cobra"
)

func newEntityCommand(
	name string,
	short string,
	state *options,
	open openApplicationFunc,
) *cobra.Command {
	collection := canonical.CollectionNodes
	if name == "rule" {
		collection = canonical.CollectionRules
	}
	root := group(name, short)
	root.AddCommand(
		newEntityListCommand(collection, state, open),
		newEntityShowCommand(collection, state, open),
		newEntityCreateCommand(collection, state, open),
		newEntityUpdateCommand(collection, state, open),
		newEntityDeleteCommand(collection, state, open),
		newEntityEnabledCommand(collection, true, state, open),
		newEntityEnabledCommand(collection, false, state, open),
		newEntityMoveCommand(collection, state, open),
	)
	return root
}

func newEntityListCommand(collection canonical.Collection, state *options, open openApplicationFunc) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List canonical " + string(collection),
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			instance, err := openApplication(cmd.Context(), state.settingsPath, open)
			if err != nil {
				return err
			}
			defer instance.Close()
			result, err := instance.ListEntities(cmd.Context(), collection)
			if err != nil {
				return classifyCanonicalError("entity_list_failed", err)
			}
			return writeResult(cmd.OutOrStdout(), state.format, result, entityListText(result))
		},
	}
}

func newEntityShowCommand(collection canonical.Collection, state *options, open openApplicationFunc) *cobra.Command {
	return &cobra.Command{
		Use:   "show ID",
		Short: "Show one canonical " + singularCollection(collection),
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			instance, err := openApplication(cmd.Context(), state.settingsPath, open)
			if err != nil {
				return err
			}
			defer instance.Close()
			revision, entity, err := instance.GetEntity(cmd.Context(), collection, args[0])
			if err != nil {
				return classifyCanonicalError("entity_read_failed", err)
			}
			result := struct {
				Revision application.CanonicalSnapshot `json:"revision"`
				Entity   map[string]any                `json:"entity"`
			}{Revision: revision, Entity: entity}
			return writeResult(cmd.OutOrStdout(), state.format, result, prettyJSON(entity))
		},
	}
}

func newEntityCreateCommand(collection canonical.Collection, state *options, open openApplicationFunc) *cobra.Command {
	var filePath, baseRevision string
	command := &cobra.Command{
		Use:   "create",
		Short: "Create one canonical " + singularCollection(collection),
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			expectedHead, err := requiredBaseRevision(cmd, baseRevision)
			if err != nil {
				return err
			}
			entity, err := readEntityInput(cmd.InOrStdin(), filePath)
			if err != nil {
				return &Error{Kind: ErrorValidation, Code: "entity_input_failed", Message: err.Error(), Cause: err}
			}
			instance, err := openApplication(cmd.Context(), state.settingsPath, open)
			if err != nil {
				return err
			}
			defer instance.Close()
			result, err := instance.CreateEntity(cmd.Context(), expectedHead, collection, entity)
			if err != nil {
				return classifyCanonicalError("entity_create_failed", err)
			}
			return writeCanonicalSave(cmd, state, result)
		},
	}
	addEntityWriteFlags(command, &filePath, &baseRevision)
	return command
}

func newEntityUpdateCommand(collection canonical.Collection, state *options, open openApplicationFunc) *cobra.Command {
	var filePath, baseRevision string
	command := &cobra.Command{
		Use:   "update ID",
		Short: "Replace one canonical " + singularCollection(collection),
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			expectedHead, err := requiredBaseRevision(cmd, baseRevision)
			if err != nil {
				return err
			}
			entity, err := readEntityInput(cmd.InOrStdin(), filePath)
			if err != nil {
				return &Error{Kind: ErrorValidation, Code: "entity_input_failed", Message: err.Error(), Cause: err}
			}
			instance, err := openApplication(cmd.Context(), state.settingsPath, open)
			if err != nil {
				return err
			}
			defer instance.Close()
			result, err := instance.ReplaceEntity(cmd.Context(), expectedHead, collection, args[0], entity)
			if err != nil {
				return classifyCanonicalError("entity_update_failed", err)
			}
			return writeCanonicalSave(cmd, state, result)
		},
	}
	addEntityWriteFlags(command, &filePath, &baseRevision)
	return command
}

func newEntityDeleteCommand(collection canonical.Collection, state *options, open openApplicationFunc) *cobra.Command {
	var baseRevision string
	command := &cobra.Command{
		Use:   "delete ID",
		Short: "Delete one canonical " + singularCollection(collection),
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			expectedHead, err := requiredBaseRevision(cmd, baseRevision)
			if err != nil {
				return err
			}
			instance, err := openApplication(cmd.Context(), state.settingsPath, open)
			if err != nil {
				return err
			}
			defer instance.Close()
			result, err := instance.DeleteEntity(cmd.Context(), expectedHead, collection, args[0])
			if err != nil {
				return classifyCanonicalError("entity_delete_failed", err)
			}
			return writeCanonicalSave(cmd, state, result)
		},
	}
	command.Flags().StringVar(&baseRevision, "base-revision", "", "revision ID used as the compare-and-swap base")
	return command
}

func newEntityEnabledCommand(
	collection canonical.Collection,
	enabled bool,
	state *options,
	open openApplicationFunc,
) *cobra.Command {
	name := "disable"
	if enabled {
		name = "enable"
	}
	var baseRevision string
	command := &cobra.Command{
		Use:   name + " ID",
		Short: strings.ToUpper(name[:1]) + name[1:] + " one canonical " + singularCollection(collection),
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			expectedHead, err := requiredBaseRevision(cmd, baseRevision)
			if err != nil {
				return err
			}
			instance, err := openApplication(cmd.Context(), state.settingsPath, open)
			if err != nil {
				return err
			}
			defer instance.Close()
			result, err := instance.SetEntityEnabled(cmd.Context(), expectedHead, collection, args[0], enabled)
			if err != nil {
				return classifyCanonicalError("entity_update_failed", err)
			}
			return writeCanonicalSave(cmd, state, result)
		},
	}
	command.Flags().StringVar(&baseRevision, "base-revision", "", "revision ID used as the compare-and-swap base")
	return command
}

func newEntityMoveCommand(collection canonical.Collection, state *options, open openApplicationFunc) *cobra.Command {
	var baseRevision, beforeID string
	command := &cobra.Command{
		Use:   "move ID",
		Short: "Move one canonical " + singularCollection(collection) + " before another, or to the end",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			expectedHead, err := requiredBaseRevision(cmd, baseRevision)
			if err != nil {
				return err
			}
			instance, err := openApplication(cmd.Context(), state.settingsPath, open)
			if err != nil {
				return err
			}
			defer instance.Close()
			result, err := instance.MoveEntity(cmd.Context(), expectedHead, collection, args[0], beforeID)
			if err != nil {
				return classifyCanonicalError("entity_move_failed", err)
			}
			return writeCanonicalSave(cmd, state, result)
		},
	}
	command.Flags().StringVar(&beforeID, "before", "", "place before this ID; omit to move to the end")
	command.Flags().StringVar(&baseRevision, "base-revision", "", "revision ID used as the compare-and-swap base")
	return command
}

func addEntityWriteFlags(command *cobra.Command, filePath, baseRevision *string) {
	command.Flags().StringVar(filePath, "file", "", "entity JSON file, or - for stdin")
	command.Flags().StringVar(baseRevision, "base-revision", "", "revision ID used as the compare-and-swap base")
}

func requiredBaseRevision(command *cobra.Command, value string) (string, error) {
	if !command.Flags().Changed("base-revision") || strings.TrimSpace(value) == "" {
		return "", &Error{Kind: ErrorUsage, Code: "base_revision_required", Message: "--base-revision requires a revision ID"}
	}
	return strings.TrimSpace(value), nil
}

func readEntityInput(stdin io.Reader, filePath string) (map[string]any, error) {
	if filePath == "" {
		return nil, errors.New("--file is required; use - for stdin")
	}
	data, err := readInputFile(stdin, filePath, canonical.MaximumBytes, "entity")
	if err != nil {
		return nil, err
	}
	var entity map[string]any
	if err := jsonstrict.Decode(data, canonical.MaximumBytes, &entity); err != nil {
		return nil, fmt.Errorf("decode entity JSON: %w", err)
	}
	if entity == nil {
		return nil, errors.New("entity must be a JSON object")
	}
	return entity, nil
}

func readInputFile(stdin io.Reader, filePath string, maximum int64, label string) ([]byte, error) {
	var reader io.Reader
	var closeFile func() error
	if filePath == "-" {
		reader = stdin
	} else {
		file, err := os.Open(filePath)
		if err != nil {
			return nil, fmt.Errorf("open %s input: %w", label, err)
		}
		reader = file
		closeFile = file.Close
	}
	if closeFile != nil {
		defer closeFile()
	}
	data, err := io.ReadAll(io.LimitReader(reader, maximum+1))
	if err != nil {
		return nil, fmt.Errorf("read %s input: %w", label, err)
	}
	if int64(len(data)) > maximum {
		return nil, fmt.Errorf("%s input exceeds %d bytes", label, maximum)
	}
	return data, nil
}

func classifyCanonicalError(code string, err error) error {
	switch {
	case application.IsRevisionConflict(err):
		return &Error{Kind: ErrorConflict, Code: "canonical_revision_conflict", Message: err.Error(), Cause: err}
	case errors.Is(err, canonical.ErrEntityExists):
		return &Error{Kind: ErrorConflict, Code: "entity_exists", Message: err.Error(), Cause: err}
	case errors.Is(err, canonical.ErrEntityReferenced):
		return &Error{Kind: ErrorConflict, Code: "entity_referenced", Message: err.Error(), Cause: err}
	case errors.Is(err, canonical.ErrEntityNotFound):
		return &Error{Kind: ErrorDomain, Code: "entity_not_found", Message: err.Error(), Cause: err}
	case errors.Is(err, canonical.ErrInvalidDocument):
		return &Error{Kind: ErrorValidation, Code: "canonical_invalid", Message: err.Error(), Cause: err}
	default:
		return &Error{Kind: ErrorDomain, Code: code, Message: err.Error(), Cause: err}
	}
}

func writeCanonicalSave(command *cobra.Command, state *options, result application.CanonicalSave) error {
	text := fmt.Sprintf("saved canonical revision #%d %s", result.Revision.Sequence, result.Revision.ID)
	if result.NoChange {
		text = fmt.Sprintf("canonical revision #%d %s is unchanged", result.Revision.Sequence, result.Revision.ID)
	} else if result.TaskID != "" {
		text += " (task " + result.TaskID + ")"
	}
	return writeResult(command.OutOrStdout(), state.format, result, text)
}

func entityListText(result application.EntityList) string {
	var output strings.Builder
	fmt.Fprintf(&output, "REVISION\t#%d\t%s\n", result.Revision.Sequence, result.Revision.ID)
	for _, entity := range result.Entities {
		fmt.Fprintf(&output, "%v\t%v\t%v\n", entity["id"], entity["kind"], entity["enabled"])
	}
	return strings.TrimSuffix(output.String(), "\n")
}

func singularCollection(collection canonical.Collection) string {
	if collection == canonical.CollectionRules {
		return "rule"
	}
	return "node"
}

func prettyJSON(value any) string {
	var output bytes.Buffer
	encoder := json.NewEncoder(&output)
	encoder.SetEscapeHTML(false)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(value); err != nil {
		return fmt.Sprintf("%v", value)
	}
	return strings.TrimSuffix(output.String(), "\n")
}
