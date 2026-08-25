// SPDX-License-Identifier: GPL-3.0-or-later

package httpapi

import (
	"errors"
	"net/http"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/rehuony/sing-box-panel/internal/application"
	"github.com/rehuony/sing-box-panel/internal/canonical"
	"github.com/rehuony/sing-box-panel/internal/capability"
	"github.com/rehuony/sing-box-panel/internal/coreartifact"
	"github.com/rehuony/sing-box-panel/internal/jsonstrict"
	"github.com/rehuony/sing-box-panel/internal/manualjson"
	"github.com/rehuony/sing-box-panel/internal/runtimeidentity"
	"github.com/rehuony/sing-box-panel/internal/store"
)

const (
	maximumCoreInstallRequestBytes = 4 << 10
	maximumCoreImportRequestBytes  = 32 << 10
	maximumCoreEmptyRequestBytes   = 1 << 10
	maximumCapabilityRequestBytes  = 4 << 10
	maximumCoreQueryBytes          = 8 << 10
	maximumRuntimeRequestBytes     = 4 << 10
)

func (handler *Handler) handleApplicationRoute(w http.ResponseWriter, request *http.Request, path string) bool {
	var next http.HandlerFunc
	if path == "/api/v1/config/apply" {
		if request.Method == http.MethodPost {
			next = handler.queueCoreActivate
		} else {
			next = methodNotAllowed
		}
	} else if path == "/api/v1/config/render" {
		if request.Method == http.MethodPost {
			next = handler.renderStructuredConfiguration
		} else {
			next = methodNotAllowed
		}
	} else if path == "/api/v1/config/artifacts" {
		if request.Method == http.MethodGet {
			next = handler.listStartupArtifacts
		} else {
			next = methodNotAllowed
		}
	} else if path == "/api/v1/config/manual/preview" {
		if request.Method == http.MethodPost {
			next = handler.previewManualReplacement
		} else {
			next = methodNotAllowed
		}
	} else if identifier, operation, matched := matchManualReattachRoute(path); matched {
		switch {
		case operation == "preview" && request.Method == http.MethodGet:
			next = func(w http.ResponseWriter, request *http.Request) {
				handler.previewManualReattach(w, request, identifier)
			}
		case operation == "apply" && request.Method == http.MethodPost:
			next = func(w http.ResponseWriter, request *http.Request) {
				handler.applyManualReattach(w, request, identifier)
			}
		default:
			next = methodNotAllowed
		}
	} else if resource, identifier, matched := matchObservabilityRoute(path); matched {
		next = handler.observabilityHandler(request.Method, resource, identifier)
	} else if resource, identifier, matched := matchCoreRoute(path); matched {
		switch {
		case resource == "catalog-assets" && request.Method == http.MethodGet:
			next = handler.listCatalogAssets
		case resource == "catalog-refresh" && request.Method == http.MethodPost:
			next = handler.queueCatalogRefresh
		case resource == "artifacts" && identifier == "" && request.Method == http.MethodGet:
			next = handler.listCoreArtifacts
		case resource == "artifacts" && identifier != "" && request.Method == http.MethodGet:
			next = func(w http.ResponseWriter, request *http.Request) {
				handler.getCoreArtifact(w, request, identifier)
			}
		case resource == "artifacts" && identifier != "" && request.Method == http.MethodDelete:
			next = func(w http.ResponseWriter, request *http.Request) {
				handler.deleteCoreArtifact(w, request, identifier)
			}
		case resource == "artifact-quarantine" && request.Method == http.MethodPost:
			next = func(w http.ResponseWriter, request *http.Request) {
				handler.restrictCoreArtifact(w, request, identifier, store.CoreArtifactQuarantined)
			}
		case resource == "artifact-revoke" && request.Method == http.MethodPost:
			next = func(w http.ResponseWriter, request *http.Request) {
				handler.restrictCoreArtifact(w, request, identifier, store.CoreArtifactRevoked)
			}
		case resource == "install" && request.Method == http.MethodPost:
			next = handler.queueCoreInstall
		case resource == "import" && request.Method == http.MethodPost:
			next = handler.queueCoreImport
		case resource == "capability" && request.Method == http.MethodGet:
			next = handler.coreCapability
		case resource == "capability-generations" && request.Method == http.MethodGet:
			next = handler.listCapabilityGenerations
		case resource == "capability-refresh" && request.Method == http.MethodPost:
			next = handler.refreshCapabilityGeneration
		case resource == "capability-inspect" && request.Method == http.MethodGet:
			next = handler.inspectCapabilityCandidate
		case resource == "capability-upgrade" && request.Method == http.MethodPost:
			next = handler.upgradeCapabilityCandidate
		case resource == "capability-quarantine" && request.Method == http.MethodPost:
			next = func(w http.ResponseWriter, request *http.Request) {
				handler.quarantineCapabilityManifest(w, request, identifier)
			}
		case resource == "status" && request.Method == http.MethodGet:
			next = handler.coreRuntimeStatus
		case resource == "check" && request.Method == http.MethodPost:
			next = handler.queueStartupCheck
		case resource == "activate" && request.Method == http.MethodPost:
			next = handler.queueCoreActivate
		case resource == "start" && request.Method == http.MethodPost:
			next = func(w http.ResponseWriter, request *http.Request) {
				handler.queueRuntimeLifecycle(w, request, "start")
			}
		case resource == "stop" && request.Method == http.MethodPost:
			next = func(w http.ResponseWriter, request *http.Request) {
				handler.queueRuntimeLifecycle(w, request, "stop")
			}
		case resource == "restart" && request.Method == http.MethodPost:
			next = func(w http.ResponseWriter, request *http.Request) {
				handler.queueRuntimeLifecycle(w, request, "restart")
			}
		case resource == "rollback" && request.Method == http.MethodPost:
			next = func(w http.ResponseWriter, request *http.Request) {
				handler.queueRuntimeLifecycle(w, request, "rollback")
			}
		default:
			next = methodNotAllowed
		}
	} else if identifier, matched := matchManualRoute(path); matched {
		switch {
		case identifier == "" && request.Method == http.MethodGet:
			next = handler.listManualArtifacts
		case identifier == "" && request.Method == http.MethodPut:
			next = handler.replaceManualArtifact
		case identifier != invalidManualRoute && identifier != "" && request.Method == http.MethodGet:
			next = func(w http.ResponseWriter, request *http.Request) {
				handler.getManualArtifact(w, request, identifier)
			}
		case identifier != invalidManualRoute && identifier != "" && request.Method == http.MethodDelete:
			next = func(w http.ResponseWriter, request *http.Request) {
				handler.discardManualArtifact(w, request, identifier)
			}
		default:
			next = methodNotAllowed
		}
	} else if resource, identifier, operation, matched := matchSubscriptionRoute(path); matched {
		next = handler.subscriptionManagementHandler(request.Method, resource, identifier, operation)
	} else if collection, identifier, operation, matched := matchEntityRoute(path); matched {
		switch {
		case identifier == "" && operation == "" && request.Method == http.MethodGet:
			next = func(w http.ResponseWriter, request *http.Request) { handler.listEntities(w, request, collection) }
		case identifier == "" && operation == "" && request.Method == http.MethodPost:
			next = func(w http.ResponseWriter, request *http.Request) { handler.createEntity(w, request, collection) }
		case identifier != "" && operation == "" && request.Method == http.MethodGet:
			next = func(w http.ResponseWriter, request *http.Request) {
				handler.getEntity(w, request, collection, identifier)
			}
		case identifier != "" && operation == "" && request.Method == http.MethodPut:
			next = func(w http.ResponseWriter, request *http.Request) {
				handler.replaceEntity(w, request, collection, identifier)
			}
		case identifier != "" && operation == "" && request.Method == http.MethodDelete:
			next = func(w http.ResponseWriter, request *http.Request) {
				handler.deleteEntity(w, request, collection, identifier)
			}
		case identifier != "" && operation == "enabled" && request.Method == http.MethodPatch:
			next = func(w http.ResponseWriter, request *http.Request) {
				handler.setEntityEnabled(w, request, collection, identifier)
			}
		case identifier != "" && operation == "move" && request.Method == http.MethodPost:
			next = func(w http.ResponseWriter, request *http.Request) {
				handler.moveEntity(w, request, collection, identifier)
			}
		default:
			next = methodNotAllowed
		}
	} else if reference, operation, matched := matchRevisionRoute(path); matched {
		switch {
		case reference == "" && operation == "" && request.Method == http.MethodGet:
			next = handler.listRevisions
		case reference == "" && operation == "diff" && request.Method == http.MethodGet:
			next = handler.diffRevisions
		case reference != "" && operation == "" && request.Method == http.MethodGet:
			next = func(w http.ResponseWriter, request *http.Request) { handler.getRevision(w, request, reference) }
		case reference != "" && operation == "restore" && request.Method == http.MethodPost:
			next = func(w http.ResponseWriter, request *http.Request) { handler.restoreRevision(w, request, reference) }
		default:
			next = methodNotAllowed
		}
	} else if taskID, operation, matched := matchTaskRoute(path); matched {
		switch {
		case taskID == "" && operation == "" && request.Method == http.MethodGet:
			next = handler.listTasks
		case taskID != "" && operation == "" && request.Method == http.MethodGet:
			next = func(w http.ResponseWriter, request *http.Request) { handler.getTask(w, request, taskID) }
		case taskID != "" && operation == "cancel" && request.Method == http.MethodPost:
			next = func(w http.ResponseWriter, request *http.Request) { handler.cancelTask(w, request, taskID) }
		default:
			next = methodNotAllowed
		}
	}
	if next == nil {
		return false
	}
	handler.authenticated(next)(w, request)
	return true
}

