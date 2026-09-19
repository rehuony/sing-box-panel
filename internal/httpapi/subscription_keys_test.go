package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/rehuony/sing-box-panel/internal/application"
	"github.com/rehuony/sing-box-panel/internal/store"
)

func TestChannelScopedKeysShareDownloadLimitAcrossChannels(t *testing.T) {
	ctx := context.Background()
	db, app, handler, channel, startup := newSubscriptionPublicationHTTPFixture(t, "")
	response := authenticatedRequest(handler, http.MethodPost, "/api/v1/subscription/tokens", `{"label":"Shared key","download_limit":2}`, "")
	if response.Code != http.StatusCreated {
		t.Fatalf("create: %d %s", response.Code, response.Body.String())
	}
	var key application.CreatedSubscriptionToken
	if err := json.Unmarshal(response.Body.Bytes(), &key); err != nil {
		t.Fatal(err)
	}
	if key.Metadata.UserID != "" || strings.Contains(response.Body.String(), "token_sha256") {
		t.Fatalf("unexpected metadata: %s", response.Body.String())
	}
	path := "/sub/" + key.Token + "/" + channel.ID
	if failed := publicSubscriptionRequest(handler, path); failed.Code != http.StatusServiceUnavailable {
		t.Fatalf("before apply: %d", failed.Code)
	}
	unchanged, err := app.SubscriptionToken(ctx, key.Metadata.ID)
	if err != nil || unchanged.BodyResponseCount != 0 {
		t.Fatalf("failed response consumed quota: %+v %v", unchanged, err)
	}
	prepared, err := app.PrepareActivationBundle(ctx, startup.ID, store.MonitoringProcessOnly)
	if err != nil {
		t.Fatal(err)
	}
	applySubscriptionHTTPBundle(t, db, app, prepared.Bundle.ID)
	second, err := app.CreateSubscriptionChannel(ctx, application.CreateSubscriptionChannelRequest{Name: "Second", Format: store.SubscriptionFormatSingBox, PublicHost: "second.example", Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	first := publicSubscriptionRequest(handler, path)
	if first.Code != http.StatusOK || !strings.Contains(first.Body.String(), "publish.example") {
		t.Fatalf("download: %d %s", first.Code, first.Body.String())
	}
	if conditional := publicSubscriptionConditionalRequest(handler, path, first.Header().Get("ETag")); conditional.Code != http.StatusNotModified {
		t.Fatalf("conditional: %d %s", conditional.Code, conditional.Body.String())
	}
	if secondResponse := publicSubscriptionRequest(handler, "/sub/"+key.Token+"/"+second.ID); secondResponse.Code != http.StatusOK {
		t.Fatalf("second channel: %d %s", secondResponse.Code, secondResponse.Body.String())
	}
	if exhausted := publicSubscriptionRequest(handler, path); exhausted.Code != http.StatusNotFound {
		t.Fatalf("exhausted: %d %s", exhausted.Code, exhausted.Body.String())
	}
	used, err := app.SubscriptionToken(ctx, key.Metadata.ID)
	if err != nil || used.Active || used.BodyResponseCount != 2 || used.SuccessfulRequestCount != 3 {
		t.Fatalf("shared counters: %+v %v", used, err)
	}
	preview := authenticatedRequest(handler, http.MethodPost, "/api/v1/subscription/channels/"+channel.ID+"/preview", `{}`, "")
	if preview.Code != http.StatusOK || !strings.Contains(preview.Body.String(), `"node_count":1`) {
		t.Fatalf("channel preview: %d %s", preview.Code, preview.Body.String())
	}
	for _, body := range []string{`{"label":"bad","download_limit":0}`, `{"label":"bad","download_limit":1.5}`, `{"label":"bad","download_limit":1000000001}`} {
		invalid := authenticatedRequest(handler, http.MethodPost, "/api/v1/subscription/tokens", body, "")
		if invalid.Code != http.StatusUnprocessableEntity {
			t.Fatalf("invalid quota: %d %s", invalid.Code, invalid.Body.String())
		}
	}
}
