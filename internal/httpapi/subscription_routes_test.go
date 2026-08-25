// SPDX-License-Identifier: GPL-3.0-or-later

package httpapi

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/rehuony/sing-box-panel/internal/application"
	"github.com/rehuony/sing-box-panel/internal/buildinfo"
	"github.com/rehuony/sing-box-panel/internal/canonical"
	"github.com/rehuony/sing-box-panel/internal/settings"
	"github.com/rehuony/sing-box-panel/internal/store"
)

func TestSubscriptionManagementHTTPCRUDStrictnessAndCAS(t *testing.T) {
	ctx := context.Background()
	database, app, handler := newSubscriptionHTTPServices(t, "")
	_ = database

	unauthenticated := httptest.NewRecorder()
	handler.ServeHTTP(unauthenticated, httptest.NewRequest(http.MethodGet, "/api/v1/subscription/channels", nil))
	if unauthenticated.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated status=%d body=%s", unauthenticated.Code, unauthenticated.Body.String())
	}

	ambiguous := authenticatedRequest(handler, http.MethodPost, "/api/v1/subscription/channels",
		`{"name":"public","format":"sing-box","enabled":true,"enabled":false}`, "")
	if ambiguous.Code != http.StatusUnprocessableEntity {
		t.Fatalf("ambiguous status=%d body=%s", ambiguous.Code, ambiguous.Body.String())
	}
	unknown := authenticatedRequest(handler, http.MethodPost, "/api/v1/subscription/channels",
		`{"name":"public","format":"sing-box","enabled":true,"future":true}`, "")
	if unknown.Code != http.StatusUnprocessableEntity {
		t.Fatalf("unknown field status=%d body=%s", unknown.Code, unknown.Body.String())
	}

	createdResponse := authenticatedRequest(handler, http.MethodPost, "/api/v1/subscription/channels",
		`{"name":"public","format":"sing-box","config":{},"enabled":true}`, "")
	if createdResponse.Code != http.StatusCreated || createdResponse.Header().Get("ETag") == "" {
		t.Fatalf("create status=%d etag=%q body=%s", createdResponse.Code, createdResponse.Header().Get("ETag"), createdResponse.Body.String())
	}
	var channel application.SubscriptionChannel
	if err := json.Unmarshal(createdResponse.Body.Bytes(), &channel); err != nil {
		t.Fatal(err)
	}

	listed := authenticatedRequest(handler, http.MethodGet, "/api/v1/subscription/channels", "", "")
	if listed.Code != http.StatusOK || !strings.Contains(listed.Body.String(), channel.ID) {
		t.Fatalf("list status=%d body=%s", listed.Code, listed.Body.String())
	}
	invalidQuery := authenticatedRequest(handler, http.MethodGet, "/api/v1/subscription/channels?future=true", "", "")
	if invalidQuery.Code != http.StatusBadRequest {
		t.Fatalf("invalid query status=%d body=%s", invalidQuery.Code, invalidQuery.Body.String())
	}
	show := authenticatedRequest(handler, http.MethodGet, "/api/v1/subscription/channels/"+channel.ID, "", "")
	if show.Code != http.StatusOK || show.Header().Get("ETag") != createdResponse.Header().Get("ETag") {
		t.Fatalf("show status=%d etag=%q body=%s", show.Code, show.Header().Get("ETag"), show.Body.String())
	}

	missingCAS := authenticatedRequest(handler, http.MethodPut, "/api/v1/subscription/channels/"+channel.ID,
		`{"name":"renamed","format":"loon","config":{},"enabled":false}`, "")
	if missingCAS.Code != http.StatusPreconditionRequired {
		t.Fatalf("missing CAS status=%d body=%s", missingCAS.Code, missingCAS.Body.String())
	}
	updated := authenticatedRequest(handler, http.MethodPut, "/api/v1/subscription/channels/"+channel.ID,
		`{"name":"renamed","format":"loon","config":{},"enabled":false}`, createdResponse.Header().Get("ETag"))
	if updated.Code != http.StatusOK || updated.Header().Get("ETag") == createdResponse.Header().Get("ETag") {
		t.Fatalf("update status=%d etag=%q body=%s", updated.Code, updated.Header().Get("ETag"), updated.Body.String())
	}
	stale := authenticatedRequest(handler, http.MethodPut, "/api/v1/subscription/channels/"+channel.ID,
		`{"name":"stale","format":"loon","config":{},"enabled":false}`, createdResponse.Header().Get("ETag"))
	if stale.Code != http.StatusPreconditionFailed {
		t.Fatalf("stale status=%d body=%s", stale.Code, stale.Body.String())
	}

	sourceResponse := authenticatedRequest(handler, http.MethodPost, "/api/v1/subscription/sources",
		`{"name":"attached","source_kind":"local","config":{},"enabled":true}`, "")
	if sourceResponse.Code != http.StatusCreated {
		t.Fatalf("source create status=%d body=%s", sourceResponse.Code, sourceResponse.Body.String())
	}
	var source application.SubscriptionSource
	if err := json.Unmarshal(sourceResponse.Body.Bytes(), &source); err != nil {
		t.Fatal(err)
	}
	snapshot := authenticatedRequest(handler, http.MethodPut, "/api/v1/subscription/sources/"+source.ID+"/snapshot",
		`{"latest_snapshot":{"nodes":[{"tag":"attached"}]}}`, sourceResponse.Header().Get("ETag"))
	if snapshot.Code != http.StatusOK || !strings.Contains(snapshot.Body.String(), `"attached"`) {
		t.Fatalf("snapshot status=%d body=%s", snapshot.Code, snapshot.Body.String())
	}
	updateSource := authenticatedRequest(handler, http.MethodPut, "/api/v1/subscription/sources/"+source.ID,
		`{"name":"attached remote","source_kind":"remote","config":{"url":"https://example.test/sub"},"enabled":false}`,
		snapshot.Header().Get("ETag"))
	if updateSource.Code != http.StatusOK || !strings.Contains(updateSource.Body.String(), `"attached remote"`) ||
		!strings.Contains(updateSource.Body.String(), `"latest_snapshot"`) {
		t.Fatalf("source update status=%d body=%s", updateSource.Code, updateSource.Body.String())
	}
	showSource := authenticatedRequest(handler, http.MethodGet, "/api/v1/subscription/sources/"+source.ID, "", "")
	if showSource.Code != http.StatusOK || showSource.Header().Get("ETag") != updateSource.Header().Get("ETag") {
		t.Fatalf("source show status=%d etag=%q body=%s", showSource.Code, showSource.Header().Get("ETag"), showSource.Body.String())
	}
	deleteSource := authenticatedRequest(handler, http.MethodDelete, "/api/v1/subscription/sources/"+source.ID,
		"", updateSource.Header().Get("ETag"))
	if deleteSource.Code != http.StatusNoContent {
		t.Fatalf("source delete status=%d body=%s", deleteSource.Code, deleteSource.Body.String())
	}

	tokenResponse := authenticatedRequest(handler, http.MethodPost, "/api/v1/subscription/tokens",
		`{"channel_id":"`+channel.ID+`"}`, "")
	if tokenResponse.Code != http.StatusCreated {
		t.Fatalf("token create status=%d body=%s", tokenResponse.Code, tokenResponse.Body.String())
	}
	var token application.CreatedSubscriptionToken
	if err := json.Unmarshal(tokenResponse.Body.Bytes(), &token); err != nil {
		t.Fatal(err)
	}
	if token.Token == "" {
		t.Fatal("token plaintext missing from one-time create response")
	}
	tokens := authenticatedRequest(handler, http.MethodGet, "/api/v1/subscription/tokens", "", "")
	if tokens.Code != http.StatusOK || bytes.Contains(tokens.Body.Bytes(), []byte(token.Token)) {
		t.Fatalf("token list status=%d body=%s", tokens.Code, tokens.Body.String())
	}
	inUse := authenticatedRequest(handler, http.MethodDelete, "/api/v1/subscription/channels/"+channel.ID,
		"", updated.Header().Get("ETag"))
	if inUse.Code != http.StatusConflict {
		t.Fatalf("in-use delete status=%d body=%s", inUse.Code, inUse.Body.String())
	}

	// Cookie-authenticated writes are covered by the same CSRF/Origin boundary.
	login := httptest.NewRequest(http.MethodPost, "/api/v1/auth/session", strings.NewReader(`{"token":"correct-management-token"}`))
	loginResponse := httptest.NewRecorder()
	handler.ServeHTTP(loginResponse, login)
	cookies := loginResponse.Result().Cookies()
	withoutCSRF := httptest.NewRequest(http.MethodPost, "/api/v1/subscription/sources", strings.NewReader(`{"name":"blocked","source_kind":"local","enabled":false}`))
	for _, cookie := range cookies {
		withoutCSRF.AddCookie(cookie)
	}
	withoutCSRFResponse := httptest.NewRecorder()
	handler.ServeHTTP(withoutCSRFResponse, withoutCSRF)
	if withoutCSRFResponse.Code != http.StatusForbidden {
		t.Fatalf("write without CSRF status=%d body=%s", withoutCSRFResponse.Code, withoutCSRFResponse.Body.String())
	}

	if _, err := app.SubscriptionChannel(ctx, channel.ID); err != nil {
		t.Fatalf("management CRUD did not persist through application: %v", err)
	}
}

