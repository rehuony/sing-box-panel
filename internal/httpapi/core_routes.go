// SPDX-License-Identifier: GPL-3.0-or-later

package httpapi

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/rehuony/sing-box-panel/internal/application"
	"github.com/rehuony/sing-box-panel/internal/store"
)

func (handler *Handler) listCatalogAssets(w http.ResponseWriter, request *http.Request) {
	if !handler.requireCommands(w, request) {
		return
	}
	query, ok := strictCoreQuery(w, request, "exact_version", "architecture", "variant", "installable")
	if !ok {
		return
	}
	if !validOptionalExactVersion(query.Get("exact_version"), true) ||
		!validOptionalArchitecture(query.Get("architecture")) ||
		!validOptionalVariant(query.Get("variant")) {
		writeProblem(w, request, http.StatusBadRequest, "catalog_filter_invalid", "Catalog filter invalid", "The catalog filter contains an unsupported value.")
		return
	}
	installable, ok := optionalStrictBool(w, request, query, "installable")
	if !ok {
		return
	}
	result, err := handler.commands.ListCatalogAssets(request.Context(), application.CatalogAssetFilter{
		ExactVersion: query.Get("exact_version"),
		Architecture: query.Get("architecture"),
		Variant:      query.Get("variant"),
		Installable:  installable,
	})
	if err != nil {
		if application.IsCatalogNotInitialized(err) {
			writeProblem(w, request, http.StatusConflict, "catalog_not_initialized", "Catalog not initialized", "Refresh the core catalog before listing assets.")
			return
		}
		writeProblem(w, request, http.StatusInternalServerError, "catalog_asset_list_failed", "Catalog operation failed", "The core catalog assets could not be listed.")
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (handler *Handler) queueCatalogRefresh(w http.ResponseWriter, request *http.Request) {
	if !handler.requireCommands(w, request) {
		return
	}
	if _, ok := strictCoreQuery(w, request); !ok {
		return
	}
	var input struct {
		Force *bool `json:"force"`
	}
	if !decodeStrictRequest(w, request, maximumCoreEmptyRequestBytes, &input) {
		return
	}
	if input.Force == nil {
		writeProblem(w, request, http.StatusUnprocessableEntity, "catalog_refresh_invalid", "Catalog refresh request is invalid", "The force field is required.")
		return
	}
	task, err := handler.commands.QueueCatalogRefresh(
		request.Context(), application.CatalogRefreshOptions{Force: *input.Force},
	)
	if err != nil {
		writeProblem(w, request, http.StatusInternalServerError, "catalog_refresh_failed", "Catalog refresh failed", "The catalog refresh task could not be queued.")
		return
	}
	writeJSON(w, http.StatusAccepted, task)
}

func (handler *Handler) listCoreArtifacts(w http.ResponseWriter, request *http.Request) {
	if !handler.requireCommands(w, request) {
		return
	}
	query, ok := strictCoreQuery(
		w, request,
		"exact_version", "architecture", "variant", "source_kind", "verification_state",
		"before_time", "before_id", "limit",
	)
	if !ok {
		return
	}
	if !validOptionalExactVersion(query.Get("exact_version"), true) ||
		!validOptionalArchitecture(query.Get("architecture")) ||
		!validOptionalVariant(query.Get("variant")) ||
		!validOptionalCoreArtifactSource(query.Get("source_kind")) ||
		!validOptionalCoreArtifactVerification(query.Get("verification_state")) {
		writeProblem(w, request, http.StatusBadRequest, "core_artifact_filter_invalid", "Core artifact filter invalid", "The core artifact filter contains an unsupported value.")
		return
	}
	limit, ok := optionalLimit(w, request)
	if !ok {
		return
	}
	cursor, ok := coreArtifactCursor(w, request, query.Get("before_time"), query.Get("before_id"))
	if !ok {
		return
	}
	page, err := handler.commands.ListCoreArtifacts(request.Context(), application.CoreArtifactListFilter{
		ExactVersion:      query.Get("exact_version"),
		Architecture:      query.Get("architecture"),
		Variant:           query.Get("variant"),
		SourceKind:        store.CoreArtifactSourceKind(query.Get("source_kind")),
		VerificationState: store.CoreArtifactVerificationState(query.Get("verification_state")),
		Cursor:            cursor,
		Limit:             limit,
	})
	if err != nil {
		writeProblem(w, request, http.StatusInternalServerError, "core_artifact_list_failed", "Core artifact operation failed", "The core artifacts could not be listed.")
		return
	}
	writeJSON(w, http.StatusOK, page)
}

func coreArtifactCursor(
	w http.ResponseWriter,
	request *http.Request,
	rawTime string,
	identifier string,
) (*application.CoreArtifactCursor, bool) {
	if rawTime == "" && identifier == "" {
		return nil, true
	}
	if rawTime == "" || identifier == "" || !validStableIdentifier(identifier) {
		writeProblem(w, request, http.StatusBadRequest, "query_invalid", "Query invalid", "before_time and before_id must be supplied together.")
		return nil, false
	}
	createdAt, err := time.Parse(time.RFC3339Nano, strings.TrimSpace(rawTime))
	if err != nil {
		writeProblem(w, request, http.StatusBadRequest, "query_invalid", "Query invalid", "before_time must be an RFC 3339 timestamp.")
		return nil, false
	}
	return &application.CoreArtifactCursor{CreatedAt: createdAt.UTC(), ID: identifier}, true
}

func (handler *Handler) getCoreArtifact(w http.ResponseWriter, request *http.Request, identifier string) {
	if !handler.requireCommands(w, request) {
		return
	}
	if !validCoreArtifactID(identifier) {
		writeProblem(w, request, http.StatusBadRequest, "core_artifact_id_invalid", "Core artifact ID invalid", "The core artifact ID is invalid.")
		return
	}
	if _, ok := strictCoreQuery(w, request); !ok {
		return
	}
	artifact, err := handler.commands.CoreArtifact(request.Context(), identifier)
	if err != nil {
		if application.IsCoreArtifactNotFound(err) {
			writeProblem(w, request, http.StatusNotFound, "core_artifact_not_found", "Core artifact not found", "The requested core artifact does not exist.")
			return
		}
		writeProblem(w, request, http.StatusInternalServerError, "core_artifact_read_failed", "Core artifact operation failed", "The core artifact could not be read.")
		return
	}
	writeJSON(w, http.StatusOK, artifact)
}

func (handler *Handler) deleteCoreArtifact(w http.ResponseWriter, request *http.Request, identifier string) {
	if !handler.requireCommands(w, request) {
		return
	}
	if !validCoreArtifactID(identifier) {
		writeProblem(w, request, http.StatusBadRequest, "core_artifact_id_invalid", "Core artifact ID invalid", "The core artifact ID is invalid.")
		return
	}
	if _, ok := strictCoreQuery(w, request); !ok || !requireEmptyCoreBody(w, request) {
		return
	}
	if err := handler.commands.RemoveCoreArtifact(request.Context(), identifier); err != nil {
		switch {
		case application.IsCoreArtifactNotFound(err):
			writeProblem(w, request, http.StatusNotFound, "core_artifact_not_found", "Core artifact not found", "The requested core artifact does not exist.")
		case application.IsCoreArtifactInUse(err):
			writeProblem(w, request, http.StatusConflict, "core_artifact_in_use", "Core artifact is in use", "The core artifact is referenced and cannot be removed.")
		default:
			writeProblem(w, request, http.StatusInternalServerError, "core_artifact_delete_failed", "Core artifact operation failed", "The core artifact could not be removed.")
		}
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusNoContent)
}

func (handler *Handler) restrictCoreArtifact(
	w http.ResponseWriter,
	request *http.Request,
	identifier string,
	verificationState store.CoreArtifactVerificationState,
) {
	if !handler.requireCommands(w, request) {
		return
	}
	if !validCoreArtifactID(identifier) {
		writeProblem(w, request, http.StatusBadRequest, "core_artifact_id_invalid", "Core artifact ID invalid", "The core artifact ID is invalid.")
		return
	}
	if _, ok := strictCoreQuery(w, request); !ok || !requireEmptyCoreBody(w, request) {
		return
	}
	artifact, err := handler.commands.RestrictCoreArtifactVerification(
		request.Context(),
		identifier,
		verificationState,
	)
	if err != nil {
		if application.IsCoreArtifactNotFound(err) {
			writeProblem(w, request, http.StatusNotFound, "core_artifact_not_found", "Core artifact not found", "The requested core artifact does not exist.")
			return
		}
		writeProblem(w, request, http.StatusInternalServerError, "core_artifact_verification_update_failed", "Core artifact operation failed", "The core artifact verification state could not be restricted.")
		return
	}
	writeJSON(w, http.StatusOK, artifact)
}

func (handler *Handler) queueCoreInstall(w http.ResponseWriter, request *http.Request) {
	if !handler.requireCommands(w, request) {
		return
	}
	if _, ok := strictCoreQuery(w, request); !ok {
		return
	}
	var input struct {
		AssetID *int64 `json:"asset_id"`
	}
	if !decodeStrictRequest(w, request, maximumCoreInstallRequestBytes, &input) {
		return
	}
	if input.AssetID == nil || *input.AssetID < 1 {
		writeProblem(w, request, http.StatusUnprocessableEntity, "core_install_invalid", "Core install request invalid", "asset_id must be a positive integer.")
		return
	}
	assets, err := handler.commands.ListCatalogAssets(request.Context(), application.CatalogAssetFilter{Installable: true})
	if err != nil {
		if application.IsCatalogNotInitialized(err) {
			writeProblem(w, request, http.StatusConflict, "catalog_not_initialized", "Catalog not initialized", "Refresh the core catalog before installing an asset.")
			return
		}
		writeProblem(w, request, http.StatusInternalServerError, "core_install_failed", "Core install failed", "The catalog could not be inspected before queuing the installation.")
		return
	}
	installable := false
	for _, asset := range assets.Assets {
		if asset.AssetID == *input.AssetID {
			installable = true
			break
		}
	}
	if !installable {
		writeProblem(w, request, http.StatusUnprocessableEntity, "core_install_invalid", "Core install request invalid", "The catalog asset cannot be installed.")
		return
	}
	task, err := handler.commands.QueueCoreInstall(request.Context(), *input.AssetID)
	if err != nil {
		writeProblem(w, request, http.StatusInternalServerError, "core_install_failed", "Core install failed", "The core installation task could not be queued.")
		return
	}
	writeJSON(w, http.StatusAccepted, task)
}

func (handler *Handler) queueCoreImport(w http.ResponseWriter, request *http.Request) {
	if !handler.requireCommands(w, request) {
		return
	}
	if _, ok := strictCoreQuery(w, request); !ok {
		return
	}
	request.Body = http.MaxBytesReader(w, request.Body, maximumCoreUploadBytes+(64<<10))
	reader, err := request.MultipartReader()
	if err != nil {
		writeProblem(w, request, http.StatusUnsupportedMediaType, "core_import_media_type", "Core import media type invalid", "Use multipart/form-data with one archive file.")
		return
	}
	fields := make(map[string]string, 5)
	var stagedPath, actualDigest string
	queued := false
	defer func() {
		if !queued && stagedPath != "" {
			_ = os.Remove(stagedPath)
		}
	}()
	for {
		part, nextErr := reader.NextPart()
		if errors.Is(nextErr, io.EOF) {
			break
		}
		if nextErr != nil {
			writeProblem(w, request, http.StatusUnprocessableEntity, "core_import_invalid", "Core import request invalid", "The multipart upload is malformed or exceeds its size limit.")
			return
		}
		name := part.FormName()
		if name == "archive" {
			if stagedPath != "" || part.FileName() == "" {
				_ = part.Close()
				writeProblem(w, request, http.StatusUnprocessableEntity, "core_import_invalid", "Core import request invalid", "Exactly one archive file is required.")
				return
			}
			stagedPath, actualDigest, err = handler.stageCoreUpload(part)
			_ = part.Close()
			if err != nil {
				writeProblem(w, request, http.StatusUnprocessableEntity, "core_import_invalid", "Core import request invalid", "The uploaded archive is empty, too large, or could not be staged privately.")
				return
			}
			continue
		}
		if name != "source_description" && name != "sha256" && name != "exact_version" && name != "architecture" && name != "variant" {
			_ = part.Close()
			writeProblem(w, request, http.StatusUnprocessableEntity, "core_import_invalid", "Core import request invalid", "The multipart upload contains an unknown field.")
			return
		}
		if _, duplicate := fields[name]; duplicate {
			_ = part.Close()
			writeProblem(w, request, http.StatusUnprocessableEntity, "core_import_invalid", "Core import request invalid", "The multipart upload contains a duplicate field.")
			return
		}
		value, readErr := io.ReadAll(io.LimitReader(part, 4097))
		_ = part.Close()
		if readErr != nil || len(value) > 4096 {
			writeProblem(w, request, http.StatusUnprocessableEntity, "core_import_invalid", "Core import request invalid", "A multipart metadata field exceeds its size limit.")
			return
		}
		fields[name] = string(value)
	}
	importRequest := application.CoreImportRequest{
		SourcePath: stagedPath, SourceDescription: fields["source_description"],
		SHA256: fields["sha256"], ExactVersion: fields["exact_version"],
		Architecture: fields["architecture"], Variant: fields["variant"], DeleteSource: true,
	}
	if stagedPath == "" || actualDigest != strings.ToLower(importRequest.SHA256) || !validCoreImportRequest(importRequest) {
		writeProblem(w, request, http.StatusUnprocessableEntity, "core_import_invalid", "Core import request invalid", "The local core archive cannot be imported with the supplied metadata.")
		return
	}
	task, err := handler.commands.QueueCoreImport(request.Context(), importRequest)
	if err != nil {
		writeProblem(w, request, http.StatusInternalServerError, "core_import_failed", "Core import failed", "The core import task could not be queued.")
		return
	}
	queued = true
	writeJSON(w, http.StatusAccepted, task)
}

func (handler *Handler) stageCoreUpload(source io.Reader) (string, string, error) {
	directory := filepath.Join(handler.settings.DataDir, "imports")
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return "", "", err
	}
	if err := os.Chmod(directory, 0o700); err != nil {
		return "", "", err
	}
	info, err := os.Lstat(directory)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 || info.Mode().Perm() != 0o700 {
		return "", "", errors.New("private import directory is unsafe")
	}
	file, err := os.CreateTemp(directory, "core-upload-*")
	if err != nil {
		return "", "", err
	}
	path := file.Name()
	keep := false
	defer func() {
		_ = file.Close()
		if !keep {
			_ = os.Remove(path)
		}
	}()
	if err := file.Chmod(0o600); err != nil {
		return "", "", err
	}
	hash := sha256.New()
	written, err := io.Copy(io.MultiWriter(file, hash), io.LimitReader(source, maximumCoreUploadBytes+1))
	if err != nil || written < 1 || written > maximumCoreUploadBytes {
		return "", "", errors.New("uploaded core archive size is invalid")
	}
	if err := file.Sync(); err != nil {
		return "", "", err
	}
	if err := file.Close(); err != nil {
		return "", "", err
	}
	keep = true
	return path, hex.EncodeToString(hash.Sum(nil)), nil
}
