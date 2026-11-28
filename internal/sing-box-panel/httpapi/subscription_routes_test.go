// SPDX-License-Identifier: GPL-3.0-or-later

package httpapi

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/rehuony/sing-box-panel/internal/buildinfo"
	"github.com/rehuony/sing-box-panel/internal/sing-box-panel/app"
	"github.com/rehuony/sing-box-panel/internal/sing-box-panel/settings"
	"github.com/rehuony/sing-box-panel/internal/sing-box-panel/store"
	"github.com/rehuony/sing-box-panel/internal/sing-box-panel/subscription"
	"github.com/rehuony/sing-box-panel/internal/sing-box-panel/testutil"
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

	ambiguous := authenticatedRequest(t, handler, http.MethodPost, "/api/v1/subscription/channels",
		`{"name":"public","format":"sing-box","enabled":true,"enabled":false}`, "")
	if ambiguous.Code != http.StatusUnprocessableEntity {
		t.Fatalf("ambiguous status=%d body=%s", ambiguous.Code, ambiguous.Body.String())
	}
	unknown := authenticatedRequest(t, handler, http.MethodPost, "/api/v1/subscription/channels",
		`{"name":"public","format":"sing-box","enabled":true,"future":true}`, "")
	if unknown.Code != http.StatusUnprocessableEntity {
		t.Fatalf("unknown field status=%d body=%s", unknown.Code, unknown.Body.String())
	}

	createdResponse := authenticatedRequest(t, handler, http.MethodPost, "/api/v1/subscription/channels",
		`{"name":"public","format":"sing-box","config":`+emptyChannelConfig+`,"enabled":true}`, "")
	if createdResponse.Code != http.StatusCreated || createdResponse.Header().Get("ETag") == "" {
		t.Fatalf("create status=%d etag=%q body=%s", createdResponse.Code, createdResponse.Header().Get("ETag"), createdResponse.Body.String())
	}
	var channel subscription.SubscriptionChannel
	if err := json.Unmarshal(createdResponse.Body.Bytes(), &channel); err != nil {
		t.Fatal(err)
	}

	listed := authenticatedRequest(t, handler, http.MethodGet, "/api/v1/subscription/channels", "", "")
	if listed.Code != http.StatusOK || !strings.Contains(listed.Body.String(), channel.ID) {
		t.Fatalf("list status=%d body=%s", listed.Code, listed.Body.String())
	}
	invalidQuery := authenticatedRequest(t, handler, http.MethodGet, "/api/v1/subscription/channels?future=true", "", "")
	if invalidQuery.Code != http.StatusBadRequest {
		t.Fatalf("invalid query status=%d body=%s", invalidQuery.Code, invalidQuery.Body.String())
	}
	show := authenticatedRequest(t, handler, http.MethodGet, "/api/v1/subscription/channels/"+channel.ID, "", "")
	if show.Code != http.StatusOK || show.Header().Get("ETag") != createdResponse.Header().Get("ETag") {
		t.Fatalf("show status=%d etag=%q body=%s", show.Code, show.Header().Get("ETag"), show.Body.String())
	}

	missingCAS := authenticatedRequest(t, handler, http.MethodPut, "/api/v1/subscription/channels/"+channel.ID,
		`{"name":"renamed","format":"loon","config":`+emptyChannelConfig+`,"enabled":false}`, "")
	if missingCAS.Code != http.StatusPreconditionRequired {
		t.Fatalf("missing CAS status=%d body=%s", missingCAS.Code, missingCAS.Body.String())
	}
	updated := authenticatedRequest(t, handler, http.MethodPut, "/api/v1/subscription/channels/"+channel.ID,
		`{"name":"renamed","format":"loon","config":`+emptyChannelConfig+`,"enabled":false}`, createdResponse.Header().Get("ETag"))
	if updated.Code != http.StatusOK || updated.Header().Get("ETag") == createdResponse.Header().Get("ETag") {
		t.Fatalf("update status=%d etag=%q body=%s", updated.Code, updated.Header().Get("ETag"), updated.Body.String())
	}
	stale := authenticatedRequest(t, handler, http.MethodPut, "/api/v1/subscription/channels/"+channel.ID,
		`{"name":"stale","format":"loon","config":`+emptyChannelConfig+`,"enabled":false}`, createdResponse.Header().Get("ETag"))
	if stale.Code != http.StatusPreconditionFailed {
		t.Fatalf("stale status=%d body=%s", stale.Code, stale.Body.String())
	}

	userResponse := authenticatedRequest(t, handler, http.MethodPost, "/api/v1/subscription/users",
		`{"name":"primary user","description":"HTTP fixture","enabled":true}`, "")
	if userResponse.Code != http.StatusCreated || userResponse.Header().Get("ETag") == "" {
		t.Fatalf("user create status=%d body=%s", userResponse.Code, userResponse.Body.String())
	}
	var user subscription.SubscriptionUser
	if err := json.Unmarshal(userResponse.Body.Bytes(), &user); err != nil {
		t.Fatal(err)
	}
	emptyGrants := authenticatedRequest(t, handler, http.MethodGet,
		"/api/v1/subscription/users/"+user.ID+"/grants", "", "")
	if emptyGrants.Code != http.StatusOK || !strings.Contains(emptyGrants.Body.String(), `"grants":[]`) {
		t.Fatalf("user grants status=%d body=%s", emptyGrants.Code, emptyGrants.Body.String())
	}

	tokenResponse := authenticatedRequest(t, handler, http.MethodPost, "/api/v1/subscription/tokens",
		`{"user_id":"`+user.ID+`","label":"primary"}`, "")
	if tokenResponse.Code != http.StatusCreated {
		t.Fatalf("token create status=%d body=%s", tokenResponse.Code, tokenResponse.Body.String())
	}
	var token subscription.CreatedSubscriptionToken
	if err := json.Unmarshal(tokenResponse.Body.Bytes(), &token); err != nil {
		t.Fatal(err)
	}
	if token.Token == "" {
		t.Fatal("token plaintext missing from one-time create response")
	}
	secretPath := "/api/v1/subscription/tokens/" + token.Metadata.ID + "/secret"
	secret := authenticatedRequest(t, handler, http.MethodGet, secretPath, "", "")
	var revealed subscription.SubscriptionTokenSecret
	if secret.Code != http.StatusOK || secret.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("secret response status=%d cache=%q", secret.Code, secret.Header().Get("Cache-Control"))
	}
	if err := json.Unmarshal(secret.Body.Bytes(), &revealed); err != nil || revealed.Token != token.Token {
		t.Fatal("secret did not match issued token")
	}
	unauthorizedSecret := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, secretPath, nil)
	request.Header.Set("Authorization", "Bearer "+token.Token)
	handler.ServeHTTP(unauthorizedSecret, request)
	if unauthorizedSecret.Code != http.StatusUnauthorized || bytes.Contains(unauthorizedSecret.Body.Bytes(), []byte(token.Token)) {
		t.Fatal("subscription token must not authorize secret reads")
	}
	metadata := authenticatedRequest(t, handler, http.MethodGet, "/api/v1/subscription/tokens/"+token.Metadata.ID, "", "")
	if metadata.Code != http.StatusOK || bytes.Contains(metadata.Body.Bytes(), []byte(token.Token)) {
		t.Fatal("ordinary token read leaked its secret")
	}
	tokens := authenticatedRequest(t, handler, http.MethodGet, "/api/v1/subscription/tokens", "", "")
	if tokens.Code != http.StatusOK || bytes.Contains(tokens.Body.Bytes(), []byte(token.Token)) {
		t.Fatalf("token list status=%d body=%s", tokens.Code, tokens.Body.String())
	}
	deleted := authenticatedRequest(t, handler, http.MethodDelete, "/api/v1/subscription/channels/"+channel.ID,
		"", updated.Header().Get("ETag"))
	if deleted.Code != http.StatusNoContent {
		t.Fatalf("delete with global token status=%d body=%s", deleted.Code, deleted.Body.String())
	}

	// Cookie-authenticated writes are covered by the same CSRF/Origin boundary.
	login := httptest.NewRequest(http.MethodPost, "/api/v1/auth/session", strings.NewReader(`{"email":"admin@example.com","password":"test-administrator-password"}`))
	login.Header.Set("Content-Type", "application/json")
	login.Header.Set("Origin", "http://example.com")
	loginResponse := httptest.NewRecorder()
	handler.ServeHTTP(loginResponse, login)
	cookies := loginResponse.Result().Cookies()
	withoutCSRF := httptest.NewRequest(http.MethodPost, "/api/v1/subscription/sources", strings.NewReader(`{"name":"blocked","kind":"file","filter":{"include":[],"exclude":[]},"content":"socks5://a.example:1080"}`))
	for _, cookie := range cookies {
		withoutCSRF.AddCookie(cookie)
	}
	withoutCSRFResponse := httptest.NewRecorder()
	handler.ServeHTTP(withoutCSRFResponse, withoutCSRF)
	if withoutCSRFResponse.Code != http.StatusForbidden {
		t.Fatalf("write without CSRF status=%d body=%s", withoutCSRFResponse.Code, withoutCSRFResponse.Body.String())
	}

	if _, err := app.Subscriptions.SubscriptionChannel(ctx, channel.ID); !errors.Is(err, store.ErrSubscriptionChannelNotFound) {
		t.Fatalf("management delete did not persist through application: %v", err)
	}
}