func TestSubscriptionPreviewHTTPUsesCheckedImmutableArtifact(t *testing.T) {
	_, _, handler, channel, startup := newSubscriptionPublicationHTTPFixture(t, "")
	preview := authenticatedRequest(handler, http.MethodPost,
		"/api/v1/subscription/channels/"+channel.ID+"/preview",
		`{"startup_artifact_id":"`+startup.ID+`"}`, "")
	if preview.Code != http.StatusOK || !strings.Contains(preview.Body.String(), `"node_count":1`) ||
		!strings.Contains(preview.Body.String(), `"startup_artifact_id":"`+startup.ID+`"`) {
		t.Fatalf("preview status=%d body=%s", preview.Code, preview.Body.String())
	}
}

func TestPublicSubscriptionHTTPFrozenPublicationAndTokenLifecycle(t *testing.T) {
	ctx := context.Background()
	database, app, handler, channel, startup := newSubscriptionPublicationHTTPFixture(t, "/panel")
	created, err := app.CreateSubscriptionToken(ctx, application.CreateSubscriptionTokenRequest{ChannelID: channel.ID})
	if err != nil {
		t.Fatal(err)
	}

	beforeApply := publicSubscriptionRequest(handler, "/panel/sub/"+created.Token+"?format=sing-box")
	if beforeApply.Code != http.StatusServiceUnavailable || !strings.Contains(beforeApply.Body.String(), `"code":"not_applied"`) ||
		strings.Contains(beforeApply.Body.String(), created.Token) {
		t.Fatalf("before apply status=%d body=%s", beforeApply.Code, beforeApply.Body.String())
	}

	prepared, err := app.PrepareActivationBundle(ctx, startup.ID, store.MonitoringProcessOnly)
	if err != nil {
		t.Fatal(err)
	}
	applySubscriptionHTTPBundle(t, database, app, prepared.Bundle.ID)

	served := publicSubscriptionRequest(handler, "/panel/sub/"+created.Token+"?format=sing-box")
	if served.Code != http.StatusOK || served.Header().Get("Content-Type") != "application/json" ||
		!strings.Contains(served.Body.String(), "publish.example") || served.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("served status=%d content-type=%q body=%s", served.Code, served.Header().Get("Content-Type"), served.Body.String())
	}
	omittedBoundFormat := publicSubscriptionRequest(handler, "/panel/sub/"+created.Token)
	if omittedBoundFormat.Code != http.StatusOK {
		t.Fatalf("bound token without format status=%d body=%s", omittedBoundFormat.Code, omittedBoundFormat.Body.String())
	}
	unbound, err := app.CreateSubscriptionToken(ctx, application.CreateSubscriptionTokenRequest{})
	if err != nil {
		t.Fatal(err)
	}
	unboundWithoutFormat := publicSubscriptionRequest(handler, "/panel/sub/"+unbound.Token)
	if unboundWithoutFormat.Code != http.StatusBadRequest {
		t.Fatalf("unbound token without format status=%d body=%s", unboundWithoutFormat.Code, unboundWithoutFormat.Body.String())
	}
	unboundWithFormat := publicSubscriptionRequest(handler, "/panel/sub/"+unbound.Token+"?format=sing-box")
	if unboundWithFormat.Code != http.StatusOK {
		t.Fatalf("unbound token with format status=%d body=%s", unboundWithFormat.Code, unboundWithFormat.Body.String())
	}
	mismatch := publicSubscriptionRequest(handler, "/panel/sub/"+created.Token+"?format=mihomo")
	if mismatch.Code != http.StatusNotFound || strings.Contains(mismatch.Body.String(), created.Token) {
		t.Fatalf("format mismatch status=%d body=%s", mismatch.Code, mismatch.Body.String())
	}
	for _, target := range []string{
		"/panel/sub/" + created.Token + "?format=future",
		"/panel/sub/" + created.Token + "?format=sing-box&format=loon",
		"/panel/sub/" + created.Token + "?unexpected=true",
	} {
		response := publicSubscriptionRequest(handler, target)
		if response.Code != http.StatusBadRequest || strings.Contains(response.Body.String(), created.Token) {
			t.Fatalf("strict query %q status=%d body=%s", target, response.Code, response.Body.String())
		}
	}

	// Mutating the current channel cannot alter the already-applied frozen bytes.
	update := authenticatedRequest(handler, http.MethodPut,
		"/panel/api/v1/subscription/channels/"+channel.ID,
		`{"name":"sing-box","format":"sing-box","config":{"exclude_tags":["publish"]},"enabled":true}`,
		subscriptionETag(channel.UpdatedAt))
	if update.Code != http.StatusOK {
		t.Fatalf("channel update status=%d body=%s", update.Code, update.Body.String())
	}
	stillFrozen := publicSubscriptionRequest(handler, "/panel/sub/"+created.Token+"?format=sing-box")
	if stillFrozen.Code != http.StatusOK || !bytes.Equal(stillFrozen.Body.Bytes(), served.Body.Bytes()) {
		t.Fatalf("mutable channel changed applied publication: status=%d body=%s", stillFrozen.Code, stillFrozen.Body.String())
	}

	rotationResponse := authenticatedRequest(handler, http.MethodPost,
		"/panel/api/v1/subscription/tokens/"+created.Metadata.ID+"/rotate", `{}`, "")
	if rotationResponse.Code != http.StatusCreated {
		t.Fatalf("rotate status=%d body=%s", rotationResponse.Code, rotationResponse.Body.String())
	}
	var rotation application.SubscriptionTokenRotation
	if err := json.Unmarshal(rotationResponse.Body.Bytes(), &rotation); err != nil {
		t.Fatal(err)
	}
	oldAfterRotate := publicSubscriptionRequest(handler, "/panel/sub/"+created.Token+"?format=sing-box")
	unknown := publicSubscriptionRequest(handler, "/panel/sub/unknown-public-token?format=sing-box")
	if oldAfterRotate.Code != http.StatusNotFound || unknown.Code != http.StatusNotFound ||
		!sameSafePublicProblem(oldAfterRotate.Body.Bytes(), unknown.Body.Bytes()) ||
		bytes.Contains(oldAfterRotate.Body.Bytes(), []byte(created.Token)) {
		t.Fatalf("old=%s unknown=%s", oldAfterRotate.Body.String(), unknown.Body.String())
	}
	newToken := publicSubscriptionRequest(handler, "/panel/sub/"+rotation.Token+"?format=sing-box")
	if newToken.Code != http.StatusOK {
		t.Fatalf("replacement token status=%d body=%s", newToken.Code, newToken.Body.String())
	}
	revokedResponse := authenticatedRequest(handler, http.MethodPost,
		"/panel/api/v1/subscription/tokens/"+rotation.Created.ID+"/revoke", "", "")
	if revokedResponse.Code != http.StatusOK {
		t.Fatalf("revoke status=%d body=%s", revokedResponse.Code, revokedResponse.Body.String())
	}
	revoked := publicSubscriptionRequest(handler, "/panel/sub/"+rotation.Token+"?format=sing-box")
	if revoked.Code != http.StatusNotFound || !sameSafePublicProblem(revoked.Body.Bytes(), unknown.Body.Bytes()) ||
		bytes.Contains(revoked.Body.Bytes(), []byte(rotation.Token)) {
		t.Fatalf("revoked=%s unknown=%s", revoked.Body.String(), unknown.Body.String())
	}

	const expiredPlaintext = "expired-public-token"
	expiredDigest := sha256.Sum256([]byte(expiredPlaintext))
	expiresAt := time.Now().UTC().Add(-time.Minute)
	if _, err := database.CreateSubscriptionToken(ctx, store.SubscriptionToken{
		ID: "token_http_expired", TokenSHA256: hex.EncodeToString(expiredDigest[:]),
		ChannelID: channel.ID, ExpiresAt: &expiresAt, CreatedAt: expiresAt.Add(-time.Hour),
	}); err != nil {
		t.Fatal(err)
	}
	expired := publicSubscriptionRequest(handler, "/panel/sub/"+expiredPlaintext+"?format=sing-box")
	if expired.Code != http.StatusNotFound || !sameSafePublicProblem(expired.Body.Bytes(), unknown.Body.Bytes()) ||
		bytes.Contains(expired.Body.Bytes(), []byte(expiredPlaintext)) {
		t.Fatalf("expired=%s unknown=%s", expired.Body.String(), unknown.Body.String())
	}

	outsideBase := publicSubscriptionRequest(handler, "/sub/"+rotation.Token+"?format=sing-box")
	if outsideBase.Code != http.StatusNotFound {
		t.Fatalf("outside base path status=%d body=%s", outsideBase.Code, outsideBase.Body.String())
	}
}