func matchManualReattachRoute(path string) (identifier string, operation string, matched bool) {
	const prefix = "/api/v1/config/manual/"
	if !strings.HasPrefix(path, prefix) {
		return "", "", false
	}
	parts := strings.Split(strings.TrimPrefix(path, prefix), "/")
	switch {
	case len(parts) == 3 && parts[0] != "" && parts[1] == "reattach" && parts[2] == "preview":
		return parts[0], "preview", true
	case len(parts) == 2 && parts[0] != "" && parts[1] == "reattach":
		return parts[0], "apply", true
	case len(parts) > 1:
		return "", "invalid", true
	default:
		return "", "", false
	}
}

func matchCoreRoute(path string) (string, string, bool) {
	switch path {
	case "/api/v1/core/catalog/assets":
		return "catalog-assets", "", true
	case "/api/v1/core/catalog/refresh":
		return "catalog-refresh", "", true
	case "/api/v1/core/artifacts":
		return "artifacts", "", true
	case "/api/v1/core/install":
		return "install", "", true
	case "/api/v1/core/import":
		return "import", "", true
	case "/api/v1/core/capability":
		return "capability", "", true
	case "/api/v1/core/capability/generations":
		return "capability-generations", "", true
	case "/api/v1/core/capability/refresh":
		return "capability-refresh", "", true
	case "/api/v1/core/capability/inspect":
		return "capability-inspect", "", true
	case "/api/v1/core/capability/upgrade":
		return "capability-upgrade", "", true
	case "/api/v1/core/status":
		return "status", "", true
	case "/api/v1/core/check":
		return "check", "", true
	case "/api/v1/core/activate":
		return "activate", "", true
	case "/api/v1/core/start":
		return "start", "", true
	case "/api/v1/core/stop":
		return "stop", "", true
	case "/api/v1/core/restart":
		return "restart", "", true
	case "/api/v1/core/rollback":
		return "rollback", "", true
	}
	const capabilitiesPrefix = "/api/v1/core/capabilities/"
	if strings.HasPrefix(path, capabilitiesPrefix) {
		parts := strings.Split(strings.TrimPrefix(path, capabilitiesPrefix), "/")
		if len(parts) == 2 && parts[0] != "" && parts[1] == "quarantine" {
			return "capability-quarantine", parts[0], true
		}
		return "invalid", "", true
	}
	const artifactsPrefix = "/api/v1/core/artifacts/"
	if !strings.HasPrefix(path, artifactsPrefix) {
		return "", "", false
	}
	remainder := strings.TrimPrefix(path, artifactsPrefix)
	parts := strings.Split(remainder, "/")
	if len(parts) == 1 && parts[0] != "" {
		return "artifacts", parts[0], true
	}
	if len(parts) == 2 && parts[0] != "" {
		switch parts[1] {
		case "quarantine":
			return "artifact-quarantine", parts[0], true
		case "revoke":
			return "artifact-revoke", parts[0], true
		}
	}
	if remainder == "" || strings.Contains(remainder, "/") {
		return "invalid", "", true
	}
	return "invalid", "", true
}

const invalidManualRoute = "\x00"

func matchManualRoute(path string) (string, bool) {
	const prefix = "/api/v1/config/manual"
	if path == prefix {
		return "", true
	}
	if !strings.HasPrefix(path, prefix+"/") {
		return "", false
	}
	identifier := strings.TrimPrefix(path, prefix+"/")
	if identifier == "" || strings.Contains(identifier, "/") {
		return invalidManualRoute, true
	}
	return identifier, true
}

func matchEntityRoute(path string) (canonical.Collection, string, string, bool) {
	for _, candidate := range []struct {
		prefix     string
		collection canonical.Collection
	}{{"/api/v1/nodes", canonical.CollectionNodes}, {"/api/v1/rules", canonical.CollectionRules}} {
		if path == candidate.prefix {
			return candidate.collection, "", "", true
		}
		if !strings.HasPrefix(path, candidate.prefix+"/") {
			continue
		}
		parts := strings.Split(strings.TrimPrefix(path, candidate.prefix+"/"), "/")
		if len(parts) == 1 && parts[0] != "" {
			return candidate.collection, parts[0], "", true
		}
		if len(parts) == 2 && parts[0] != "" && (parts[1] == "enabled" || parts[1] == "move") {
			return candidate.collection, parts[0], parts[1], true
		}
		return candidate.collection, "", "invalid", true
	}
	return "", "", "", false
}

func matchRevisionRoute(path string) (string, string, bool) {
	const prefix = "/api/v1/config/revisions"
	if path == prefix {
		return "", "", true
	}
	if path == prefix+"/diff" {
		return "", "diff", true
	}
	if !strings.HasPrefix(path, prefix+"/") {
		return "", "", false
	}
	parts := strings.Split(strings.TrimPrefix(path, prefix+"/"), "/")
	if len(parts) == 1 && parts[0] != "" {
		return parts[0], "", true
	}
	if len(parts) == 2 && parts[0] != "" && parts[1] == "restore" {
		return parts[0], "restore", true
	}
	return "", "invalid", true
}