func TestSubscriptionRuleSetIconsAndOrderPerClient(t *testing.T) {
	_, _, handler := newSubscriptionHTTPServices(t, "")
	const base = "/api/v1/subscription/rule-sets"
	create := func(body string) subscription.SubscriptionRuleSet {
		t.Helper()
		response := authenticatedRequest(t, handler, http.MethodPost, base, body, "")
		var set subscription.SubscriptionRuleSet
		if response.Code != http.StatusCreated || json.Unmarshal(response.Body.Bytes(), &set) != nil {
			t.Fatalf("create status=%d body=%s", response.Code, response.Body.String())
		}
		return set
	}
	zeta := create(`{"client":"mihomo","name":"Zeta","rules":[]}`)
	alpha := create(`{"client":"mihomo","name":"Alpha","icon":"shield-ban","rules":[]}`)
	other := create(`{"client":"sing-box","name":"Other","rules":[]}`)
	if zeta.Icon != "layers" || alpha.Icon != "shield-ban" {
		t.Fatalf("icons = %q, %q", zeta.Icon, alpha.Icon)
	}
	if bad := authenticatedRequest(t, handler, http.MethodPost, base, `{"client":"mihomo","name":"Bad","icon":"Not An Icon","rules":[]}`, ""); bad.Code != http.StatusUnprocessableEntity ||
		!strings.Contains(bad.Body.String(), "icon: invalid_icon") {
		t.Fatalf("invalid icon status=%d body=%s", bad.Code, bad.Body.String())
	}
	order := func() []string {
		t.Helper()
		var list subscription.SubscriptionRuleSetList
		if response := authenticatedRequest(t, handler, http.MethodGet, base+"?client=mihomo", "", ""); json.Unmarshal(response.Body.Bytes(), &list) != nil {
			t.Fatalf("list body=%s", response.Body.String())
		}
		ids := []string{}
		for _, set := range list.Items {
			ids = append(ids, set.ID)
		}
		return ids
	}
	// A new set joins the end of its client's order, not its place by name.
	if got := order(); !slices.Equal(got, []string{zeta.ID, alpha.ID}) {
		t.Fatalf("order after creating = %v", got)
	}
	saved := authenticatedRequest(t, handler, http.MethodPut, "/api/v1/subscription/rule-set-order",
		`{"client":"mihomo","ids":["`+alpha.ID+`","`+zeta.ID+`"]}`, "")
	if saved.Code != http.StatusNoContent || !slices.Equal(order(), []string{alpha.ID, zeta.ID}) {
		t.Fatalf("save order status=%d body=%s", saved.Code, saved.Body.String())
	}
	// An order that misses a set, or names another client's, is stale.
	for _, ids := range []string{`["` + alpha.ID + `"]`, `["` + alpha.ID + `","` + other.ID + `"]`} {
		stale := authenticatedRequest(t, handler, http.MethodPut, "/api/v1/subscription/rule-set-order", `{"client":"mihomo","ids":`+ids+`}`, "")
		if stale.Code != http.StatusConflict || !strings.Contains(stale.Body.String(), "subscription_rule_sets_changed") {
			t.Fatalf("stale order %s status=%d body=%s", ids, stale.Code, stale.Body.String())
		}
	}
	// An update without an icon keeps it.
	updated := authenticatedRequest(t, handler, http.MethodPut, base+"/"+alpha.ID, `{"name":"Alpha","rules":[]}`, `"`+alpha.UpdatedAt.Format(time.RFC3339Nano)+`"`)
	var kept subscription.SubscriptionRuleSet
	if updated.Code != http.StatusOK || json.Unmarshal(updated.Body.Bytes(), &kept) != nil || kept.Icon != "shield-ban" {
		t.Fatalf("update status=%d body=%s", updated.Code, updated.Body.String())
	}
}

