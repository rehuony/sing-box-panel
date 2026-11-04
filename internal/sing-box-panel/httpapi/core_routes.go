// SPDX-License-Identifier: GPL-3.0-or-later

package httpapi

import (
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/rehuony/sing-box-panel/internal/sing-box-panel/corecache"
	"github.com/rehuony/sing-box-panel/internal/sing-box-panel/store"
)

func (handler *Handler) listCatalogAssets(w http.ResponseWriter, request *http.Request) {
	if !handler.requireCommands(w, request) {
		return
	}
	query, ok := strictCoreQuery(w, request, "exact_version", "architecture", "variant")
	if !ok {
		return
	}
	if !validOptionalExactVersion(query.Get("exact_version"), true) ||
		!validOptionalArchitecture(query.Get("architecture")) ||
		!validOptionalVariant(query.Get("variant")) {
		writeProblem(w, request, http.StatusBadRequest, "catalog_filter_invalid", "Catalog filter invalid", "The catalog filter contains an unsupported value.")
		return
	}
	result, err := handler.services.Cores.ListCatalogAssets(request.Context(), corecache.CatalogAssetFilter{
		ExactVersion: query.Get("exact_version"),
		Architecture: query.Get("architecture"),
		Variant:      query.Get("variant"),
	})
	if err != nil {
		if corecache.IsCatalogNotInitialized(err) {
			writeProblem(w, request, http.StatusConflict, "catalog_not_initialized", "Catalog not initialized", "Refresh the core catalog before listing assets.")
			return
		}
		writeProblem(w, request, http.StatusInternalServerError, "catalog_asset_list_failed", "Catalog operation failed", "The core catalog assets could not be listed.")
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (handler *Handler) refreshCatalog(w http.ResponseWriter, request *http.Request) {
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
	result, err := handler.services.Cores.RefreshCatalog(
		request.Context(), corecache.CatalogRefreshOptions{Force: *input.Force},
	)
	if err != nil {
		var failure *corecache.Failure
		detail := "The catalog could not be refreshed."
		if errors.As(err, &failure) {
			detail = failure.Error()
		}
		writeProblem(w, request, http.StatusBadGateway, "catalog_refresh_failed", "Catalog refresh failed", detail)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"refreshed_at": result.RefreshedAt, "not_modified": result.NotModified,
		"releases": len(result.Catalog.Releases), "assets": len(result.Catalog.Assets()),
	})
}

func (handler *Handler) listCoreArtifacts(w http.ResponseWriter, request *http.Request) {
	if !handler.requireCommands(w, request) {
		return
	}
	query, ok := strictCoreQuery(
		w, request,
		"exact_version", "architecture", "variant", "source_kind",
		"before_time", "before_id", "limit",
	)
	if !ok {
		return
	}
	if !validOptionalExactVersion(query.Get("exact_version"), true) ||
		!validOptionalArchitecture(query.Get("architecture")) ||
		!validOptionalVariant(query.Get("variant")) ||
		!validOptionalCoreArtifactSource(query.Get("source_kind")) {
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
	page, err := handler.services.Fleet.ListCores(request.Context(), requestNodeID(request), corecache.CoreArtifactListFilter{
		ExactVersion: query.Get("exact_version"),
		Architecture: query.Get("architecture"),
		Variant:      query.Get("variant"),
		SourceKind:   query.Get("source_kind"),
		Cursor:       cursor,
		Limit:        limit,
	})
	if err != nil {
		writeNodeProblem(w, request, "core_artifact_list_failed", err)
		return
	}
	writeJSON(w, http.StatusOK, page)
}

func coreArtifactCursor(
	w http.ResponseWriter,
	request *http.Request,
	rawTime string,
	identifier string,
) (*corecache.CoreArtifactCursor, bool) {
	if rawTime == "" && identifier == "" {
		return nil, true
	}
	if rawTime == "" || identifier == "" || !store.ValidIdentifier(identifier) {
		writeProblem(w, request, http.StatusBadRequest, "query_invalid", "Query invalid", "before_time and before_id must be supplied together.")
		return nil, false
	}
	createdAt, err := time.Parse(time.RFC3339Nano, strings.TrimSpace(rawTime))
	if err != nil {
		writeProblem(w, request, http.StatusBadRequest, "query_invalid", "Query invalid", "before_time must be an RFC 3339 timestamp.")
		return nil, false
	}
	return &corecache.CoreArtifactCursor{CreatedAt: createdAt.UTC(), ID: identifier}, true
}

func (handler *Handler) getCoreArtifact(w http.ResponseWriter, request *http.Request, identifier string) {
	if !handler.requireCommands(w, request) {
		return
	}
	if _, ok := strictCoreQuery(w, request); !ok {
		return
	}
	artifact, err := handler.services.Fleet.Core(request.Context(), requestNodeID(request), identifier)
	if err != nil {
		writeNodeProblem(w, request, "core_artifact_read_failed", err)
		return
	}
	writeJSON(w, http.StatusOK, artifact)
}

func (handler *Handler) deleteCoreArtifact(w http.ResponseWriter, request *http.Request, identifier string) {
	if !handler.requireCommands(w, request) {
		return
	}
	if _, ok := strictCoreQuery(w, request); !ok || !requireEmptyCoreBody(w, request) {
		return
	}
	if err := handler.services.Fleet.RemoveCore(request.Context(), requestNodeID(request), identifier); err != nil {
		writeNodeProblem(w, request, "core_artifact_delete_failed", err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusNoContent)
}

func (handler *Handler) installCore(w http.ResponseWriter, request *http.Request) {
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
	assets, err := handler.services.Cores.ListCatalogAssets(request.Context(), corecache.CatalogAssetFilter{})
	if err != nil {
		if corecache.IsCatalogNotInitialized(err) {
			writeProblem(w, request, http.StatusConflict, "catalog_not_initialized", "Catalog not initialized", "Refresh the core catalog before installing an asset.")
			return
		}
		writeProblem(w, request, http.StatusInternalServerError, "core_install_failed", "Core install failed", "The catalog could not be inspected before installation.")
		return
	}
	found := false
	for _, asset := range assets.Assets {
		if asset.AssetID == *input.AssetID {
			found = true
			break
		}
	}
	if !found {
		writeProblem(w, request, http.StatusUnprocessableEntity, "core_install_invalid", "Core install request invalid", "The catalog asset cannot be installed.")
		return
	}
	cached, err := handler.services.Cores.CacheOfficial(request.Context(), *input.AssetID)
	if err != nil {
		writeProblem(w, request, http.StatusBadGateway, "core_download_failed", "Core download failed", "The panel could not download and verify the release asset.")
		return
	}
	result, err := handler.services.Fleet.InstallCore(request.Context(), requestNodeID(request), cached)
	if err != nil {
		writeNodeProblem(w, request, "core_install_failed", err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (handler *Handler) importCore(w http.ResponseWriter, request *http.Request) {
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
	fields := make(map[string]string, 4)
	var stagedPath string
	defer func() {
		if stagedPath != "" {
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
			stagedPath, err = handler.stageCoreUpload(part)
			_ = part.Close()
			if err != nil {
				writeProblem(w, request, http.StatusUnprocessableEntity, "core_import_invalid", "Core import request invalid", "The uploaded archive is empty, too large, or could not be staged privately.")
				return
			}
			continue
		}
		if name != "source_description" && name != "exact_version" && name != "architecture" && name != "variant" {
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
	upload := corecache.CoreUpload{
		SourceDescription: fields["source_description"], ExactVersion: fields["exact_version"],
		Architecture: fields["architecture"], Variant: fields["variant"],
	}
	if stagedPath == "" || !validCoreUpload(upload) {
		writeProblem(w, request, http.StatusUnprocessableEntity, "core_import_invalid", "Core import request invalid", "The local core archive cannot be imported with the supplied metadata.")
		return
	}
	// CacheUpload consumes the staged file.
	path := stagedPath
	stagedPath = ""
	cached, err := handler.services.Cores.CacheUpload(request.Context(), path, upload)
	if err != nil {
		writeProblem(w, request, http.StatusInternalServerError, "core_import_failed", "Core import failed", "The core archive could not be cached.")
		return
	}
	result, err := handler.services.Fleet.InstallCore(request.Context(), requestNodeID(request), cached)
	if err != nil {
		writeNodeProblem(w, request, "core_import_failed", err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

// stageCoreUpload writes an uploaded archive to the panel's private upload
// staging directory, where the core cache accepts it from.
func (handler *Handler) stageCoreUpload(source io.Reader) (string, error) {
	directory := filepath.Join(handler.settings.DataDir, "imports")
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return "", err
	}
	if err := os.Chmod(directory, 0o700); err != nil {
		return "", err
	}
	info, err := os.Lstat(directory)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 || info.Mode().Perm() != 0o700 {
		return "", errors.New("private import directory is unsafe")
	}
	file, err := os.CreateTemp(directory, "core-upload-*")
	if err != nil {
		return "", err
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
		return "", err
	}
	written, err := io.Copy(file, io.LimitReader(source, maximumCoreUploadBytes+1))
	if err != nil || written < 1 || written > maximumCoreUploadBytes {
		return "", errors.New("uploaded core archive size is invalid")
	}
	if err := file.Sync(); err != nil {
		return "", err
	}
	if err := file.Close(); err != nil {
		return "", err
	}
	keep = true
	return path, nil
}