func matchTaskRoute(path string) (string, string, bool) {
	const prefix = "/api/v1/tasks"
	if path == prefix {
		return "", "", true
	}
	if !strings.HasPrefix(path, prefix+"/") {
		return "", "", false
	}
	parts := strings.Split(strings.TrimPrefix(path, prefix+"/"), "/")
	if len(parts) == 1 && parts[0] != "" {
		return parts[0], "", true
	}
	if len(parts) == 2 && parts[0] != "" && parts[1] == "cancel" {
		return parts[0], "cancel", true
	}
	return "", "invalid", true
}

func methodNotAllowed(w http.ResponseWriter, request *http.Request) {
	writeProblem(w, request, http.StatusMethodNotAllowed, "method_not_allowed", "Method not allowed", "The resource does not support this HTTP method.")
}

func (handler *Handler) requireCommands(w http.ResponseWriter, request *http.Request) bool {
	if handler.commands != nil {
		return true
	}
	writeProblem(w, request, http.StatusServiceUnavailable, "application_unavailable", "Application unavailable", "Configuration services are not ready.")
	return false
}

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
	if _, ok := strictCoreQuery(w, request); !ok || !requireEmptyCoreBody(w, request) {
		return
	}
	task, err := handler.commands.QueueCatalogRefresh(request.Context())
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
	var input struct {
		SourcePath        string `json:"source_path"`
		SourceDescription string `json:"source_description"`
		SHA256            string `json:"sha256"`
		ExactVersion      string `json:"exact_version"`
		Architecture      string `json:"architecture"`
		Variant           string `json:"variant"`
	}
	if !decodeStrictRequest(w, request, maximumCoreImportRequestBytes, &input) {
		return
	}
	importRequest := application.CoreImportRequest{
		SourcePath: input.SourcePath, SourceDescription: input.SourceDescription,
		SHA256: input.SHA256, ExactVersion: input.ExactVersion,
		Architecture: input.Architecture, Variant: input.Variant,
	}
	if !validCoreImportRequest(importRequest) {
		writeProblem(w, request, http.StatusUnprocessableEntity, "core_import_invalid", "Core import request invalid", "The local core archive cannot be imported with the supplied metadata.")
		return
	}
	task, err := handler.commands.QueueCoreImport(request.Context(), importRequest)
	if err != nil {
		writeProblem(w, request, http.StatusInternalServerError, "core_import_failed", "Core import failed", "The core import task could not be queued.")
		return
	}
	writeJSON(w, http.StatusAccepted, task)
}

func (handler *Handler) coreCapability(w http.ResponseWriter, request *http.Request) {
	if !handler.requireCommands(w, request) {
		return
	}
	query, ok := strictCoreQuery(w, request, "core_version")
	if !ok {
		return
	}
	explicitVersion := query.Get("core_version")
	if explicitVersion != "" && !validOptionalExactVersion(explicitVersion, true) {
		writeProblem(w, request, http.StatusBadRequest, "core_version_invalid", "Core version invalid", "core_version must be a non-zero exact stable semantic version.")
		return
	}
	// An empty explicitVersion is intentional: Application resolves and verifies
	// the actual running core instead of silently substituting a catalog version.
	status, err := handler.commands.CoreCapabilityStatus(request.Context(), explicitVersion)
	if err != nil {
		switch {
		case application.IsNoRunningCore(err):
			writeProblem(w, request, http.StatusConflict, "core_not_running", "Core is not running", "No verified running core is available for capability resolution.")
		case errors.Is(err, runtimeidentity.ErrStaleObservation), errors.Is(err, runtimeidentity.ErrInspectionUnavailable):
			writeProblem(w, request, http.StatusServiceUnavailable, "core_capability_unavailable", "Core capability unavailable", "The verified running core could not be inspected.")
		default:
			writeProblem(w, request, http.StatusInternalServerError, "core_capability_failed", "Core capability failed", "The core capability status could not be read.")
		}
		return
	}
	writeJSON(w, http.StatusOK, newCoreCapabilityResponse(status))
}

func (handler *Handler) listCapabilityGenerations(w http.ResponseWriter, request *http.Request) {
	if !handler.requireCommands(w, request) {
		return
	}
	if _, ok := strictCoreQuery(w, request, "limit"); !ok {
		return
	}
	limit, ok := optionalLimit(w, request)
	if !ok {
		return
	}
	result, err := handler.commands.ListCapabilityGenerations(request.Context(), limit)
	if err != nil {
		writeCapabilityProblem(w, request, "capability_generation_list_failed", err)
		return
	}
	writeJSON(w, http.StatusOK, struct {
		Items []application.CapabilityGenerationView `json:"items"`
	}{Items: result})
}