func TestSubscriptionRuleSetHTTPCRUDAndChannelRoutes(t *testing.T) {
	_, _, handler := newSubscriptionHTTPServices(t, "")
	const base = "/api/v1/subscription/rule-sets"
	created := authenticatedRequest(t, handler, http.MethodPost, base,
		`{"client":"mihomo","name":"Streaming","rules":[{"kind":"domain","value":"a.example"}]}`, "")
	if created.Code != http.StatusCreated || created.Header().Get("ETag") == "" {
		t.Fatalf("create status=%d body=%s", created.Code, created.Body.String())
	}
	var set subscription.SubscriptionRuleSet
	if err := json.Unmarshal(created.Body.Bytes(), &set); err != nil {
		t.Fatal(err)
	}
	// Names are unique within a client, ignoring case.
	duplicate := authenticatedRequest(t, handler, http.MethodPost, base, `{"client":"mihomo","name":"streaming","rules":[]}`, "")
	if duplicate.Code != http.StatusUnprocessableEntity || !strings.Contains(duplicate.Body.String(), "name: invalid_or_duplicate_name") {
		t.Fatalf("duplicate status=%d body=%s", duplicate.Code, duplicate.Body.String())
	}
	if other := authenticatedRequest(t, handler, http.MethodPost, base, `{"client":"sing-box","name":"Streaming","rules":[]}`, ""); other.Code != http.StatusCreated {
		t.Fatalf("other client status=%d body=%s", other.Code, other.Body.String())
	}
	listed := authenticatedRequest(t, handler, http.MethodGet, base+"?client=mihomo", "", "")
	var list subscription.SubscriptionRuleSetList
	if listed.Code != http.StatusOK || json.Unmarshal(listed.Body.Bytes(), &list) != nil || len(list.Items) != 1 || list.Items[0].ID != set.ID {
		t.Fatalf("list status=%d body=%s", listed.Code, listed.Body.String())
	}

	body := `{"name":"Streaming","rules":[{"kind":"domain","value":"a.example"},{"kind":"domain_suffix","value":"b.example"}]}`
	if missing := authenticatedRequest(t, handler, http.MethodPut, base+"/"+set.ID, body, ""); missing.Code != http.StatusPreconditionRequired {
		t.Fatalf("missing If-Match status=%d", missing.Code)
	}
	updated := authenticatedRequest(t, handler, http.MethodPut, base+"/"+set.ID, body, created.Header().Get("ETag"))
	if updated.Code != http.StatusOK || updated.Header().Get("ETag") == created.Header().Get("ETag") {
		t.Fatalf("update status=%d body=%s", updated.Code, updated.Body.String())
	}
	if stale := authenticatedRequest(t, handler, http.MethodPut, base+"/"+set.ID, body, created.Header().Get("ETag")); stale.Code != http.StatusPreconditionFailed {
		t.Fatalf("stale update status=%d", stale.Code)
	}

	config := `{"groups":[],"routes":[{"id":"own","enabled":true,"rule":{"kind":"domain","value":"own.example"},"exit":{"kind":"direct"}},
		{"id":"set","enabled":true,"rule_set_id":"` + set.ID + `","exit":{"kind":"reject"}}],"default_exit":{"kind":"direct"}}`
	mismatch := authenticatedRequest(t, handler, http.MethodPost, "/api/v1/subscription/channels",
		`{"name":"sing-box","format":"sing-box","enabled":true,"config":`+config+`}`, "")
	if mismatch.Code != http.StatusUnprocessableEntity || !strings.Contains(mismatch.Body.String(), "routes[1].rule_set_id: rule_set_client_mismatch") {
		t.Fatalf("client mismatch status=%d body=%s", mismatch.Code, mismatch.Body.String())
	}
	channelResponse := authenticatedRequest(t, handler, http.MethodPost, "/api/v1/subscription/channels",
		`{"name":"mihomo","format":"mihomo","enabled":true,"config":`+config+`}`, "")
	if channelResponse.Code != http.StatusCreated {
		t.Fatalf("channel status=%d body=%s", channelResponse.Code, channelResponse.Body.String())
	}
	var channel subscription.SubscriptionChannel
	if err := json.Unmarshal(channelResponse.Body.Bytes(), &channel); err != nil {
		t.Fatal(err)
	}
	preview := authenticatedRequest(t, handler, http.MethodPost, "/api/v1/subscription/channels/"+channel.ID+"/preview", "{}", "")
	var rendered subscription.SubscriptionPreview
	if preview.Code != http.StatusOK || json.Unmarshal(preview.Body.Bytes(), &rendered) != nil ||
		!strings.Contains(string(rendered.Result.Content), "- DOMAIN,own.example,DIRECT\n  - DOMAIN,a.example,REJECT\n  - DOMAIN-SUFFIX,b.example,REJECT\n") {
		t.Fatalf("preview status=%d body=%s", preview.Code, rendered.Result.Content)
	}
	draft := authenticatedRequest(t, handler, http.MethodPost, "/api/v1/subscription/channels/"+channel.ID+"/preview",
		`{"draft":{"format":"mihomo","config":`+config+`}}`, "")
	var drafted subscription.SubscriptionPreview
	if draft.Code != http.StatusOK || json.Unmarshal(draft.Body.Bytes(), &drafted) != nil || !bytes.Equal(drafted.Result.Content, rendered.Result.Content) {
		t.Fatalf("draft preview status=%d body=%s", draft.Code, draft.Body.String())
	}
	if draft := authenticatedRequest(t, handler, http.MethodPost, "/api/v1/subscription/channels/"+channel.ID+"/preview",
		`{"draft":{"format":"sing-box","config":`+config+`}}`, ""); draft.Code != http.StatusUnprocessableEntity || !strings.Contains(draft.Body.String(), "rule_set_client_mismatch") {
		t.Fatalf("mismatched draft status=%d body=%s", draft.Code, draft.Body.String())
	}
	current := authenticatedRequest(t, handler, http.MethodGet, base+"/"+set.ID, "", "")
	if err := json.Unmarshal(current.Body.Bytes(), &set); err != nil || len(set.Channels) != 1 || set.Channels[0].ID != channel.ID {
		t.Fatalf("rule set channels = %s", current.Body.String())
	}

	// Deleting the rule set deletes the exit rule that routes by it, and an
	// edit of the channel begun before then no longer saves.
	if deleted := authenticatedRequest(t, handler, http.MethodDelete, base+"/"+set.ID, "", current.Header().Get("ETag")); deleted.Code != http.StatusNoContent {
		t.Fatalf("delete status=%d body=%s", deleted.Code, deleted.Body.String())
	}
	if gone := authenticatedRequest(t, handler, http.MethodGet, base+"/"+set.ID, "", ""); gone.Code != http.StatusNotFound {
		t.Fatalf("deleted rule set status=%d", gone.Code)
	}
	reread := authenticatedRequest(t, handler, http.MethodGet, "/api/v1/subscription/channels/"+channel.ID, "", "")
	var after subscription.SubscriptionChannel
	if err := json.Unmarshal(reread.Body.Bytes(), &after); err != nil || len(after.Config.Routes) != 1 || after.Config.Routes[0].ID != "own" || !after.UpdatedAt.After(channel.UpdatedAt) {
		t.Fatalf("channel after rule set deletion = %s", reread.Body.String())
	}
	if stale := authenticatedRequest(t, handler, http.MethodPut, "/api/v1/subscription/channels/"+channel.ID,
		`{"name":"mihomo","format":"mihomo","enabled":true,"config":`+emptyChannelConfig+`}`, subscriptionETag(channel.UpdatedAt)); stale.Code != http.StatusPreconditionFailed {
		t.Fatalf("stale channel save status=%d body=%s", stale.Code, stale.Body.String())
	}
}

