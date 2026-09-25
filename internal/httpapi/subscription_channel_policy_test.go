// SPDX-License-Identifier: GPL-3.0-or-later

package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/rehuony/sing-box-panel/internal/application"
	"github.com/rehuony/sing-box-panel/internal/store"
	"github.com/rehuony/sing-box-panel/internal/subscription"
)

func TestChannelPolicyPreviewDeliveryAndVisibility(t *testing.T) {
	ctx := context.Background()
	_, app, handler := newSubscriptionHTTPServices(t, "")
	node, err := app.CreateSubscriptionNode(ctx, []byte(`{"type":"socks","tag":"Example","server":"node.example.com","server_port":1080}`))
	if err != nil {
		t.Fatal(err)
	}
	policy := &subscription.ChannelPolicy{
		Selection:   subscription.NodeSelection{IDs: []string{node.ID}, ExcludedIDs: []string{}, NewNodePolicy: "exclude"},
		Organizer:   subscription.NodeOrganizer{Sort: "none", ExcludeNames: []string{}, Incompatible: "error"},
		DefaultExit: subscription.RouteExit{Kind: "node", ID: node.ID}, Groups: []subscription.RuleGroup{},
		Template: &subscription.NativeTemplate{Format: subscription.RenderFormatSingBox, Content: `{"log":{"level":"warn"}}`},
	}
	config, _ := json.Marshal(store.SubscriptionChannelConfig{Policy: policy})
	if _, err := store.DecodeSubscriptionChannelConfig(config); err != nil {
		t.Fatal("decode", err)
	}
	if err := subscription.ValidateChannelPolicy(policy, subscription.RenderFormatSingBox); err != nil {
		t.Fatal("policy", err)
	}
	input := application.CreateSubscriptionChannelRequest{Name: "Modern", Format: store.SubscriptionFormatSingBox, Config: config, Enabled: true}
	raw, _ := json.Marshal(input)
	created := authenticatedRequest(handler, http.MethodPost, "/api/v1/subscription/channels", string(raw), "")
	if created.Code != 201 {
		t.Fatal(created.Code, created.Body.String())
	}
	var channel application.SubscriptionChannel
	if err := json.Unmarshal(created.Body.Bytes(), &channel); err != nil {
		t.Fatal(err)
	}
	key, err := app.CreateSubscriptionToken(ctx, application.CreateSubscriptionTokenRequest{Label: "All channels"})
	if err != nil {
		t.Fatal(err)
	}
	publicURL := "/sub/" + key.Token + "/" + channel.ID
	preview := authenticatedRequest(handler, http.MethodPost, "/api/v1/subscription/channels/"+channel.ID+"/preview", "{}", "")
	var rendered application.SubscriptionPreview
	if preview.Code != 200 {
		t.Fatal(preview.Code, preview.Body.String())
	}
	if err := json.Unmarshal(preview.Body.Bytes(), &rendered); err != nil {
		t.Fatal(err)
	}
	public := publicSubscriptionRequest(handler, publicURL)
	if public.Code != 200 || public.Body.String() != string(rendered.Result.Content) {
		t.Fatal("preview and delivery differ", public.Code, public.Body.String())
	}
	// An unsaved template preview is isolated from persisted/public configuration.
	policy.Template.Content = `{"log":{"level":"error"}}`
	config, _ = json.Marshal(store.SubscriptionChannelConfig{Policy: policy})
	raw, _ = json.Marshal(map[string]any{"draft": application.SubscriptionDraftPreview{Format: store.SubscriptionFormatSingBox, Config: config}})
	preview = authenticatedRequest(handler, http.MethodPost, "/api/v1/subscription/channels/"+channel.ID+"/preview", string(raw), "")
	if preview.Code != 200 {
		t.Fatal(preview.Code, preview.Body.String())
	}
	json.Unmarshal(preview.Body.Bytes(), &rendered)
	if !strings.Contains(string(rendered.Result.Content), `"level": "error"`) {
		t.Fatal("draft did not preview")
	}
	public = publicSubscriptionRequest(handler, publicURL)
	if !strings.Contains(public.Body.String(), `"level": "warn"`) {
		t.Fatal("draft persisted unexpectedly")
	}
	if _, err := app.SetSubscriptionNodeVisibility(ctx, node.ID, true, 0); err != nil {
		t.Fatal(err)
	}
	public = publicSubscriptionRequest(handler, publicURL)
	if public.Code != 200 || strings.Contains(public.Body.String(), "node.example.com") || !strings.Contains(public.Body.String(), `"action": "reject"`) {
		t.Fatal("hidden node leaked or silently fell back", public.Code, public.Body.String())
	}
	unchanged, err := app.SubscriptionChannel(ctx, channel.ID)
	if err != nil || !strings.Contains(string(unchanged.Config), node.ID) {
		t.Fatal("visibility changed selection", err)
	}
	// A template cannot replace the generated authentication/nodes or rules.
	policy.Template.Content = `{"outbounds":[{"password":"must-not-reflect-secret"}]}`
	config, _ = json.Marshal(store.SubscriptionChannelConfig{Policy: policy})
	input.Config = config
	raw, _ = json.Marshal(input)
	invalid := authenticatedRequest(handler, http.MethodPut, "/api/v1/subscription/channels/"+channel.ID, string(raw), subscriptionETag(channel.UpdatedAt))
	if invalid.Code != 422 || !strings.Contains(invalid.Body.String(), "policy.template.content/outbounds") || strings.Contains(invalid.Body.String(), "must-not-reflect-secret") {
		t.Fatal(invalid.Code, invalid.Body.String())
	}
	for _, content := range []string{`{"log":{"level":"must-not-reflect-secret"}}`, `{"log":{"level":42}}`, "{\n\"log\": invalid}"} {
		policy.Template.Content = content
		config, _ = json.Marshal(store.SubscriptionChannelConfig{Policy: policy})
		raw, _ = json.Marshal(map[string]any{"draft": application.SubscriptionDraftPreview{Format: store.SubscriptionFormatSingBox, Config: config}})
		invalid = authenticatedRequest(handler, http.MethodPost, "/api/v1/subscription/channels/"+channel.ID+"/preview", string(raw), "")
		if invalid.Code != 422 || !strings.Contains(invalid.Body.String(), "policy.template.content") || strings.Contains(invalid.Body.String(), "must-not-reflect-secret") {
			t.Fatal("invalid native template was accepted or reflected", invalid.Code, invalid.Body.String())
		}
		input.Config = config
		raw, _ = json.Marshal(input)
		invalid = authenticatedRequest(handler, http.MethodPut, "/api/v1/subscription/channels/"+channel.ID, string(raw), subscriptionETag(channel.UpdatedAt))
		if invalid.Code != 422 {
			t.Fatal("invalid native template was persisted", invalid.Code, invalid.Body.String())
		}
	}
	unchanged, err = app.SubscriptionChannel(ctx, channel.ID)
	if err != nil || unchanged.UpdatedAt != channel.UpdatedAt || !strings.Contains(string(unchanged.Config), `warn`) {
		t.Fatal("invalid save modified channel", err)
	}
}

