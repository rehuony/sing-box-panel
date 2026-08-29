// SPDX-License-Identifier: GPL-3.0-or-later

package httpapi

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/getkin/kin-openapi/openapi3filter"
	"github.com/getkin/kin-openapi/routers"
	"github.com/getkin/kin-openapi/routers/legacy"
	"github.com/rehuony/sing-box-panel/internal/application"
	"github.com/rehuony/sing-box-panel/internal/buildinfo"
	"github.com/rehuony/sing-box-panel/internal/settings"
	"github.com/rehuony/sing-box-panel/internal/store"
)

func TestManagementOperationRegistryMatchesOpenAPIAndDispatcher(t *testing.T) {
	contract := loadOpenAPIContract(t)
	openAPIOperations := make(map[string]string)
	for path, item := range contract.Paths.Map() {
		for method, operation := range item.Operations() {
			openAPIOperations[strings.ToUpper(method)+" "+path] = operation.OperationID
		}
	}
	registered := make(map[string]string, len(managementOperations))
	for _, operation := range managementOperations {
		key := operation.Method + " " + operation.Path
		if previous, duplicate := registered[key]; duplicate {
			t.Fatalf("duplicate registered operation %s (%s and %s)", key, previous, operation.OperationID)
		}
		registered[key] = operation.OperationID
	}
	if len(registered) != len(openAPIOperations) {
		t.Fatalf("registered operation count = %d, OpenAPI count = %d", len(registered), len(openAPIOperations))
	}
	for key, operationID := range openAPIOperations {
		if registered[key] != operationID {
			t.Errorf("OpenAPI operation %s = %q, registered = %q", key, operationID, registered[key])
		}
	}
	for key, operationID := range registered {
		if openAPIOperations[key] != operationID {
			t.Errorf("registered operation %s = %q, OpenAPI = %q", key, operationID, openAPIOperations[key])
		}
	}

	handler := NewHandler(HandlerOptions{
		Settings: settings.Settings{Auth: settings.Auth{Token: "operation-registry-test"}},
		Build:    buildinfo.Info{Version: "test"},
	})
	for _, operation := range managementOperations {
		requestPath := instantiateOperationPath(operation.Path)
		if !strings.HasPrefix(requestPath, "/sub/") {
			requestPath = "/api/v1" + requestPath
		}
		request := httptest.NewRequest(operation.Method, requestPath, nil)
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		if response.Code == http.StatusMethodNotAllowed {
			t.Errorf("dispatcher rejected registered operation %s %s", operation.Method, requestPath)
			continue
		}
		var problem Problem
		if strings.HasPrefix(response.Header().Get("Content-Type"), "application/problem+json") &&
			json.Unmarshal(response.Body.Bytes(), &problem) == nil && problem.Code == "operation_not_found" {
			t.Errorf("dispatcher did not implement registered operation %s %s (%s)", operation.Method, requestPath, operation.OperationID)
		}
	}
}

