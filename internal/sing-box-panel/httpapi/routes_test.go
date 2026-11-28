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

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/getkin/kin-openapi/openapi3filter"
	"github.com/getkin/kin-openapi/routers"
	"github.com/getkin/kin-openapi/routers/legacy"
	"github.com/rehuony/sing-box-panel/internal/buildinfo"
	"github.com/rehuony/sing-box-panel/internal/sing-box-panel/nodeconfig"
	"github.com/rehuony/sing-box-panel/internal/sing-box-panel/settings"
	"github.com/rehuony/sing-box-panel/internal/sing-box-panel/store"
	"github.com/rehuony/sing-box-panel/internal/sing-box-panel/subscription"
	"github.com/rehuony/sing-box-panel/internal/sing-box-panel/testutil"
)

func TestRoutesMatchOpenAPIAndDispatcher(t *testing.T) {
	contract := loadOpenAPIContract(t)
	openAPIOperations := make(map[string]string)
	for path, item := range contract.Paths.Map() {
		for method, operation := range item.Operations() {
			openAPIOperations[strings.ToUpper(method)+" "+path] = operation.OperationID
		}
	}
	registered := make(map[string]string, len(routes))
	for _, operation := range routes {
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

	handler := newTestHandler(t, HandlerOptions{
		Settings: settings.Settings{Auth: settings.Auth{Email: testutil.AdminEmail, PasswordHash: testutil.PasswordHash}},
		Build:    buildinfo.Info{Version: "test"},
	})
	for _, operation := range routes {
		requestPath := instantiateOperationPath(operation.Path)
		if operation.access != accessDownload {
			matched, found, _ := matchRoute(httptest.NewRequest(operation.Method, "/", nil), requestPath)
			if !found || matched.OperationID != operation.OperationID {
				t.Errorf("%s %s dispatches to %q, want %q", operation.Method, requestPath, matched.OperationID, operation.OperationID)
			}
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
	value := settings.Defaults()
	value.Auth.Email = testutil.AdminEmail
	value.Auth.PasswordHash = testutil.PasswordHash
	value = settingsFileFixture(t, value)
	handler := newTestHandler(t, HandlerOptions{
		Settings: value,
		Build:    buildinfo.Info{Version: "test"},
		Services: newTestServices(t, database, value),
	})

	nodePath := testNodePath(t, database)
	configurationFile := nodePath + "/config/file"
	serveConformingRequest(t, router, handler, http.MethodPut, configurationFile,
		`{"revision":0,"content":"{}"}`, http.StatusOK, true, nil)

	fileResponse := serveConformingRequest(t, router, handler, http.MethodGet, configurationFile, "", http.StatusOK, true, nil)
	var file nodeconfig.ConfigurationFile
	if err := json.Unmarshal(fileResponse.Body.Bytes(), &file); err != nil {
		t.Fatal(err)
	}
	fileBody, err := json.Marshal(nodeconfig.ConfigurationFileWrite{Revision: file.Revision, Content: "{"})
	if err != nil {
		t.Fatal(err)
	}
	serveConformingRequest(t, router, handler, http.MethodPut, configurationFile, string(fileBody), http.StatusOK, true, nil)
	panel := serveConformingRequest(t, router, handler, http.MethodGet, "/api/v1/panel/settings", "", http.StatusOK, true, nil)
	var panelView settings.PanelSettingsView
	if err := json.Unmarshal(panel.Body.Bytes(), &panelView); err != nil {
		t.Fatal(err)
	}
	panelBody, err := json.Marshal(settings.PanelSettingsWrite{Preferences: panelView.Preferences, Revision: panelView.Revision})
	if err != nil {
		t.Fatal(err)
	}
	serveConformingRequest(t, router, handler, http.MethodPut, "/api/v1/panel/settings", string(panelBody), http.StatusOK, true, nil)

	created := serveConformingRequest(t, router, handler, http.MethodPost, "/api/v1/subscription/sources",
		`{"name":"contract source","kind":"file","filter":{"include":[],"exclude":["skip"]},"content":"socks5://a.example:1080#A\nsocks5://b.example:1080#B\nsocks5://c.example:1080#skip"}`,
		http.StatusCreated, true, nil)
	var source subscription.SubscriptionSource
	if err := json.Unmarshal(created.Body.Bytes(), &source); err != nil {
		t.Fatal(err)
	}
	serveConformingRequest(t, router, handler, http.MethodGet, "/api/v1/subscription/sources", "", http.StatusOK, true, nil)
	serveConformingRequest(t, router, handler, http.MethodPut, "/api/v1/subscription/source-order", `{"ids":["`+source.ID+`"]}`, http.StatusNoContent, true, nil)
	serveConformingRequest(t, router, handler, http.MethodPut, "/api/v1/subscription/source-order", `{"ids":[]}`, http.StatusConflict, true, nil)
	serveConformingRequest(t, router, handler, http.MethodPost, "/api/v1/subscription/sources/preview",
		`{"source_id":"`+source.ID+`","filter":{"include":[],"exclude":[]}}`, http.StatusOK, true, nil)
	serveConformingRequest(t, router, handler, http.MethodPut, "/api/v1/subscription/sources/"+source.ID,
		`{"name":"contract source","filter":{"include":[],"exclude":["skip"]},"content":"trojan://broken.example:443#Broken"}`,
		http.StatusUnprocessableEntity, true, map[string]string{"If-Match": created.Header().Get("ETag")})
	listed := serveConformingRequest(t, router, handler, http.MethodGet, "/api/v1/subscription/nodes", "", http.StatusOK, true, nil)
	var pool subscription.SubscriptionNodeList
	if err := json.Unmarshal(listed.Body.Bytes(), &pool); err != nil || len(pool.Items) != 2 {
		t.Fatalf("pool = %s, %v", listed.Body.String(), err)
	}
	node := serveConformingRequest(t, router, handler, http.MethodGet, "/api/v1/subscription/nodes/"+pool.Items[0].ID, "", http.StatusOK, true, nil)
	serveConformingRequest(t, router, handler, http.MethodPut, "/api/v1/subscription/nodes/"+pool.Items[0].ID,
		`{"display_name":"Renamed"}`, http.StatusOK, true, map[string]string{"If-Match": node.Header().Get("ETag")})
	serveConformingRequest(t, router, handler, http.MethodPut, "/api/v1/subscription/node-order",
		`{"ids":["`+pool.Items[1].ID+`","`+pool.Items[0].ID+`"]}`, http.StatusOK, true, nil)
	serveConformingRequest(t, router, handler, http.MethodPut, "/api/v1/subscription/node-order",
		`{"ids":["`+pool.Items[1].ID+`"]}`, http.StatusConflict, true, nil)
	ruleSet := serveConformingRequest(t, router, handler, http.MethodPost, "/api/v1/subscription/rule-sets",
		`{"client":"mihomo","name":"contract rules","rules":[{"kind":"domain_suffix","value":"example.net"},
		{"kind":"remote","remote":{"name":"Remote","url":"https://rules.example/list.yaml","format":"yaml","behavior":"domain","accelerated":false}}]}`,
		http.StatusCreated, true, nil)
	var set subscription.SubscriptionRuleSet
	if err := json.Unmarshal(ruleSet.Body.Bytes(), &set); err != nil {
		t.Fatal(err)
	}
	serveConformingRequest(t, router, handler, http.MethodPut, "/api/v1/subscription/rule-sets/"+set.ID,
		`{"name":"contract rules","rules":[{"kind":"ip_cidr","value":"10.0.0.0/8"}]}`, http.StatusOK, true,
		map[string]string{"If-Match": ruleSet.Header().Get("ETag")})
	channel := serveConformingRequest(t, router, handler, http.MethodPost, "/api/v1/subscription/channels",
		`{"name":"contract channel","format":"mihomo","enabled":true,"config":{"groups":[{"id":"main","name":"Main","enabled":true,"type":"url-test",
		"health_check":{"url":"https://probe.example/","interval":600,"tolerance":10},
		"match":{"include":["A"],"exclude":[],"origins":["source:`+source.ID+`"],"protocols":["socks"]},
		"members":[{"kind":"node","id":"`+pool.Items[0].ID+`"},{"kind":"builtin","id":"direct"}]}],
		"routes":[{"id":"rule","enabled":true,"rule":{"kind":"domain","value":"example.org"},"exit":{"kind":"node","id":"`+pool.Items[1].ID+`"}},
		{"id":"set","enabled":true,"rule_set_id":"`+set.ID+`","exit":{"kind":"reject"}}],
		"default_exit":{"kind":"group","id":"main"}}}`, http.StatusCreated, true, nil)
	var stored subscription.SubscriptionChannel
	if err := json.Unmarshal(channel.Body.Bytes(), &stored); err != nil {
		t.Fatal(err)
	}
	serveConformingRequest(t, router, handler, http.MethodGet, "/api/v1/subscription/channels", "", http.StatusOK, true, nil)
	serveConformingRequest(t, router, handler, http.MethodPut, "/api/v1/subscription/channel-order", `{"ids":["`+stored.ID+`"]}`, http.StatusNoContent, true, nil)
	serveConformingRequest(t, router, handler, http.MethodGet, "/api/v1/subscription/tokens", "", http.StatusOK, true, nil)
	serveConformingRequest(t, router, handler, http.MethodPut, "/api/v1/subscription/token-order", `{"ids":[]}`, http.StatusNoContent, true, nil)
	serveConformingRequest(t, router, handler, http.MethodGet, "/api/v1/subscription/channels/"+stored.ID, "", http.StatusOK, true, nil)
	serveConformingRequest(t, router, handler, http.MethodPost, "/api/v1/subscription/channels/"+stored.ID+"/preview", `{}`, http.StatusOK, true, nil)
	serveConformingRequest(t, router, handler, http.MethodGet, "/api/v1/subscription/rule-sets?client=mihomo", "", http.StatusOK, true, nil)
	current := serveConformingRequest(t, router, handler, http.MethodGet, "/api/v1/subscription/rule-sets/"+set.ID, "", http.StatusOK, true, nil)
	serveConformingRequest(t, router, handler, http.MethodDelete, "/api/v1/subscription/rule-sets/"+set.ID, "", http.StatusNoContent, true,
		map[string]string{"If-Match": current.Header().Get("ETag")})

	// A development build reports its version and updates nothing.
	serveConformingRequest(t, router, handler, http.MethodGet, "/api/v1/system/update", "", http.StatusOK, true, nil)
	serveConformingRequest(t, router, handler, http.MethodPost, "/api/v1/system/update", "", http.StatusConflict, true, nil)
	serveConformingRequest(t, router, handler, http.MethodPost, nodePath+"/update", "", http.StatusConflict, true, nil)
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
			testutil.Authorize(t, handler, request)
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
	contractPath := filepath.Join(filepath.Dir(filename), "..", "..", "..", "api", "panel-openapi.yaml")
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