func TestChannelStrategyTypesPersistPreviewAndDeliver(t *testing.T) {
	ctx := context.Background()
	_, app, handler := newSubscriptionHTTPServices(t, "")
	node, err := app.CreateSubscriptionNode(ctx, []byte(`{"type":"socks","tag":"Example","server":"node.example.com","server_port":1080}`))
	if err != nil {
		t.Fatal(err)
	}
	key, err := app.CreateSubscriptionToken(ctx, application.CreateSubscriptionTokenRequest{Label: "Strategy checks"})
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		format   string
		kind     string
		builtins []string
	}{
		{"sing-box", "select", []string{"direct"}},
		{"sing-box", "url-test", []string{"direct"}},
		{"mihomo", "select", []string{"direct", "reject"}},
		{"mihomo", "url-test", []string{}},
		{"mihomo", "fallback", []string{"direct"}},
		{"loon", "select", []string{"direct", "reject"}},
		{"loon", "url-test", []string{}},
		{"loon", "fallback", []string{}},
	} {
		t.Run(test.format+" "+test.kind, func(t *testing.T) {
			policy := &subscription.ChannelPolicy{
				Selection:   subscription.NodeSelection{IDs: []string{node.ID}, ExcludedIDs: []string{}, NewNodePolicy: "exclude"},
				Organizer:   subscription.NodeOrganizer{ExcludeNames: []string{}, Sort: "none", Incompatible: "error"},
				DefaultExit: subscription.RouteExit{Kind: "group", ID: "group"},
				Groups:      []subscription.RuleGroup{{ID: "group", Name: "Main", Enabled: true, Type: test.kind, BuiltinNodes: test.builtins, NodeIDs: []string{node.ID}, DefaultExit: subscription.RouteExit{Kind: "node", ID: node.ID}, Rules: []subscription.ChannelRule{}, HealthCheck: &subscription.GroupHealthCheck{URL: "https://probe.example/check", Interval: 3600, Tolerance: 0}}},
			}
			for _, kind := range test.builtins {
				policy.Groups[0].CandidateOrder = append(policy.Groups[0].CandidateOrder, "builtin:"+kind)
			}
			policy.Groups[0].CandidateOrder = append(policy.Groups[0].CandidateOrder, "node:"+node.ID)
			config, _ := json.Marshal(store.SubscriptionChannelConfig{Policy: policy})
			raw, _ := json.Marshal(map[string]any{"name": test.format + " " + test.kind, "format": test.format, "config": json.RawMessage(config), "enabled": true})
			created := authenticatedRequest(handler, http.MethodPost, "/api/v1/subscription/channels", string(raw), "")
			if created.Code != http.StatusCreated {
				t.Fatal(created.Code, created.Body.String())
			}
			var channel application.SubscriptionChannel
			if err := json.Unmarshal(created.Body.Bytes(), &channel); err != nil {
				t.Fatal(err)
			}
			stored, err := app.SubscriptionChannel(ctx, channel.ID)
			if err != nil {
				t.Fatal(err)
			}
			var restored store.SubscriptionChannelConfig
			if err := json.Unmarshal(stored.Config, &restored); err != nil {
				t.Fatal(err)
			}
			actual := restored.Policy.Groups[0]
			if !slices.Equal(actual.CandidateOrder, policy.Groups[0].CandidateOrder) {
				t.Fatalf("lost candidate order: %v", actual.CandidateOrder)
			}
			if actual.Type != test.kind || actual.BuiltinNodes == nil || len(actual.BuiltinNodes) != len(test.builtins) || actual.HealthCheck.Tolerance != 0 {
				t.Fatalf("lost group settings: %+v", actual)
			}
			preview := authenticatedRequest(handler, http.MethodPost, "/api/v1/subscription/channels/"+channel.ID+"/preview", "{}", "")
			if preview.Code != http.StatusOK {
				t.Fatal(preview.Code, preview.Body.String())
			}
			var rendered application.SubscriptionPreview
			if err := json.Unmarshal(preview.Body.Bytes(), &rendered); err != nil {
				t.Fatal(err)
			}
			public := publicSubscriptionRequest(handler, "/sub/"+key.Token+"/"+channel.ID)
			if public.Code != http.StatusOK || public.Body.String() != string(rendered.Result.Content) {
				t.Fatal("preview/delivery mismatch", public.Code, public.Body.String())
			}
			if test.format == "mihomo" && !strings.HasPrefix(public.Body.String(), "proxies:\n  - ") {
				t.Fatal("preview/delivery YAML is not formatted", public.Body.String())
			}
		})
	}
}