func TestRepresentativeHTTPResponsesConformToOpenAPI(t *testing.T) {
	ctx := context.Background()
	contract := loadOpenAPIContract(t)
	router, err := legacy.NewRouter(contract)
	if err != nil {
		t.Fatalf("create OpenAPI router: %v", err)
	}
	database, err := store.Open(ctx, filepath.Join(t.TempDir(), "panel.db"))
	if err != nil {
		t.Fatalf("open test store: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })
	handler := NewHandler(HandlerOptions{
		Settings: settings.Settings{Auth: settings.Auth{Token: "openapi-response-test"}},
		Build:    buildinfo.Info{Version: "test"},
		Commands: application.FromStore(database),
	})

	canonical := serveConformingRequest(t, router, handler, http.MethodPut, "/api/v1/config/canonical",
		`{"schema_version":2,"configuration":{}}`, http.StatusOK, true, map[string]string{"If-Match": `"none"`})
	var saved application.CanonicalSave
	if err := json.Unmarshal(canonical.Body.Bytes(), &saved); err != nil || saved.TaskID == "" {
		t.Fatalf("decode canonical save: save=%+v err=%v", saved, err)
	}
	serveConformingRequest(t, router, handler, http.MethodGet, "/api/v1/core/status", "", http.StatusOK, true, nil)
	serveConformingRequest(t, router, handler, http.MethodGet, "/api/v1/tasks/"+saved.TaskID, "", http.StatusOK, true, nil)

	now := time.Date(2026, time.August, 29, 10, 0, 0, 0, time.UTC)
	source, err := database.CreateSubscriptionSource(ctx, store.SubscriptionSource{
		ID: "source-contract", Name: "contract source", SourceKind: store.SubscriptionSourceLocal,
		Config: json.RawMessage(`{}`), Enabled: true, CreatedAt: now,
	})
	if err != nil {
		t.Fatal(err)
	}
	version, err := database.SaveSubscriptionSourceVersion(ctx, store.SaveSubscriptionSourceVersionInput{
		Version: store.SubscriptionSourceVersion{
			ID: "version-contract", SourceID: source.ID, Format: "uri-list",
			RawBody: []byte("socks://127.0.0.1:1080"), NormalizedNodes: json.RawMessage(`[]`),
			Diagnostics: json.RawMessage(`[]`), FetchedAt: now.Add(time.Minute), CreatedAt: now.Add(time.Minute),
		},
		ExpectedSourceUpdatedAt: source.UpdatedAt,
		UpdatedAt:               now.Add(time.Minute),
	})
	if err != nil {
		t.Fatal(err)
	}
	serveConformingRequest(t, router, handler, http.MethodGet,
		"/api/v1/subscription/sources/"+source.ID+"/versions/"+version.Version.ID,
		"", http.StatusOK, true, nil)
	serveConformingRequest(t, router, handler, http.MethodGet, "/api/v1/tasks/"+saved.TaskID,
		"", http.StatusUnauthorized, false, nil)
}

func serveConformingRequest(
	t *testing.T,
	router routers.Router,
	handler http.Handler,
	method string,
	target string,
	body string,
	wantStatus int,
	authenticated bool,
	headers map[string]string,
) *httptest.ResponseRecorder {
	t.Helper()
	newRequest := func() *http.Request {
		request := httptest.NewRequest(method, target, strings.NewReader(body))
		if authenticated {
			request.Header.Set("Authorization", "Bearer openapi-response-test")
		}
		if body != "" {
			request.Header.Set("Content-Type", "application/json")
		}
		for name, value := range headers {
			request.Header.Set(name, value)
		}
		return request
	}
	contractRequest := newRequest()
	if err := validateOpenAPIRequestValue(router, contractRequest); err != nil {
		t.Fatal(err)
	}
	request := newRequest()
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != wantStatus {
		t.Fatalf("%s %s status=%d want=%d body=%s", method, target, response.Code, wantStatus, response.Body.String())
	}
	if err := validateOpenAPIResponseValue(router, request, response); err != nil {
		t.Fatal(err)
	}
	return response
}

func validateOpenAPIRequestValue(router routers.Router, request *http.Request) error {
	route, pathParameters, err := router.FindRoute(request)
	if err != nil {
		return fmt.Errorf("match OpenAPI request route: %w", err)
	}
	if err := openapi3filter.ValidateRequest(request.Context(), &openapi3filter.RequestValidationInput{
		Request: request, PathParams: pathParameters, Route: route,
		Options: &openapi3filter.Options{AuthenticationFunc: openapi3filter.NoopAuthenticationFunc},
	}); err != nil {
		return fmt.Errorf("request does not conform to %s: %w", route.Operation.OperationID, err)
	}
	return nil
}

func validateOpenAPIResponseValue(router routers.Router, request *http.Request, response *httptest.ResponseRecorder) error {
	route, pathParameters, err := router.FindRoute(request)
	if err != nil {
		return fmt.Errorf("match OpenAPI route: %w", err)
	}
	input := &openapi3filter.ResponseValidationInput{
		RequestValidationInput: &openapi3filter.RequestValidationInput{
			Request: request, PathParams: pathParameters, Route: route,
		},
		Status: response.Code,
		Header: response.Header(),
	}
	input.SetBodyBytes(response.Body.Bytes())
	if err := openapi3filter.ValidateResponse(request.Context(), input); err != nil {
		return fmt.Errorf("response does not conform to %s: %w\n%s", route.Operation.OperationID, err, response.Body.String())
	}
	return nil
}

func loadOpenAPIContract(t *testing.T) *openapi3.T {
	t.Helper()
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("locate operation registry test source")
	}
	contractPath := filepath.Join(filepath.Dir(filename), "..", "..", "api", "openapi.yaml")
	contract, err := openapi3.NewLoader().LoadFromFile(contractPath)
	if err != nil {
		t.Fatalf("load OpenAPI contract: %v", err)
	}
	if err := contract.Validate(context.Background()); err != nil {
		t.Fatalf("validate OpenAPI contract: %v", err)
	}
	return contract
}

func instantiateOperationPath(template string) string {
	parts := strings.Split(template, "/")
	for index, part := range parts {
		if strings.HasPrefix(part, "{") && strings.HasSuffix(part, "}") {
			parts[index] = "contract-test"
		}
	}
	return strings.Join(parts, "/")
}