func TestSubscriptionPreviewHTTPUsesSelectedUser(t *testing.T) {
	_, app, handler, channel, publish := newSubscriptionPublicationHTTPFixture(t, "")
	ctx := context.Background()
	user, err := app.Subscriptions.CreateSubscriptionUser(ctx, subscription.CreateSubscriptionUserRequest{Name: "preview", Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	node := publish()
	preview := authenticatedRequest(t, handler, http.MethodPost,
		"/api/v1/subscription/channels/"+channel.ID+"/preview",
		`{"user_id":"`+user.ID+`"}`, "")
	if preview.Code != http.StatusOK || !strings.Contains(preview.Body.String(), `"node_count":0`) {
		t.Fatalf("preview of a user without grants status=%d body=%s", preview.Code, preview.Body.String())
	}
	if _, err := app.Subscriptions.ReplaceSubscriptionUserGrants(ctx, user.ID, []string{node}, user.UpdatedAt); err != nil {
		t.Fatal(err)
	}
	preview = authenticatedRequest(t, handler, http.MethodPost,
		"/api/v1/subscription/channels/"+channel.ID+"/preview",
		`{"user_id":"`+user.ID+`"}`, "")
	if preview.Code != http.StatusOK || !strings.Contains(preview.Body.String(), `"node_count":1`) ||
		!strings.Contains(preview.Body.String(), `"user_id":"`+user.ID+`"`) {
		t.Fatalf("preview status=%d body=%s", preview.Code, preview.Body.String())
	}
}

func TestPublicSubscriptionHTTPFrozenPublicationAndTokenLifecycle(t *testing.T) {
	ctx := context.Background()
	database, app, handler, channel, publish := newSubscriptionPublicationHTTPFixture(t, "/panel")
	user, err := app.Subscriptions.CreateSubscriptionUser(ctx, subscription.CreateSubscriptionUserRequest{
		Name: "public HTTP user", Enabled: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	created, err := app.Subscriptions.CreateSubscriptionToken(ctx, subscription.CreateSubscriptionTokenRequest{
		UserID: user.ID, Label: "primary",
	})
	if err != nil {
		t.Fatal(err)
	}
	publicPath := "/panel/sub/" + created.Token + "/" + channel.ID
	beforeApply := publicSubscriptionRequest(handler, publicPath)
	if beforeApply.Code != http.StatusServiceUnavailable || !strings.Contains(beforeApply.Body.String(), `"code":"not_applied"`) ||
		strings.Contains(beforeApply.Body.String(), created.Token) {
		t.Fatalf("before apply status=%d body=%s", beforeApply.Code, beforeApply.Body.String())
	}

	node := publish()
	if _, err := app.Subscriptions.ReplaceSubscriptionUserGrants(ctx, user.ID, []string{node}, user.UpdatedAt); err != nil {
		t.Fatal(err)
	}
	channel, err = app.Subscriptions.SubscriptionChannel(ctx, channel.ID)
	if err != nil {
		t.Fatal(err)
	}

	served := publicSubscriptionRequest(handler, publicPath)
	if got := served.Header().Get("Subscription-Userinfo"); got != "upload=0; download=0; total=0" {
		t.Fatalf("subscription traffic header = %q", got)
	}
	if served.Code != http.StatusOK || served.Header().Get("Content-Type") != "application/json" ||
		!strings.Contains(served.Body.String(), "publish.example") || served.Header().Get("Cache-Control") != "no-store" ||
		served.Header().Get("ETag") == "" {
		t.Fatalf("served status=%d content-type=%q body=%s", served.Code, served.Header().Get("Content-Type"), served.Body.String())
	}
	notModified := publicSubscriptionConditionalRequest(handler, publicPath, served.Header().Get("ETag"))
	if got := notModified.Header().Get("Subscription-Userinfo"); got != "upload=0; download=0; total=0" {
		t.Fatalf("conditional subscription traffic header = %q", got)
	}
	if notModified.Code != http.StatusNotModified || notModified.Body.Len() != 0 ||
		notModified.Header().Get("ETag") != served.Header().Get("ETag") {
		t.Fatalf("not modified status=%d etag=%q body=%s", notModified.Code, notModified.Header().Get("ETag"), notModified.Body.String())
	}
	usage, err := app.Subscriptions.SubscriptionToken(ctx, created.Metadata.ID)
	if err != nil || usage.SuccessfulRequestCount != 2 || usage.BodyResponseCount != 1 ||
		usage.BytesServed != int64(served.Body.Len()) || usage.LastUsedAt == nil {
		t.Fatalf("token usage=%+v err=%v", usage, err)
	}
	missingChannel := publicSubscriptionRequest(handler, "/panel/sub/"+created.Token+"/channel_missing")
	if missingChannel.Code != http.StatusNotFound || strings.Contains(missingChannel.Body.String(), created.Token) {
		t.Fatalf("missing channel status=%d body=%s", missingChannel.Code, missingChannel.Body.String())
	}
	for _, target := range []string{
		"/panel/sub/" + created.Token,
		publicPath + "/extra",
		publicPath + "?unexpected=true",
	} {
		response := publicSubscriptionRequest(handler, target)
		if response.Code != http.StatusNotFound && response.Code != http.StatusBadRequest {
			t.Fatalf("strict public path %q status=%d body=%s", target, response.Code, response.Body.String())
		}
		if strings.Contains(response.Body.String(), created.Token) {
			t.Fatalf("strict public path leaked token %q body=%s", target, response.Body.String())
		}
	}

	// The channel is evaluated live: without the node in a group it is not
	// published.
	update := authenticatedRequest(t, handler, http.MethodPut,
		"/panel/api/v1/subscription/channels/"+channel.ID,
		`{"name":"sing-box","format":"sing-box","config":`+emptyChannelConfig+`,"enabled":true}`,
		subscriptionETag(channel.UpdatedAt))
	if update.Code != http.StatusOK {
		t.Fatalf("channel update status=%d body=%s", update.Code, update.Body.String())
	}
	livePolicy := publicSubscriptionRequest(handler, publicPath)
	if livePolicy.Code != http.StatusOK || bytes.Equal(livePolicy.Body.Bytes(), served.Body.Bytes()) ||
		strings.Contains(livePolicy.Body.String(), "publish.example") {
		t.Fatalf("live channel policy was not applied: status=%d body=%s", livePolicy.Code, livePolicy.Body.String())
	}

	rotationResponse := authenticatedRequest(t, handler, http.MethodPost,
		"/panel/api/v1/subscription/tokens/"+created.Metadata.ID+"/rotate", `{}`, "")
	if rotationResponse.Code != http.StatusCreated {
		t.Fatalf("rotate status=%d body=%s", rotationResponse.Code, rotationResponse.Body.String())
	}
	var rotation subscription.SubscriptionTokenRotation
	if err := json.Unmarshal(rotationResponse.Body.Bytes(), &rotation); err != nil {
		t.Fatal(err)
	}
	oldAfterRotate := publicSubscriptionRequest(handler, publicPath)
	unknown := publicSubscriptionRequest(handler, "/panel/sub/unknown-public-token/"+channel.ID)
	if oldAfterRotate.Code != http.StatusNotFound || unknown.Code != http.StatusNotFound ||
		!sameSafePublicProblem(oldAfterRotate.Body.Bytes(), unknown.Body.Bytes()) ||
		bytes.Contains(oldAfterRotate.Body.Bytes(), []byte(created.Token)) {
		t.Fatalf("old=%s unknown=%s", oldAfterRotate.Body.String(), unknown.Body.String())
	}
	newToken := publicSubscriptionRequest(handler, "/panel/sub/"+rotation.Token+"/"+channel.ID)
	if newToken.Code != http.StatusOK {
		t.Fatalf("replacement token status=%d body=%s", newToken.Code, newToken.Body.String())
	}
	revokedResponse := authenticatedRequest(t, handler, http.MethodPost,
		"/panel/api/v1/subscription/tokens/"+rotation.Created.ID+"/revoke", "", "")
	if revokedResponse.Code != http.StatusOK {
		t.Fatalf("revoke status=%d body=%s", revokedResponse.Code, revokedResponse.Body.String())
	}
	revoked := publicSubscriptionRequest(handler, "/panel/sub/"+rotation.Token+"/"+channel.ID)
	if revoked.Code != http.StatusNotFound || !sameSafePublicProblem(revoked.Body.Bytes(), unknown.Body.Bytes()) ||
		bytes.Contains(revoked.Body.Bytes(), []byte(rotation.Token)) {
		t.Fatalf("revoked=%s unknown=%s", revoked.Body.String(), unknown.Body.String())
	}

	const expiredPlaintext = "expired-public-token"
	expiredDigest := sha256.Sum256([]byte(expiredPlaintext))
	expiresAt := time.Now().UTC().Add(-time.Minute)
	if _, err := database.CreateSubscriptionToken(ctx, store.SubscriptionToken{
		ID: "token_http_expired", UserID: user.ID, Label: "expired", Enabled: true,
		TokenSHA256: hex.EncodeToString(expiredDigest[:]),
		ExpiresAt:   &expiresAt, CreatedAt: expiresAt.Add(-time.Hour),
	}); err != nil {
		t.Fatal(err)
	}
	expired := publicSubscriptionRequest(handler, "/panel/sub/"+expiredPlaintext+"/"+channel.ID)
	if expired.Code != http.StatusNotFound || !sameSafePublicProblem(expired.Body.Bytes(), unknown.Body.Bytes()) ||
		bytes.Contains(expired.Body.Bytes(), []byte(expiredPlaintext)) {
		t.Fatalf("expired=%s unknown=%s", expired.Body.String(), unknown.Body.String())
	}

	outsideBase := publicSubscriptionRequest(handler, "/sub/"+rotation.Token+"/"+channel.ID)
	if outsideBase.Code != http.StatusNotFound {
		t.Fatalf("outside base path status=%d body=%s", outsideBase.Code, outsideBase.Body.String())
	}
}

func newSubscriptionHTTPServices(t *testing.T, basePath string) (*store.Store, app.Services, *Handler) {
	t.Helper()
	ctx := context.Background()
	database, err := store.Open(ctx, filepath.Join(t.TempDir(), "panel.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	value := settings.Defaults()
	value.DataDir = t.TempDir()
	value.Auth.Email = testutil.AdminEmail
	value.Auth.PasswordHash = testutil.PasswordHash
	value.Server.BasePath = basePath
	if err := value.Validate(); err != nil {
		t.Fatal(err)
	}
	value = settingsFileFixture(t, value)
	app := newTestServices(t, database, value)
	handler := newTestHandler(t, HandlerOptions{
		Settings: value, Build: buildinfo.Info{Version: "test"}, Services: app,
	})
	return database, app, handler
}

// emptyChannelConfig is a channel configuration without groups.
const emptyChannelConfig = `{"groups":[],"routes":[],"default_exit":{"kind":"direct"}}`

// newSubscriptionPublicationHTTPFixture returns a channel and publish, which
// adds the node the channel publishes and returns its pool ID; until then the
// panel has nothing to publish.
func newSubscriptionPublicationHTTPFixture(
	t *testing.T,
	basePath string,
) (*store.Store, app.Services, *Handler, subscription.SubscriptionChannel, func() string) {
	t.Helper()
	ctx := context.Background()
	database, app, handler := newSubscriptionHTTPServices(t, basePath)
	channel, err := app.Subscriptions.CreateSubscriptionChannel(ctx, subscription.CreateSubscriptionChannelRequest{
		Name: "sing-box", Format: store.SubscriptionFormatSingBox,
		Config: json.RawMessage(emptyChannelConfig), Enabled: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	publish := func() string {
		t.Helper()
		node := createSourceNodes(t, app, "publish", "ss://YWVzLTI1Ni1nY206c2VjcmV0@publish.example:443#publish\n")[0]
		current, err := app.Subscriptions.SubscriptionChannel(ctx, channel.ID)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := app.Subscriptions.UpdateSubscriptionChannel(ctx, channel.ID, subscription.UpdateSubscriptionChannelRequest{
			Name: current.Name, Format: current.Format, Enabled: true, ExpectedUpdatedAt: current.UpdatedAt,
			Config: json.RawMessage(`{"groups":[],"routes":[],"default_exit":{"kind":"node","id":"` + node + `"}}`),
		}); err != nil {
			t.Fatal(err)
		}
		return node
	}
	return database, app, handler, channel, publish
}

func publicSubscriptionRequest(handler http.Handler, target string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(http.MethodGet, target, nil)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	return response
}

func publicSubscriptionConditionalRequest(handler http.Handler, target string, etag string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(http.MethodGet, target, nil)
	request.Header.Set("If-None-Match", etag)
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