func (handler *Handler) refreshCapabilityGeneration(w http.ResponseWriter, request *http.Request) {
	if !handler.requireCommands(w, request) {
		return
	}
	if _, ok := strictCoreQuery(w, request); !ok {
		return
	}
	source, err := readBoundedBody(request, capability.MaximumGenerationBytes)
	if err != nil {
		writeProblem(w, request, http.StatusRequestEntityTooLarge, "capability_generation_too_large", "Capability generation rejected", err.Error())
		return
	}
	result, err := handler.commands.RefreshCapabilityGeneration(request.Context(), source)
	if err != nil {
		writeCapabilityProblem(w, request, "capability_refresh_failed", err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (handler *Handler) inspectCapabilityCandidate(w http.ResponseWriter, request *http.Request) {
	if !handler.requireCommands(w, request) {
		return
	}
	query, ok := strictCoreQuery(w, request, "core_version", "commit", "manifest_sha256")
	if !ok {
		return
	}
	reference := application.CapabilityUpgradeRequest{
		ExactCoreVersion: query.Get("core_version"),
		CommitSHA:        query.Get("commit"),
		ManifestSHA256:   query.Get("manifest_sha256"),
	}
	if !validCapabilityReference(reference) {
		writeProblem(w, request, http.StatusBadRequest, "capability_reference_invalid", "Capability reference invalid", "An exact core version, immutable commit, and manifest SHA-256 are required.")
		return
	}
	result, err := handler.commands.InspectCapabilityCandidate(request.Context(), reference)
	if err != nil {
		writeCapabilityProblem(w, request, "capability_inspect_failed", err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (handler *Handler) upgradeCapabilityCandidate(w http.ResponseWriter, request *http.Request) {
	if !handler.requireCommands(w, request) {
		return
	}
	if _, ok := strictCoreQuery(w, request); !ok {
		return
	}
	var input struct {
		ExactCoreVersion string `json:"exact_core_version"`
		CommitSHA        string `json:"commit_sha"`
		ManifestSHA256   string `json:"manifest_sha256"`
		Accept           *bool  `json:"accept"`
	}
	if !decodeStrictRequest(w, request, maximumRuntimeRequestBytes, &input) {
		return
	}
	reference := application.CapabilityUpgradeRequest{
		ExactCoreVersion: input.ExactCoreVersion,
		CommitSHA:        input.CommitSHA,
		ManifestSHA256:   input.ManifestSHA256,
	}
	if input.Accept == nil || !validCapabilityReference(reference) {
		writeProblem(w, request, http.StatusUnprocessableEntity, "capability_upgrade_invalid", "Capability upgrade invalid", "The immutable reference and explicit accept decision are required.")
		return
	}
	if !*input.Accept {
		preview, err := handler.commands.PreviewCapabilityUpgrade(request.Context(), reference)
		if err != nil {
			writeCapabilityProblem(w, request, "capability_upgrade_preview_failed", err)
			return
		}
		writeJSON(w, http.StatusOK, preview)
		return
	}
	result, err := handler.commands.UpgradeCapability(request.Context(), reference)
	if err != nil {
		writeCapabilityProblem(w, request, "capability_upgrade_failed", err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (handler *Handler) quarantineCapabilityManifest(
	w http.ResponseWriter,
	request *http.Request,
	manifestSHA256 string,
) {
	if !handler.requireCommands(w, request) {
		return
	}
	if _, ok := strictCoreQuery(w, request); !ok {
		return
	}
	digest, err := coreartifact.ParseSHA256(manifestSHA256)
	if err != nil || digest.IsZero() || digest.String() != manifestSHA256 {
		writeProblem(w, request, http.StatusBadRequest, "capability_quarantine_digest_invalid", "Capability digest invalid", "The path must identify one non-zero manifest SHA-256.")
		return
	}
	var input struct {
		ReasonCode string `json:"reason_code"`
	}
	if !decodeStrictRequest(w, request, maximumCapabilityRequestBytes, &input) {
		return
	}
	result, err := handler.commands.QuarantineCapabilityManifest(request.Context(), application.CapabilityQuarantineRequest{
		ManifestSHA256: digest.String(),
		ReasonCode:     input.ReasonCode,
	})
	if err != nil {
		switch {
		case errors.Is(err, application.ErrCapabilityQuarantineInvalid):
			writeProblem(w, request, http.StatusUnprocessableEntity, "capability_quarantine_invalid", "Capability quarantine invalid", err.Error())
		case errors.Is(err, application.ErrCapabilityQuarantineConflict):
			writeProblem(w, request, http.StatusConflict, "capability_quarantine_conflict", "Capability already quarantined", err.Error())
		default:
			writeProblem(w, request, http.StatusInternalServerError, "capability_quarantine_failed", "Capability quarantine failed", "The capability manifest could not be quarantined.")
		}
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func validCapabilityReference(reference application.CapabilityUpgradeRequest) bool {
	if !validOptionalExactVersion(reference.ExactCoreVersion, true) || reference.ExactCoreVersion == "" {
		return false
	}
	if len(reference.CommitSHA) != 40 && len(reference.CommitSHA) != 64 {
		return false
	}
	for _, character := range reference.CommitSHA {
		if (character < '0' || character > '9') && (character < 'a' || character > 'f') {
			return false
		}
	}
	digest, err := coreartifact.ParseSHA256(reference.ManifestSHA256)
	return err == nil && !digest.IsZero()
}

func writeCapabilityProblem(w http.ResponseWriter, request *http.Request, code string, err error) {
	switch {
	case errors.Is(err, capability.ErrInvalidGeneration):
		writeProblem(w, request, http.StatusUnprocessableEntity, "capability_generation_invalid", "Capability generation invalid", err.Error())
	case errors.Is(err, application.ErrCapabilityCandidateQuarantined),
		errors.Is(err, store.ErrCapabilityManifestQuarantined):
		writeProblem(w, request, http.StatusConflict, "capability_candidate_quarantined", "Capability candidate quarantined", err.Error())
	case errors.Is(err, store.ErrCapabilityGenerationConflict):
		writeProblem(w, request, http.StatusConflict, "capability_generation_conflict", "Capability generation conflict", err.Error())
	case errors.Is(err, store.ErrCapabilityGenerationNotFound),
		errors.Is(err, store.ErrCapabilityManifestNotFound):
		writeProblem(w, request, http.StatusNotFound, "capability_candidate_not_found", "Capability candidate not found", "The immutable capability candidate does not exist locally.")
	default:
		writeProblem(w, request, http.StatusInternalServerError, code, "Capability operation failed", "The capability operation could not be completed.")
	}
}

func (handler *Handler) coreRuntimeStatus(w http.ResponseWriter, request *http.Request) {
	if !handler.requireCommands(w, request) {
		return
	}
	if _, ok := strictCoreQuery(w, request); !ok {
		return
	}
	status, err := handler.commands.RuntimeStatus(request.Context())
	if err != nil {
		writeRuntimeProblem(w, request, "runtime_status_failed", err)
		return
	}
	writeJSON(w, http.StatusOK, status)
}

func (handler *Handler) queueStartupCheck(w http.ResponseWriter, request *http.Request) {
	if !handler.requireCommands(w, request) {
		return
	}
	if _, ok := strictCoreQuery(w, request); !ok {
		return
	}
	var input struct {
		StartupArtifactID string `json:"startup_artifact_id"`
	}
	if !decodeStrictRequest(w, request, maximumRuntimeRequestBytes, &input) {
		return
	}
	if !validStableIdentifier(input.StartupArtifactID) {
		writeProblem(w, request, http.StatusUnprocessableEntity, "startup_artifact_id_invalid", "Startup artifact ID invalid", "startup_artifact_id must identify one immutable candidate.")
		return
	}
	task, err := handler.commands.QueueStartupCheck(request.Context(), input.StartupArtifactID)
	if err != nil {
		writeRuntimeProblem(w, request, "startup_check_failed", err)
		return
	}
	writeJSON(w, http.StatusAccepted, task)
}

func (handler *Handler) queueCoreActivate(w http.ResponseWriter, request *http.Request) {
	if !handler.requireCommands(w, request) {
		return
	}
	if _, ok := strictCoreQuery(w, request); !ok {
		return
	}
	var input struct {
		StartupArtifactID string               `json:"startup_artifact_id"`
		MonitoringTier    store.MonitoringTier `json:"monitoring_tier"`
	}
	if !decodeStrictRequest(w, request, maximumRuntimeRequestBytes, &input) {
		return
	}
	if !validStableIdentifier(input.StartupArtifactID) || !validMonitoringTier(input.MonitoringTier, true) {
		writeProblem(w, request, http.StatusUnprocessableEntity, "activation_request_invalid", "Activation request invalid", "A startup artifact ID and a supported monitoring tier are required.")
		return
	}
	prepared, task, err := handler.commands.PrepareAndQueueRuntimeApply(
		request.Context(), input.StartupArtifactID, input.MonitoringTier,
	)
	if err != nil {
		writeRuntimeProblem(w, request, "runtime_activate_failed", err)
		return
	}
	writeJSON(w, http.StatusAccepted, struct {
		Activation application.ActivationSummary `json:"activation"`
		Task       application.Task              `json:"task"`
	}{Activation: prepared.Summary(), Task: task})
}

func (handler *Handler) queueRuntimeLifecycle(w http.ResponseWriter, request *http.Request, operation string) {
	if !handler.requireCommands(w, request) {
		return
	}
	if _, ok := strictCoreQuery(w, request); !ok || !requireEmptyCoreBody(w, request) {
		return
	}
	var (
		task application.Task
		err  error
	)
	switch operation {
	case "start":
		task, err = handler.commands.QueueRuntimeStart(request.Context())
	case "stop":
		task, err = handler.commands.QueueRuntimeStop(request.Context())
	case "restart":
		task, err = handler.commands.QueueRuntimeRestart(request.Context())
	case "rollback":
		task, err = handler.commands.QueueRuntimeRollback(request.Context())
	default:
		err = errors.New("unsupported runtime operation")
	}
	if err != nil {
		writeRuntimeProblem(w, request, "runtime_"+operation+"_failed", err)
		return
	}
	writeJSON(w, http.StatusAccepted, task)
}

func (handler *Handler) listManualArtifacts(w http.ResponseWriter, request *http.Request) {
	if !handler.requireCommands(w, request) {
		return
	}
	query, ok := strictCoreQuery(w, request, "core_version", "core_artifact_id", "limit")
	if !ok {
		return
	}
	if !validOptionalExactVersion(query.Get("core_version"), true) ||
		(query.Get("core_artifact_id") != "" && !validStableIdentifier(query.Get("core_artifact_id"))) {
		writeProblem(w, request, http.StatusBadRequest, "manual_filter_invalid", "Manual filter invalid", "The manual artifact filter contains an unsupported value.")
		return
	}
	limit, ok := optionalLimit(w, request)
	if !ok {
		return
	}
	resolution, artifacts, err := handler.commands.ListManualArtifacts(
		request.Context(), query.Get("core_version"), query.Get("core_artifact_id"), limit,
	)
	if err != nil {
		writeManualProblem(w, request, "manual_list_failed", err)
		return
	}
	writeJSON(w, http.StatusOK, struct {
		Resolution application.CoreVersionResolution `json:"resolution"`
		Items      []application.ManualArtifact      `json:"items"`
	}{Resolution: resolution, Items: artifacts})
}

func (handler *Handler) getManualArtifact(w http.ResponseWriter, request *http.Request, identifier string) {
	if !handler.requireCommands(w, request) {
		return
	}
	if !validStableIdentifier(identifier) {
		writeProblem(w, request, http.StatusBadRequest, "startup_artifact_id_invalid", "Startup artifact ID invalid", "The startup artifact ID is invalid.")
		return
	}
	if _, ok := strictCoreQuery(w, request); !ok {
		return
	}
	artifact, err := handler.commands.ManualArtifact(request.Context(), identifier)
	if err != nil {
		writeManualProblem(w, request, "manual_read_failed", err)
		return
	}
	writeJSON(w, http.StatusOK, artifact)
}

func (handler *Handler) replaceManualArtifact(w http.ResponseWriter, request *http.Request) {
	if !handler.requireCommands(w, request) {
		return
	}
	expectedHead, err := parseIfMatch(request.Header.Get("If-Match"))
	if err != nil {
		writeProblem(w, request, http.StatusPreconditionRequired, "base_revision_required", "Base revision required", err.Error())
		return
	}
	query, ok := strictCoreQuery(w, request, "core_version", "core_artifact_id", "allow_compatible")
	if !ok {
		return
	}
	allowCompatible, ok := optionalStrictBool(w, request, query, "allow_compatible")
	if !ok {
		return
	}
	if !validOptionalExactVersion(query.Get("core_version"), true) ||
		(query.Get("core_artifact_id") != "" && !validStableIdentifier(query.Get("core_artifact_id"))) {
		writeProblem(w, request, http.StatusBadRequest, "manual_binding_invalid", "Manual binding invalid", "The exact core version or artifact binding is invalid.")
		return
	}
	raw, err := readBoundedBody(request, manualjson.MaximumBytes)
	if err != nil {
		writeProblem(w, request, http.StatusRequestEntityTooLarge, "manual_too_large", "Manual configuration rejected", err.Error())
		return
	}
	saved, err := handler.commands.ReplaceManualJSON(request.Context(), application.ManualReplaceRequest{
		ExpectedHead:    expectedHead,
		CoreVersion:     query.Get("core_version"),
		CoreArtifactID:  query.Get("core_artifact_id"),
		Raw:             raw,
		AllowCompatible: allowCompatible,
	})
	if err != nil {
		writeManualProblem(w, request, "manual_replace_failed", err)
		return
	}
	w.Header().Set("ETag", quoteETag(saved.Revision.ID))
	writeJSON(w, http.StatusAccepted, saved)
}

func (handler *Handler) previewManualReplacement(w http.ResponseWriter, request *http.Request) {
	if !handler.requireCommands(w, request) {
		return
	}
	expectedHead, err := parseIfMatch(request.Header.Get("If-Match"))
	if err != nil {
		writeProblem(w, request, http.StatusPreconditionRequired, "base_revision_required", "Base revision required", err.Error())
		return
	}
	query, ok := strictCoreQuery(w, request, "core_version", "core_artifact_id", "allow_compatible")
	if !ok {
		return
	}
	allowCompatible, ok := optionalStrictBool(w, request, query, "allow_compatible")
	if !ok {
		return
	}
	if !validOptionalExactVersion(query.Get("core_version"), true) ||
		(query.Get("core_artifact_id") != "" && !validStableIdentifier(query.Get("core_artifact_id"))) {
		writeProblem(w, request, http.StatusBadRequest, "manual_binding_invalid", "Manual binding invalid", "The exact core version or artifact binding is invalid.")
		return
	}
	raw, err := readBoundedBody(request, manualjson.MaximumBytes)
	if err != nil {
		writeProblem(w, request, http.StatusRequestEntityTooLarge, "manual_too_large", "Manual configuration rejected", err.Error())
		return
	}
	preview, err := handler.commands.PreviewManualReplace(request.Context(), application.ManualReplaceRequest{
		ExpectedHead: expectedHead, CoreVersion: query.Get("core_version"),
		CoreArtifactID: query.Get("core_artifact_id"), Raw: raw,
		AllowCompatible: allowCompatible,
	})
	if err != nil {
		writeManualProblem(w, request, "manual_preview_failed", err)
		return
	}
	writeJSON(w, http.StatusOK, preview)
}

func (handler *Handler) discardManualArtifact(w http.ResponseWriter, request *http.Request, identifier string) {
	if !handler.requireCommands(w, request) {
		return
	}
	if !validStableIdentifier(identifier) {
		writeProblem(w, request, http.StatusBadRequest, "startup_artifact_id_invalid", "Startup artifact ID invalid", "The startup artifact ID is invalid.")
		return
	}
	if _, ok := strictCoreQuery(w, request); !ok || !requireEmptyCoreBody(w, request) {
		return
	}
	artifact, err := handler.commands.DiscardManualArtifact(request.Context(), identifier)
	if err != nil {
		writeManualProblem(w, request, "manual_discard_failed", err)
		return
	}
	writeJSON(w, http.StatusOK, artifact)
}

func validStableIdentifier(value string) bool {
	if value == "" || len(value) > 256 || strings.TrimSpace(value) != value {
		return false
	}
	for _, character := range value {
		if unicode.IsControl(character) {
			return false
		}
	}
	return true
}

func validMonitoringTier(value store.MonitoringTier, allowDefault bool) bool {
	if value == "" {
		return allowDefault
	}
	return value == store.MonitoringFull || value == store.MonitoringLimited || value == store.MonitoringProcessOnly
}

func writeRuntimeProblem(w http.ResponseWriter, request *http.Request, code string, err error) {
	switch {
	case application.IsStartupArtifactNotFound(err):
		writeProblem(w, request, http.StatusNotFound, "startup_artifact_not_found", "Startup artifact not found", "The requested startup artifact does not exist.")
	case errors.Is(err, store.ErrStartupArtifactState):
		writeProblem(w, request, http.StatusConflict, "startup_artifact_state_invalid", "Startup artifact state invalid", err.Error())
	case application.IsActivationBundleNotReady(err):
		writeProblem(w, request, http.StatusConflict, "activation_bundle_not_ready", "Activation bundle not ready", "The immutable startup artifact or core binding is not ready.")
	case application.IsMonitoringTierUnavailable(err):
		writeProblem(w, request, http.StatusConflict, "monitoring_tier_unavailable", "Monitoring tier unavailable", "Only process_only monitoring is available until a live collector probe is configured.")
	case application.IsNoAppliedBundle(err):
		writeProblem(w, request, http.StatusConflict, "no_applied_bundle", "No applied bundle", "No successfully applied bundle is available for this operation.")
	case application.IsNoRollbackBundle(err):
		writeProblem(w, request, http.StatusConflict, "no_rollback_bundle", "No rollback bundle", "No frozen rollback bundle is available.")
	case errors.Is(err, runtimeidentity.ErrStaleObservation), errors.Is(err, runtimeidentity.ErrInspectionUnavailable):
		writeProblem(w, request, http.StatusServiceUnavailable, "runtime_inspection_unavailable", "Runtime inspection unavailable", "The live core identity could not be verified.")
	default:
		writeProblem(w, request, http.StatusInternalServerError, code, "Runtime operation failed", "The runtime operation could not be completed.")
	}
}

func writeManualProblem(w http.ResponseWriter, request *http.Request, code string, err error) {
	switch {
	case application.IsNoRunningCore(err):
		writeProblem(w, request, http.StatusConflict, "core_not_running", "Core is not running", "core_version was omitted and no verified core is currently running.")
	case application.IsRevisionConflict(err):
		writeProblem(w, request, http.StatusPreconditionFailed, "canonical_revision_conflict", "Revision conflict", err.Error())
	case application.IsInvalidManualJSON(err):
		writeProblem(w, request, http.StatusUnprocessableEntity, "manual_json_invalid", "Manual configuration invalid", err.Error())
	case application.IsStartupArtifactNotFound(err):
		writeProblem(w, request, http.StatusNotFound, "startup_artifact_not_found", "Startup artifact not found", "The requested manual startup artifact does not exist.")
	default:
		writeProblem(w, request, http.StatusInternalServerError, code, "Manual configuration operation failed", "The manual configuration operation could not be completed.")
	}
}

func strictCoreQuery(w http.ResponseWriter, request *http.Request, allowed ...string) (url.Values, bool) {
	if len(request.URL.RawQuery) > maximumCoreQueryBytes {
		writeProblem(w, request, http.StatusBadRequest, "query_invalid", "Query invalid", "The query string is too large.")
		return nil, false
	}
	query, err := url.ParseQuery(request.URL.RawQuery)
	if err != nil {
		writeProblem(w, request, http.StatusBadRequest, "query_invalid", "Query invalid", "The query string is malformed.")
		return nil, false
	}
	permitted := make(map[string]struct{}, len(allowed))
	for _, name := range allowed {
		permitted[name] = struct{}{}
	}
	for name, values := range query {
		if _, ok := permitted[name]; !ok || len(values) != 1 || values[0] == "" {
			writeProblem(w, request, http.StatusBadRequest, "query_invalid", "Query invalid", "Query parameters must be recognized, non-empty, and occur at most once.")
			return nil, false
		}
	}
	return query, true
}

func optionalStrictBool(w http.ResponseWriter, request *http.Request, query url.Values, name string) (bool, bool) {
	values, present := query[name]
	if !present {
		return false, true
	}
	switch values[0] {
	case "true":
		return true, true
	case "false":
		return false, true
	default:
		writeProblem(w, request, http.StatusBadRequest, "query_invalid", "Query invalid", name+" must be true or false.")
		return false, false
	}
}

func requireEmptyCoreBody(w http.ResponseWriter, request *http.Request) bool {
	body, err := readBoundedBody(request, maximumCoreEmptyRequestBytes)
	if err != nil {
		writeProblem(w, request, http.StatusRequestEntityTooLarge, "request_too_large", "Request rejected", "The request body is too large.")
		return false
	}
	if len(strings.TrimSpace(string(body))) != 0 {
		writeProblem(w, request, http.StatusUnprocessableEntity, "request_body_not_allowed", "Request invalid", "This operation does not accept a request body.")
		return false
	}
	return true
}

func validOptionalExactVersion(value string, requireNonZero bool) bool {
	if value == "" {
		return true
	}
	version, err := coreartifact.ParseExactVersion(value)
	return err == nil && (!requireNonZero || !version.IsZero())
}

func validOptionalArchitecture(value string) bool {
	return value == "" || value == "amd64" || value == "arm64"
}

func validOptionalVariant(value string) bool {
	if value == "" {
		return true
	}
	if len(value) > 64 || value[0] < 'a' || value[0] > 'z' {
		return false
	}
	for _, character := range value {
		if (character >= 'a' && character <= 'z') ||
			(character >= '0' && character <= '9') ||
			character == '-' || character == '_' || character == '.' {
			continue
		}
		return false
	}
	return true
}

func validOptionalCoreArtifactSource(value string) bool {
	return value == "" || value == string(store.CoreArtifactSourceOfficial) || value == string(store.CoreArtifactSourceUserVerified)
}

func validOptionalCoreArtifactVerification(value string) bool {
	return value == "" || value == string(store.CoreArtifactVerified) ||
		value == string(store.CoreArtifactRevoked) || value == string(store.CoreArtifactQuarantined)
}

func validCoreArtifactID(value string) bool {
	if value == "" || len(value) > 256 || strings.TrimSpace(value) != value {
		return false
	}
	for _, character := range value {
		if unicode.IsControl(character) {
			return false
		}
	}
	return true
}

func validCoreImportRequest(request application.CoreImportRequest) bool {
	if !filepath.IsAbs(request.SourcePath) || filepath.Clean(request.SourcePath) != request.SourcePath {
		return false
	}
	digest, err := coreartifact.ParseSHA256(request.SHA256)
	if err != nil || digest.IsZero() {
		return false
	}
	version, err := coreartifact.ParseExactVersion(request.ExactVersion)
	if err != nil || version.IsZero() {
		return false
	}
	architecture := coreartifact.Architecture(request.Architecture)
	if architecture != coreartifact.ArchitectureAMD64 && architecture != coreartifact.ArchitectureARM64 {
		return false
	}
	variant := coreartifact.Variant(request.Variant)
	if variant == "" {
		variant = coreartifact.VariantPlain
	}
	source, err := coreartifact.NewUserSource(request.SourceDescription)
	if err != nil {
		return false
	}
	_, err = coreartifact.NewIdentity(
		source,
		digest,
		coreartifact.OperatingSystemLinux,
		architecture,
		variant,
		version,
	)
	return err == nil
}

type coreCapabilityResponse struct {
	Resolution   application.CoreVersionResolution   `json:"resolution"`
	SupportLevel string                              `json:"support_level"`
	Pinned       bool                                `json:"pinned"`
	Pin          *coreCapabilityPinResponse          `json:"pin,omitempty"`
	Quarantined  bool                                `json:"quarantined"`
	ReasonCode   string                              `json:"reason_code,omitempty"`
	Presentation *application.CapabilityPresentation `json:"presentation,omitempty"`
}

type coreCapabilityPinResponse struct {
	ExactCoreVersion string    `json:"exact_core_version"`
	Repository       string    `json:"repository"`
	CommitSHA        string    `json:"commit_sha"`
	ManifestSHA256   string    `json:"manifest_sha256"`
	SupportLevel     string    `json:"support_level"`
	PinnedAt         time.Time `json:"pinned_at"`
}

func newCoreCapabilityResponse(status application.CapabilityStatus) coreCapabilityResponse {
	result := coreCapabilityResponse{
		Resolution:   status.Resolution,
		SupportLevel: string(status.SupportLevel),
		Pinned:       status.Pinned,
		Quarantined:  status.Quarantined,
		ReasonCode:   status.ReasonCode,
		Presentation: status.Presentation,
	}
	if status.Pin != nil {
		result.Pin = &coreCapabilityPinResponse{
			ExactCoreVersion: status.Pin.ExactCoreVersion,
			Repository:       status.Pin.Repository,
			CommitSHA:        status.Pin.CommitSHA,
			ManifestSHA256:   status.Pin.ManifestSHA256,
			SupportLevel:     string(status.Pin.SupportLevel),
			PinnedAt:         status.Pin.PinnedAt,
		}
	}
	return result
}

func (handler *Handler) listEntities(w http.ResponseWriter, request *http.Request, collection canonical.Collection) {
	if !handler.requireCommands(w, request) {
		return
	}
	result, err := handler.commands.ListEntities(request.Context(), collection)
	if err != nil {
		writeCanonicalProblem(w, request, "entity_list_failed", err)
		return
	}
	w.Header().Set("ETag", quoteETag(result.Revision.ID))
	writeJSON(w, http.StatusOK, result)
}

func (handler *Handler) getEntity(w http.ResponseWriter, request *http.Request, collection canonical.Collection, identifier string) {
	if !handler.requireCommands(w, request) {
		return
	}
	revision, entity, err := handler.commands.GetEntity(request.Context(), collection, identifier)
	if err != nil {
		writeCanonicalProblem(w, request, "entity_read_failed", err)
		return
	}
	result := struct {
		Revision application.CanonicalSnapshot `json:"revision"`
		Entity   map[string]any                `json:"entity"`
	}{Revision: revision, Entity: entity}
	w.Header().Set("ETag", quoteETag(revision.ID))
	writeJSON(w, http.StatusOK, result)
}

func (handler *Handler) createEntity(w http.ResponseWriter, request *http.Request, collection canonical.Collection) {
	expectedHead, entity, ok := handler.entityWriteInput(w, request)
	if !ok {
		return
	}
	result, err := handler.commands.CreateEntity(request.Context(), expectedHead, collection, entity)
	if err != nil {
		writeCanonicalProblem(w, request, "entity_create_failed", err)
		return
	}
	writeCanonicalSave(w, http.StatusCreated, result)
}

func (handler *Handler) replaceEntity(w http.ResponseWriter, request *http.Request, collection canonical.Collection, identifier string) {
	expectedHead, entity, ok := handler.entityWriteInput(w, request)
	if !ok {
		return
	}
	result, err := handler.commands.ReplaceEntity(request.Context(), expectedHead, collection, identifier, entity)
	if err != nil {
		writeCanonicalProblem(w, request, "entity_update_failed", err)
		return
	}
	writeCanonicalSave(w, http.StatusOK, result)
}

func (handler *Handler) deleteEntity(w http.ResponseWriter, request *http.Request, collection canonical.Collection, identifier string) {
	if !handler.requireCommands(w, request) {
		return
	}
	expectedHead, err := parseIfMatch(request.Header.Get("If-Match"))
	if err != nil {
		writeProblem(w, request, http.StatusPreconditionRequired, "base_revision_required", "Base revision required", err.Error())
		return
	}
	result, err := handler.commands.DeleteEntity(request.Context(), expectedHead, collection, identifier)
	if err != nil {
		writeCanonicalProblem(w, request, "entity_delete_failed", err)
		return
	}
	writeCanonicalSave(w, http.StatusOK, result)
}

func (handler *Handler) setEntityEnabled(w http.ResponseWriter, request *http.Request, collection canonical.Collection, identifier string) {
	if !handler.requireCommands(w, request) {
		return
	}
	expectedHead, err := parseIfMatch(request.Header.Get("If-Match"))
	if err != nil {
		writeProblem(w, request, http.StatusPreconditionRequired, "base_revision_required", "Base revision required", err.Error())
		return
	}
	var input struct {
		Enabled *bool `json:"enabled"`
	}
	if !decodeStrictRequest(w, request, canonical.MaximumBytes, &input) {
		return
	}
	if input.Enabled == nil {
		writeProblem(w, request, http.StatusUnprocessableEntity, "enabled_required", "Entity update invalid", "enabled must be explicitly true or false.")
		return
	}
	result, err := handler.commands.SetEntityEnabled(request.Context(), expectedHead, collection, identifier, *input.Enabled)
	if err != nil {
		writeCanonicalProblem(w, request, "entity_update_failed", err)
		return
	}
	writeCanonicalSave(w, http.StatusOK, result)
}

func (handler *Handler) moveEntity(w http.ResponseWriter, request *http.Request, collection canonical.Collection, identifier string) {
	if !handler.requireCommands(w, request) {
		return
	}
	expectedHead, err := parseIfMatch(request.Header.Get("If-Match"))
	if err != nil {
		writeProblem(w, request, http.StatusPreconditionRequired, "base_revision_required", "Base revision required", err.Error())
		return
	}
	var input struct {
		BeforeID string `json:"before_id"`
	}
	if !decodeStrictRequest(w, request, canonical.MaximumBytes, &input) {
		return
	}
	result, err := handler.commands.MoveEntity(request.Context(), expectedHead, collection, identifier, input.BeforeID)
	if err != nil {
		writeCanonicalProblem(w, request, "entity_move_failed", err)
		return
	}
	writeCanonicalSave(w, http.StatusOK, result)
}

func (handler *Handler) entityWriteInput(w http.ResponseWriter, request *http.Request) (string, map[string]any, bool) {
	if !handler.requireCommands(w, request) {
		return "", nil, false
	}
	expectedHead, err := parseIfMatch(request.Header.Get("If-Match"))
	if err != nil {
		writeProblem(w, request, http.StatusPreconditionRequired, "base_revision_required", "Base revision required", err.Error())
		return "", nil, false
	}
	var entity map[string]any
	if !decodeStrictRequest(w, request, canonical.MaximumBytes, &entity) {
		return "", nil, false
	}
	if entity == nil {
		writeProblem(w, request, http.StatusUnprocessableEntity, "entity_invalid", "Entity invalid", "The request body must be a JSON object.")
		return "", nil, false
	}
	return expectedHead, entity, true
}

func (handler *Handler) listRevisions(w http.ResponseWriter, request *http.Request) {
	if !handler.requireCommands(w, request) {
		return
	}
	before, ok := optionalPositiveInt64(w, request, "before_sequence")
	if !ok {
		return
	}
	limit, ok := optionalLimit(w, request)
	if !ok {
		return
	}
	page, err := handler.commands.ListCanonicalRevisions(request.Context(), before, limit)
	if err != nil {
		writeProblem(w, request, http.StatusBadRequest, "revision_filter_invalid", "Revision filter invalid", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, page)
}

func (handler *Handler) getRevision(w http.ResponseWriter, request *http.Request, reference string) {
	if !handler.requireCommands(w, request) {
		return
	}
	revision, err := handler.commands.CanonicalRevision(request.Context(), reference)
	if err != nil {
		writeRevisionProblem(w, request, "revision_read_failed", err)
		return
	}
	w.Header().Set("ETag", quoteETag(revision.ID))
	writeJSON(w, http.StatusOK, revision)
}

func (handler *Handler) diffRevisions(w http.ResponseWriter, request *http.Request) {
	if !handler.requireCommands(w, request) {
		return
	}
	from, to := request.URL.Query().Get("from"), request.URL.Query().Get("to")
	if from == "" || to == "" {
		writeProblem(w, request, http.StatusBadRequest, "revision_diff_references_required", "Revision references required", "Both from and to query parameters are required.")
		return
	}
	diff, err := handler.commands.DiffCanonicalRevisions(request.Context(), from, to)
	if err != nil {
		writeRevisionProblem(w, request, "revision_diff_failed", err)
		return
	}
	writeJSON(w, http.StatusOK, diff)
}

func (handler *Handler) restoreRevision(w http.ResponseWriter, request *http.Request, reference string) {
	if !handler.requireCommands(w, request) {
		return
	}
	expectedHead, err := parseIfMatch(request.Header.Get("If-Match"))
	if err != nil {
		writeProblem(w, request, http.StatusPreconditionRequired, "base_revision_required", "Base revision required", err.Error())
		return
	}
	result, err := handler.commands.RestoreCanonicalRevision(request.Context(), expectedHead, reference)
	if err != nil {
		if application.IsRevisionConflict(err) {
			writeProblem(w, request, http.StatusPreconditionFailed, "canonical_revision_conflict", "Revision conflict", err.Error())
			return
		}
		writeRevisionProblem(w, request, "revision_restore_failed", err)
		return
	}
	writeCanonicalSave(w, http.StatusOK, result)
}

func (handler *Handler) listTasks(w http.ResponseWriter, request *http.Request) {
	if !handler.requireCommands(w, request) {
		return
	}
	limit, ok := optionalLimit(w, request)
	if !ok {
		return
	}
	page, err := handler.commands.ListTasks(request.Context(), application.TaskListFilter{
		Lane:   store.TaskLane(request.URL.Query().Get("lane")),
		Status: store.TaskStatus(request.URL.Query().Get("status")),
		Kind:   request.URL.Query().Get("kind"),
		Limit:  limit,
	})
	if err != nil {
		writeProblem(w, request, http.StatusBadRequest, "task_filter_invalid", "Task filter invalid", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, page)
}

func (handler *Handler) getTask(w http.ResponseWriter, request *http.Request, taskID string) {
	if !handler.requireCommands(w, request) {
		return
	}
	task, err := handler.commands.Task(request.Context(), taskID)
	if err != nil {
		writeTaskProblem(w, request, "task_read_failed", err)
		return
	}
	writeJSON(w, http.StatusOK, task)
}

func (handler *Handler) cancelTask(w http.ResponseWriter, request *http.Request, taskID string) {
	if !handler.requireCommands(w, request) {
		return
	}
	task, err := handler.commands.CancelTask(request.Context(), taskID)
	if err != nil {
		writeTaskProblem(w, request, "task_cancel_failed", err)
		return
	}
	writeJSON(w, http.StatusOK, task)
}

func decodeStrictRequest(w http.ResponseWriter, request *http.Request, limit int64, target any) bool {
	raw, err := readBoundedBody(request, limit)
	if err != nil {
		writeProblem(w, request, http.StatusRequestEntityTooLarge, "request_too_large", "Request rejected", err.Error())
		return false
	}
	if err := jsonstrict.Decode(raw, limit, target); err != nil {
		writeProblem(w, request, http.StatusUnprocessableEntity, "invalid_json", "Request invalid", err.Error())
		return false
	}
	return true
}

func optionalPositiveInt64(w http.ResponseWriter, request *http.Request, name string) (int64, bool) {
	raw := request.URL.Query().Get(name)
	if raw == "" {
		return 0, true
	}
	value, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || value < 1 {
		writeProblem(w, request, http.StatusBadRequest, "query_invalid", "Query invalid", name+" must be a positive integer.")
		return 0, false
	}
	return value, true
}

func optionalLimit(w http.ResponseWriter, request *http.Request) (int, bool) {
	raw := request.URL.Query().Get("limit")
	if raw == "" {
		return 50, true
	}
	value, err := strconv.Atoi(raw)
	if err != nil || value < 1 || value > 200 {
		writeProblem(w, request, http.StatusBadRequest, "query_invalid", "Query invalid", "limit must be between 1 and 200.")
		return 0, false
	}
	return value, true
}

func writeCanonicalSave(w http.ResponseWriter, status int, result application.CanonicalSave) {
	w.Header().Set("ETag", quoteETag(result.Revision.ID))
	writeJSON(w, status, result)
}

func writeCanonicalProblem(w http.ResponseWriter, request *http.Request, code string, err error) {
	switch {
	case application.IsRevisionConflict(err):
		writeProblem(w, request, http.StatusPreconditionFailed, "canonical_revision_conflict", "Revision conflict", err.Error())
	case errors.Is(err, canonical.ErrEntityNotFound):
		writeProblem(w, request, http.StatusNotFound, "entity_not_found", "Entity not found", err.Error())
	case errors.Is(err, canonical.ErrEntityExists):
		writeProblem(w, request, http.StatusConflict, "entity_exists", "Entity exists", err.Error())
	case errors.Is(err, canonical.ErrEntityReferenced):
		writeProblem(w, request, http.StatusConflict, "entity_referenced", "Entity is referenced", err.Error())
	case errors.Is(err, canonical.ErrInvalidDocument):
		writeProblem(w, request, http.StatusUnprocessableEntity, "canonical_invalid", "Configuration invalid", err.Error())
	default:
		writeProblem(w, request, http.StatusInternalServerError, code, "Configuration operation failed", "The configuration operation could not be completed.")
	}
}

func writeRevisionProblem(w http.ResponseWriter, request *http.Request, code string, err error) {
	if application.IsRevisionNotFound(err) {
		writeProblem(w, request, http.StatusNotFound, "revision_not_found", "Revision not found", err.Error())
		return
	}
	writeProblem(w, request, http.StatusInternalServerError, code, "Revision operation failed", "The revision operation could not be completed.")
}

func writeTaskProblem(w http.ResponseWriter, request *http.Request, code string, err error) {
	if application.IsTaskNotFound(err) {
		writeProblem(w, request, http.StatusNotFound, "task_not_found", "Task not found", err.Error())
		return
	}
	writeProblem(w, request, http.StatusInternalServerError, code, "Task operation failed", "The task operation could not be completed.")
}