func TestSubscriptionChannelAPIRejectsRemovedBindings(t *testing.T) {
	_, _, handler := newSubscriptionHTTPServices(t, "")
	response := authenticatedRequest(handler, http.MethodPost, "/api/v1/subscription/channels",
		`{"name":"Removed binding","format":"mihomo","enabled":true,"config":{"export_token_ids":["token-old"]}}`, "")
	if response.Code != http.StatusUnprocessableEntity {
		t.Fatalf("removed binding accepted: %d %s", response.Code, response.Body.String())
	}
}

// Read the editor's actual seed files so frontend defaults and server validation
// cannot silently drift. Saving a smaller base must not resurrect removed fields.
func TestChannelDefaultTemplatesAndWholeReplacement(t *testing.T) {
	ctx := context.Background()
	_, app, handler := newSubscriptionHTTPServices(t, "")
	key, err := app.CreateSubscriptionToken(ctx, application.CreateSubscriptionTokenRequest{Label: "Default checks"})
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct{ format, file, empty string }{
		{"sing-box", "sing-box.json", "{}"},
		{"mihomo", "mihomo.yaml", "{}"},
		{"loon", "loon.conf", ""},
	} {
		t.Run(test.format, func(t *testing.T) {
			content, err := os.ReadFile("../../web/src/constants/channel-templates/" + test.file)
			if err != nil {
				t.Fatal(err)
			}
			p := &subscription.ChannelPolicy{
				Selection: subscription.NodeSelection{IDs: []string{}, ExcludedIDs: []string{}, NewNodePolicy: "exclude"},
				Organizer: subscription.NodeOrganizer{Sort: "none", ExcludeNames: []string{}, Incompatible: "error"},
				Groups:    []subscription.RuleGroup{}, DefaultExit: subscription.RouteExit{Kind: "direct"},
				Template: &subscription.NativeTemplate{Format: subscription.RenderFormat(test.format), Content: string(content)},
			}
			input := map[string]any{"name": test.format, "format": test.format, "enabled": true, "config": store.SubscriptionChannelConfig{Policy: p}}
			raw, _ := json.Marshal(input)
			created := authenticatedRequest(handler, http.MethodPost, "/api/v1/subscription/channels", string(raw), "")
			if created.Code != http.StatusCreated {
				t.Fatal(created.Code, created.Body.String())
			}
			var channel application.SubscriptionChannel
			if err := json.Unmarshal(created.Body.Bytes(), &channel); err != nil {
				t.Fatal(err)
			}
			url := "/api/v1/subscription/channels/" + channel.ID
			publicURL := "/sub/" + key.Token + "/" + channel.ID
			for _, empty := range []bool{false, true} {
				if empty {
					p.Template.Content = test.empty
					raw, _ = json.Marshal(input)
					saved := authenticatedRequest(handler, http.MethodPut, url, string(raw), subscriptionETag(channel.UpdatedAt))
					if saved.Code != http.StatusOK {
						t.Fatal(saved.Code, saved.Body.String())
					}
				}
				preview := authenticatedRequest(handler, http.MethodPost, url+"/preview", "{}", "")
				if preview.Code != http.StatusOK {
					t.Fatal(preview.Code, preview.Body.String())
				}
				var rendered application.SubscriptionPreview
				if err := json.Unmarshal(preview.Body.Bytes(), &rendered); err != nil {
					t.Fatal(err)
				}
				// Exercise draft validation on a brand-new instance before adding a
				// publication source; public delivery still requires one.
				if test.format == "sing-box" && !empty {
					if _, err := app.CreateSubscriptionNode(ctx, []byte(`{"type":"socks","tag":"Unselected","server":"node.example.com","server_port":1080}`)); err != nil {
						t.Fatal(err)
					}
				}
				public := publicSubscriptionRequest(handler, publicURL)
				if public.Code != http.StatusOK || public.Body.String() != string(rendered.Result.Content) {
					t.Fatal("preview/delivery mismatch", public.Code, public.Body.String())
				}
				if strings.Contains(public.Body.String(), "223.5.5.5") == empty {
					t.Fatal("base was not replaced", public.Body.String())
				}
				stored, err := app.SubscriptionChannel(ctx, channel.ID)
				if err != nil {
					t.Fatal(err)
				}
				config, err := store.DecodeSubscriptionChannelConfig(stored.Config)
				if err != nil || config.Policy.Template.Content != p.Template.Content {
					t.Fatal("template bytes changed", err)
				}
			}
		})
	}
}
