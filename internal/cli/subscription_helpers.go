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

func (input subscriptionChannelWriteInput) createRequest() (application.CreateSubscriptionChannelRequest, error) {
	if input.Name == nil || input.Format == nil || input.PublicHost == nil || input.Enabled == nil {
		return application.CreateSubscriptionChannelRequest{}, errors.New("name, format, public_host, and enabled are required")
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
		Name: *input.Name, Format: *input.Format, PublicHost: *input.PublicHost,
		Config: append(json.RawMessage(nil), input.Config...), Enabled: *input.Enabled,
	}, nil
}

func (input subscriptionChannelWriteInput) updateRequest(expected time.Time) (application.UpdateSubscriptionChannelRequest, error) {
	created, err := input.createRequest()
	if err != nil {
		return application.UpdateSubscriptionChannelRequest{}, err
	}
	return application.UpdateSubscriptionChannelRequest{
		Name: created.Name, Format: created.Format, PublicHost: created.PublicHost, Config: created.Config,
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
	return application.CreateSubscriptionSourceRequest{
		Name: *input.Name, SourceKind: *input.SourceKind,
		Config: append(json.RawMessage(nil), input.Config...), Enabled: *input.Enabled,
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
	case errors.Is(err, store.ErrSubscriptionLimitExceeded):
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
	output.WriteString("ID\tNAME\tKIND\tENABLED\tVERSION\tUPDATED_AT\n")
	for _, value := range values {
		fmt.Fprintf(&output, "%s\t%s\t%s\t%t\t%t\t%s\n", value.ID, value.Name, value.SourceKind, value.Enabled, value.HasVersion, formatSubscriptionTime(value.UpdatedAt))
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
