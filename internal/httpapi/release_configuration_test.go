// SPDX-License-Identifier: GPL-3.0-or-later

package httpapi

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/getkin/kin-openapi/routers/legacy"
	"github.com/rehuony/sing-box-panel/internal/application"
	"github.com/rehuony/sing-box-panel/internal/configuration"
	"github.com/rehuony/sing-box-panel/internal/settings"
	"github.com/rehuony/sing-box-panel/internal/store"
)

// Use the release scenario's actual input and the public OpenAPI contract so
// changes to editable text, history or response envelopes are checked together.
func TestReleaseConfigurationFileContractSurvivesReopen(t *testing.T) {
	fixture, err := os.ReadFile(filepath.Join("..", "..", "scripts", "testdata", "release-configuration.json"))
	if err != nil {
		t.Fatal(err)
	}
	document, err := configuration.Parse(fixture)
	if err != nil {
		t.Fatal(err)
	}
	router, err := legacy.NewRouter(loadOpenAPIContract(t))
	if err != nil {
		t.Fatal(err)
	}
	value := settings.Defaults()
	value.DataDir = t.TempDir()
	value.Auth.Token = "openapi-response-test"
	databasePath := filepath.Join(value.DataDir, "panel.db")
	open := func() (*store.Store, *Handler) {
		t.Helper()
		database, err := store.Open(t.Context(), databasePath)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = database.Close() })
		return database, NewHandler(HandlerOptions{Settings: value, Commands: application.FromStoreWithSettings(database, value)})
	}
	database, handler := open()
	var expectedRevision int64
	var history application.CanonicalSnapshot
	for _, content := range []string{string(fixture), "{\n  \"log\": ", string(fixture)} {
		body, err := json.Marshal(application.ConfigurationFileWrite{Revision: expectedRevision, Content: content})
		if err != nil {
			t.Fatal(err)
		}
		response := serveConformingRequest(t, router, handler, http.MethodPut, "/api/v1/config/file", string(body), http.StatusOK, true, nil)
		var saved application.ConfigurationFile
		if err := json.Unmarshal(response.Body.Bytes(), &saved); err != nil {
			t.Fatal(err)
		}
		expectedRevision++
		valid := content == string(fixture)
		if saved.Revision != expectedRevision || saved.Content != content || saved.SyntaxValid != valid || (saved.CanonicalRevisionID != "") != valid {
			t.Fatalf("file save changed text or state at revision %d", expectedRevision)
		}
		serveConformingRequest(t, router, handler, http.MethodPut, "/api/v1/config/file", string(body), http.StatusPreconditionFailed, true, nil)
		if err := database.Close(); err != nil {
			t.Fatal(err)
		}
		database, handler = open()
		response = serveConformingRequest(t, router, handler, http.MethodGet, "/api/v1/config/file", "", http.StatusOK, true, nil)
		var persisted application.ConfigurationFile
		if err := json.Unmarshal(response.Body.Bytes(), &persisted); err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(persisted, saved) {
			t.Fatalf("editable file changed after reopening at revision %d", expectedRevision)
		}
		response = serveConformingRequest(t, router, handler, http.MethodGet, "/api/v1/config/canonical", "", http.StatusOK, true, nil)
		var snapshot application.CanonicalSnapshot
		if err := json.Unmarshal(response.Body.Bytes(), &snapshot); err != nil {
			t.Fatal(err)
		}
		digest := sha256.Sum256(document.CanonicalJSON())
		if snapshot.SchemaVersion != configuration.SchemaVersion || snapshot.Sequence != 1 ||
			!bytes.Equal(snapshot.Document, document.CanonicalJSON()) || snapshot.DocumentJSON != string(document.CanonicalJSON()) ||
			snapshot.SHA256 != hex.EncodeToString(digest[:]) {
			t.Fatal("immutable history does not match the lossless configuration contract")
		}
		if history.ID == "" {
			history = snapshot
		} else if !reflect.DeepEqual(snapshot, history) {
			t.Fatal("draft save or correction changed the existing immutable history")
		}
		if valid && saved.CanonicalRevisionID != history.ID {
			t.Fatal("valid saved file is not linked to its immutable history")
		}
	}
}
