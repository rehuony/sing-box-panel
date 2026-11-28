package httpapi

import (
	"bytes"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/rehuony/sing-box-panel/internal/sing-box-panel/auth"
	"github.com/rehuony/sing-box-panel/internal/sing-box-panel/backup"
	"github.com/rehuony/sing-box-panel/internal/sing-box-panel/settings"
	"github.com/rehuony/sing-box-panel/internal/sing-box-panel/store"
	"github.com/rehuony/sing-box-panel/internal/sing-box-panel/testutil"
)

// TestPanelBackupsCreateDownloadStageAndRequestRestore walks the backup
// routes: a passphrase too short to protect a backup is refused; a manual
// backup is listed and downloaded; it stages from the directory and from an
// upload; applying the staged restore backs up the current state first and
// marks the restore for the next start.
func TestPanelBackupsCreateDownloadStageAndRequestRestore(t *testing.T) {
	dataDir := t.TempDir()
	db, err := store.Open(t.Context(), filepath.Join(dataDir, store.PanelFile))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	value := settings.Defaults()
	value.DataDir = dataDir
	cfg := settingsFileFixture(t, value)
	handler := newTestHandler(t, HandlerOptions{Settings: cfg, Services: newTestServices(t, db, cfg)})
	login := loginWithPassword(t, handler, testutil.AdminPassword)
	var session auth.AuthSession
	if err := json.Unmarshal(login.Body.Bytes(), &session); err != nil {
		t.Fatal(err)
	}
	send := func(method, path, contentType string, body []byte) *httptest.ResponseRecorder {
		request := httptest.NewRequest(method, "/api/v1"+path, bytes.NewReader(body))
		request.AddCookie(login.Result().Cookies()[0])
		request.Header.Set("Content-Type", contentType)
		request.Header.Set("X-CSRF-Token", session.CSRFToken)
		request.Header.Set("Origin", "http://example.com")
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		return response
	}
	anonymous := httptest.NewRecorder()
	handler.ServeHTTP(anonymous, httptest.NewRequest(http.MethodGet, "/api/v1/panel/backups", nil))
	if anonymous.Code != http.StatusUnauthorized {
		t.Fatalf("anonymous list: %d", anonymous.Code)
	}
	forged := httptest.NewRequest(http.MethodPost, "/api/v1/panel/backups", strings.NewReader(`{}`))
	forged.AddCookie(login.Result().Cookies()[0])
	forged.Header.Set("Content-Type", "application/json")
	refused := httptest.NewRecorder()
	handler.ServeHTTP(refused, forged)
	if refused.Code != http.StatusForbidden {
		t.Fatalf("create without CSRF: %d", refused.Code)
	}
	// A passphrase is counted in characters: seven of them are too few
	// however many bytes they take.
	for _, body := range []string{`{"categories":["settings"],"passphrase":"short"}`, `{"categories":["settings"],"passphrase":"密码密码密码密"}`,
		`{"categories":[]}`, `{"categories":["sessions"]}`} {
		if got := send(http.MethodPost, "/panel/backups", "application/json", []byte(body)); got.Code != http.StatusUnprocessableEntity {
			t.Fatalf("create %s: %d %s", body, got.Code, got.Body.String())
		}
	}
	created := send(http.MethodPost, "/panel/backups", "application/json", []byte(`{"categories":["logs","settings","nodes"]}`))
	var info backup.Info
	if created.Code != http.StatusCreated || json.Unmarshal(created.Body.Bytes(), &info) != nil || info.Kind != backup.KindManual ||
		!slices.Equal(info.Categories, []backup.Category{backup.CategorySettings, backup.CategoryNodes, backup.CategoryLogs}) {
		t.Fatalf("create: %d %s", created.Code, created.Body.String())
	}
	var list PanelBackupList
	if got := send(http.MethodGet, "/panel/backups", "", nil); got.Code != http.StatusOK || json.Unmarshal(got.Body.Bytes(), &list) != nil || len(list.Items) != 1 {
		t.Fatalf("list: %d %s", got.Code, got.Body.String())
	}
	download := send(http.MethodGet, "/panel/backups/"+info.Name, "", nil)
	if download.Code != http.StatusOK || !strings.Contains(download.Header().Get("Content-Disposition"), info.Name) || int64(download.Body.Len()) != info.Size {
		t.Fatalf("download: %d %v", download.Code, download.Header())
	}
	var staged backup.Staged
	if got := send(http.MethodPost, "/panel/backups/"+info.Name+"/restore", "application/json", []byte(`{}`)); got.Code != http.StatusOK || json.Unmarshal(got.Body.Bytes(), &staged) != nil || !staged.Manifest.Holds(backup.CategoryLogs) {
		t.Fatalf("stage stored: %d %s", got.Code, got.Body.String())
	}
	var form bytes.Buffer
	writer := multipart.NewWriter(&form)
	part, err := writer.CreateFormFile("archive", info.Name)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = part.Write(download.Body.Bytes())
	_ = writer.Close()
	if got := send(http.MethodPost, "/panel/restores", writer.FormDataContentType(), form.Bytes()); got.Code != http.StatusOK || json.Unmarshal(got.Body.Bytes(), &staged) != nil {
		t.Fatalf("stage upload: %d %s", got.Code, got.Body.String())
	}
	var plan backup.Plan
	if got := send(http.MethodPost, "/panel/restores/"+staged.ID+"/plan", "application/json", []byte(`{"categories":["nodes"]}`)); got.Code != http.StatusOK ||
		json.Unmarshal(got.Body.Bytes(), &plan) != nil || plan.SignsOut {
		t.Fatalf("plan: %d %s", got.Code, got.Body.String())
	}
	if got := send(http.MethodPost, "/panel/restores/"+staged.ID+"/plan", "application/json", []byte(`{"categories":["keys"]}`)); got.Code != http.StatusUnprocessableEntity {
		t.Fatalf("plan of a category the backup lacks: %d %s", got.Code, got.Body.String())
	}
	var requested PanelRestoreRequested
	if got := send(http.MethodPost, "/panel/restores/"+staged.ID+"/apply", "application/json", []byte(`{"take_service":false,"categories":["settings","nodes"]}`)); got.Code != http.StatusAccepted ||
		json.Unmarshal(got.Body.Bytes(), &requested) != nil || requested.Restarting || requested.Before.Kind != backup.KindPreRestore {
		t.Fatalf("apply: %d %s", got.Code, got.Body.String())
	}
	testutil.AssertPathExists(t, filepath.Join(dataDir, backup.DirectoryName, "restore.pending"))
}