func newSubscriptionHTTPServices(t *testing.T, basePath string) (*store.Store, *application.Application, *Handler) {
	t.Helper()
	ctx := context.Background()
	database, err := store.Open(ctx, filepath.Join(t.TempDir(), "panel.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	value := settings.Defaults(t.TempDir() + "/setting.json")
	value.DataDir = t.TempDir()
	value.Auth.Token = "correct-management-token"
	value.Server.BasePath = basePath
	if err := value.Validate(); err != nil {
		t.Fatal(err)
	}
	app := application.FromStore(database)
	handler := NewHandler(HandlerOptions{
		Settings: value, Build: buildinfo.Info{Version: "test"}, Commands: app,
	})
	return database, app, handler
}

func newSubscriptionPublicationHTTPFixture(
	t *testing.T,
	basePath string,
) (*store.Store, *application.Application, *Handler, application.SubscriptionChannel, store.StartupArtifact) {
	t.Helper()
	ctx := context.Background()
	database, app, handler := newSubscriptionHTTPServices(t, basePath)
	now := time.Now().UTC()
	core := store.CoreArtifact{
		ID: "core-http-publication", ExactVersion: "1.13.19", OperatingSystem: "linux",
		Architecture: "amd64", Variant: "plain", SourceKind: store.CoreArtifactSourceOfficial,
		RepositoryID: 1, ReleaseID: 2, AssetID: 3, ArchiveSHA256: strings.Repeat("a", 64),
		BinarySHA256: strings.Repeat("b", 64), BinaryPath: "/secure/core-http-publication/sing-box",
		ReportedVersion: "1.13.19", FeatureFingerprint: json.RawMessage(`{"features":[]}`),
		VerificationState: store.CoreArtifactVerified, CreatedAt: now,
	}
	if _, err := database.UpsertCoreArtifact(ctx, core); err != nil {
		t.Fatal(err)
	}
	revision, err := app.ReplaceCanonical(ctx, "", canonical.Empty().CanonicalJSON())
	if err != nil {
		t.Fatal(err)
	}
	startup, err := database.CreateStartupArtifact(ctx, store.StartupArtifact{
		ID: "startup-http-publication", Kind: store.StartupArtifactManual,
		CanonicalRevisionID: revision.Revision.ID, ExactCoreVersion: core.ExactVersion,
		RendererVersion: "manual-v1", CoreArtifactID: core.ID,
		ConfigBytes: []byte(`{"outbounds":[{"type":"shadowsocks","tag":"publish","server":"publish.example","server_port":443,"method":"aes-256-gcm","password":"secret"}]}`),
		CreatedAt:   now.Add(time.Second),
	})
	if err != nil {
		t.Fatal(err)
	}
	startup, err = database.CompleteStartupArtifactCheck(ctx, startup.ID, true, nil, now.Add(2*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	channel, err := app.CreateSubscriptionChannel(ctx, application.CreateSubscriptionChannelRequest{
		Name: "sing-box", Format: store.SubscriptionFormatSingBox,
		Config: json.RawMessage(`{}`), Enabled: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	return database, app, handler, channel, startup
}

func applySubscriptionHTTPBundle(t *testing.T, database *store.Store, app *application.Application, bundleID string) {
	t.Helper()
	ctx := context.Background()
	queued, err := app.QueueRuntimeApply(ctx, bundleID)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().Add(10 * time.Minute)
	claimed, err := database.ClaimTask(ctx, store.ClaimTaskInput{
		Lane: store.TaskLaneRuntime, LeaseOwner: "subscription-http-test", Now: now, LeaseDuration: time.Minute,
	})
	if err != nil || claimed == nil || claimed.ID != queued.ID {
		t.Fatalf("claim apply task=%+v err=%v", claimed, err)
	}
	if _, err := database.CompleteTask(ctx, claimed.ID, claimed.LeaseOwner, now.Add(time.Second), store.TaskCompletion{
		Succeeded: true, Result: json.RawMessage(`{"healthy":true}`),
	}); err != nil {
		t.Fatal(err)
	}
}

func publicSubscriptionRequest(handler http.Handler, target string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(http.MethodGet, target, nil)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	return response
}

func sameSafePublicProblem(left, right []byte) bool {
	var leftProblem, rightProblem Problem
	if json.Unmarshal(left, &leftProblem) != nil || json.Unmarshal(right, &rightProblem) != nil {
		return false
	}
	leftProblem.RequestID = ""
	rightProblem.RequestID = ""
	leftEncoded, _ := json.Marshal(leftProblem)
	rightEncoded, _ := json.Marshal(rightProblem)
	return bytes.Equal(leftEncoded, rightEncoded)
}
