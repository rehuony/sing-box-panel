// SPDX-License-Identifier: GPL-3.0-or-later

package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/rehuony/sing-box-panel/internal/sing-box-panel/app"
	"github.com/rehuony/sing-box-panel/internal/sing-box-panel/store"
	"github.com/rehuony/sing-box-panel/internal/sing-box-panel/subscription"
)

// createSourceNodes creates a file source with content and returns the IDs
// of the pool nodes it contributes, in content order.
func createSourceNodes(t *testing.T, services app.Services, name, content string) []string {
	t.Helper()
	ctx := t.Context()
	source, err := services.Subscriptions.CreateSubscriptionSource(ctx, subscription.CreateSubscriptionSourceRequest{
		Name: name, Kind: store.SubscriptionSourceFile, Filter: &subscription.SourceFilter{}, Content: &content,
	})
	if err != nil {
		t.Fatal(err)
	}
	pool, err := services.Subscriptions.SubscriptionNodes(ctx)
	if err != nil {
		t.Fatal(err)
	}
	ids := []string{}
	for _, node := range pool.Items {
		if node.Origin.SourceID == source.ID {
			ids = append(ids, node.ID)
		}
	}
	return ids
}

func TestChannelPreviewDeliveryAndNodeRemoval(t *testing.T) {
	ctx := context.Background()
	_, services, handler := newSubscriptionHTTPServices(t, "")
	nodes := createSourceNodes(t, services, "source", "socks5://node.example.com:1080#Example\nsocks5://other.example.com:1080#Other\n")
	config := subscription.ChannelConfig{
		IncompatibleNodes: "error", DefaultExit: subscription.RouteExit{Kind: "node", ID: nodes[0]}, Groups: []subscription.RuleGroup{},
		Template: &subscription.NativeTemplate{Format: subscription.RenderFormatSingBox, Content: `{"log":{"level":"warn"}}`},
	}
	input := map[string]any{"name": "Modern", "format": "sing-box", "config": config, "enabled": true}
	raw, _ := json.Marshal(input)
	created := authenticatedRequest(t, handler, http.MethodPost, "/api/v1/subscription/channels", string(raw), "")
	if created.Code != 201 {
		t.Fatal(created.Code, created.Body.String())
	}
	var channel subscription.SubscriptionChannel
	if err := json.Unmarshal(created.Body.Bytes(), &channel); err != nil {
		t.Fatal(err)
	}
	key, err := services.Subscriptions.CreateSubscriptionToken(ctx, subscription.CreateSubscriptionTokenRequest{Label: "All channels"})
	if err != nil {
		t.Fatal(err)
	}
	publicURL := "/sub/" + key.Token + "/" + channel.ID
	preview := authenticatedRequest(t, handler, http.MethodPost, "/api/v1/subscription/channels/"+channel.ID+"/preview", "{}", "")
	var rendered subscription.SubscriptionPreview
	if preview.Code != 200 {
		t.Fatal(preview.Code, preview.Body.String())
	}
	if err := json.Unmarshal(preview.Body.Bytes(), &rendered); err != nil {
		t.Fatal(err)
	}
	public := publicSubscriptionRequest(handler, publicURL)
	if public.Code != 200 || public.Body.String() != string(rendered.Result.Content) || !strings.Contains(public.Body.String(), "node.example.com") ||
		strings.Contains(public.Body.String(), "other.example.com") {
		t.Fatal("preview and delivery differ, or an unnamed node was published", public.Code, public.Body.String())
	}
	// An unsaved template preview is isolated from persisted/public configuration.
	draftConfig := config
	draftConfig.Template = &subscription.NativeTemplate{Format: subscription.RenderFormatSingBox, Content: `{"log":{"level":"error"}}`}
	raw, _ = json.Marshal(map[string]any{"draft": map[string]any{"format": "sing-box", "config": draftConfig}})
	preview = authenticatedRequest(t, handler, http.MethodPost, "/api/v1/subscription/channels/"+channel.ID+"/preview", string(raw), "")
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
	// A draft names only nodes of the pool.
	draftConfig.DefaultExit = subscription.RouteExit{Kind: "node", ID: "sn_missing"}
	raw, _ = json.Marshal(map[string]any{"draft": map[string]any{"format": "sing-box", "config": draftConfig}})
	if missing := authenticatedRequest(t, handler, http.MethodPost, "/api/v1/subscription/channels/"+channel.ID+"/preview", string(raw), ""); missing.Code != 422 || !strings.Contains(missing.Body.String(), "node_not_in_pool") {
		t.Fatal(missing.Code, missing.Body.String())
	}
	// A template cannot replace the generated authentication/nodes or rules.
	invalidConfig := config
	invalidConfig.Template = &subscription.NativeTemplate{Format: subscription.RenderFormatSingBox, Content: `{"outbounds":[{"password":"must-not-reflect-secret"}]}`}
	input["config"] = invalidConfig
	raw, _ = json.Marshal(input)
	invalid := authenticatedRequest(t, handler, http.MethodPut, "/api/v1/subscription/channels/"+channel.ID, string(raw), subscriptionETag(channel.UpdatedAt))
	if invalid.Code != 422 || !strings.Contains(invalid.Body.String(), "template.content/outbounds") || strings.Contains(invalid.Body.String(), "must-not-reflect-secret") {
		t.Fatal(invalid.Code, invalid.Body.String())
	}
	for _, content := range []string{`{"log":{"level":"must-not-reflect-secret"}}`, `{"log":{"level":42}}`, "{\n\"log\": invalid}"} {
		invalidConfig.Template = &subscription.NativeTemplate{Format: subscription.RenderFormatSingBox, Content: content}
		raw, _ = json.Marshal(map[string]any{"draft": map[string]any{"format": "sing-box", "config": invalidConfig}})
		invalid = authenticatedRequest(t, handler, http.MethodPost, "/api/v1/subscription/channels/"+channel.ID+"/preview", string(raw), "")
		if invalid.Code != 422 || !strings.Contains(invalid.Body.String(), "template.content") || strings.Contains(invalid.Body.String(), "must-not-reflect-secret") {
			t.Fatal("invalid native template was accepted or reflected", invalid.Code, invalid.Body.String())
		}
		input["config"] = invalidConfig
		raw, _ = json.Marshal(input)
		invalid = authenticatedRequest(t, handler, http.MethodPut, "/api/v1/subscription/channels/"+channel.ID, string(raw), subscriptionETag(channel.UpdatedAt))
		if invalid.Code != 422 {
			t.Fatal("invalid native template was persisted", invalid.Code, invalid.Body.String())
		}
	}
	unchanged, err := services.Subscriptions.SubscriptionChannel(ctx, channel.ID)
	if err != nil || unchanged.UpdatedAt != channel.UpdatedAt || unchanged.Config.Template.Content != `{"log":{"level":"warn"}}` {
		t.Fatal("invalid save modified channel", err)
	}
	// A node that leaves the pool leaves the channel: its default exit falls
	// back to direct instead of naming another proxy.
	sources, err := services.Subscriptions.ListSubscriptionSources(ctx, subscription.SubscriptionListRequest{})
	if err != nil {
		t.Fatal(err)
	}
	source := sources.Items[0]
	if _, err := services.Subscriptions.UpdateSubscriptionSource(ctx, source.ID, subscription.UpdateSubscriptionSourceRequest{
		Name: source.Name, Format: source.Format, Filter: &source.Filter, Content: new("socks5://other.example.com:1080#Other\n"),
	}, source.UpdatedAt); err != nil {
		t.Fatal(err)
	}
	public = publicSubscriptionRequest(handler, publicURL)
	if public.Code != 200 || strings.Contains(public.Body.String(), "example.com") || !strings.Contains(public.Body.String(), `"final": "direct"`) {
		t.Fatal("a removed node stayed published", public.Code, public.Body.String())
	}
}

func TestLoonPreviewDiagnosticsAndALPNDelivery(t *testing.T) {
	ctx := context.Background()
	_, app, handler := newSubscriptionHTTPServices(t, "")
	ids := createSourceNodes(t, app, "nodes", `{"outbounds":[{"type":"anytls","tag":"AnyTLS good","server":"good.example","server_port":443,"password":"good-secret","tls":{"enabled":true,"alpn":["h2","http/1.1"]}},
		{"type":"anytls","tag":"AnyTLS unsupported","server":"private.example","server_port":443,"password":"private-secret","tls":{"enabled":true,"fragment":true}}]}`)
	good, bad := ids[0], ids[1]
	config, _ := json.Marshal(subscription.ChannelConfig{
		IncompatibleNodes: "skip", DefaultExit: subscription.RouteExit{Kind: "group", ID: "main"},
		Groups: []subscription.RuleGroup{{ID: "main", Name: "Selected", Type: "select", Enabled: true, Members: []subscription.GroupMember{{Kind: "node", ID: good}, {Kind: "node", ID: bad}}, Rules: []subscription.ChannelRule{}}},
	})
	channel, err := app.Subscriptions.CreateSubscriptionChannel(ctx, subscription.CreateSubscriptionChannelRequest{Name: "Loon", Format: store.SubscriptionFormatLoon, Config: config, Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	response := authenticatedRequest(t, handler, http.MethodPost, "/api/v1/subscription/channels/"+channel.ID+"/preview", "{}", "")
	var preview subscription.SubscriptionPreview
	if response.Code != http.StatusOK || json.Unmarshal(response.Body.Bytes(), &preview) != nil {
		t.Fatalf("preview failed: %d", response.Code)
	}
	if preview.Result.NodeCount != 1 || len(preview.Result.Diagnostics) != 1 {
		t.Fatal("unexpected preview counts")
	}
	issue := preview.Result.Diagnostics[0]
	if issue.NodeID != bad || issue.NodeName != "AnyTLS unsupported" || issue.NodeType != "anytls" || issue.FieldPath != "outbounds[1].tls.fragment" || issue.Reason != "unsupported_tls_option" {
		t.Fatalf("wrong preview diagnostic: %+v", issue)
	}
	key, err := app.Subscriptions.CreateSubscriptionToken(ctx, subscription.CreateSubscriptionTokenRequest{Label: "Loon"})
	if err != nil {
		t.Fatal(err)
	}
	public, err := app.Subscriptions.PublicSubscription(ctx, key.Token, channel.ID)
	if err != nil || !bytes.Equal(public.Body, preview.Result.Content) {
		t.Fatal("preview and delivery differ", err)
	}
	for _, expected := range []string{`alpn="h2,http/1.1"`, "Selected = select,AnyTLS good", "FINAL,Selected"} {
		if !bytes.Contains(public.Body, []byte(expected)) {
			t.Fatalf("missing generated behavior %q", expected)
		}
	}
	diagnostics, _ := json.Marshal(public.Diagnostics)
	for _, value := range []string{bad, "AnyTLS unsupported", "private.example", "private-secret"} {
		if bytes.Contains(diagnostics, []byte(value)) || bytes.Contains(public.Body, []byte(value)) {
			t.Fatal("omitted node leaked into public response")
		}
	}
}

func TestChannelStrategyTypesPersistPreviewAndDeliver(t *testing.T) {
	ctx := context.Background()
	_, app, handler := newSubscriptionHTTPServices(t, "")
	node := createSourceNodes(t, app, "source", "socks5://node.example.com:1080#Example\n")[0]
	key, err := app.Subscriptions.CreateSubscriptionToken(ctx, subscription.CreateSubscriptionTokenRequest{Label: "Strategy checks"})
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
			members := []subscription.GroupMember{{Kind: "group", ID: "child"}}
			for _, kind := range test.builtins {
				members = append(members, subscription.GroupMember{Kind: "builtin", ID: kind})
			}
			members = append(members, subscription.GroupMember{Kind: "node", ID: node})
			remote := &subscription.RemoteRuleSet{Name: "Named rules", URL: "https://rules.example/list", Format: map[string]string{"sing-box": "source", "mihomo": "yaml", "loon": "loon"}[test.format]}
			if test.format == "mihomo" {
				remote.Behavior = "domain"
			}
			policy := &subscription.ChannelConfig{
				IncompatibleNodes: "error",
				DefaultExit:       subscription.RouteExit{Kind: "group", ID: "group"},
				Groups: []subscription.RuleGroup{
					{ID: "group", Name: "Main", Enabled: true, Type: test.kind, Members: members, HealthCheck: &subscription.GroupHealthCheck{URL: "https://probe.example/check", Interval: 3600, Tolerance: 0},
						Rules: []subscription.ChannelRule{{ID: "remote", Enabled: true, Kind: "remote", SortIndex: -5, Remote: remote, Exit: subscription.RouteExit{Kind: "group-default"}}}},
					{ID: "child", Name: "Child", Enabled: true, Type: "select", Members: []subscription.GroupMember{{Kind: "node", ID: node}}, Rules: []subscription.ChannelRule{}},
				},
			}
			config, _ := json.Marshal(policy)
			raw, _ := json.Marshal(map[string]any{"name": test.format + " " + test.kind, "format": test.format, "config": json.RawMessage(config), "enabled": true})
			created := authenticatedRequest(t, handler, http.MethodPost, "/api/v1/subscription/channels", string(raw), "")
			if created.Code != http.StatusCreated {
				t.Fatal(created.Code, created.Body.String())
			}
			var channel subscription.SubscriptionChannel
			if err := json.Unmarshal(created.Body.Bytes(), &channel); err != nil {
				t.Fatal(err)
			}
			stored, err := app.Subscriptions.SubscriptionChannel(ctx, channel.ID)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(stored.Config, *policy) {
				t.Fatalf("channel did not read back as saved: %+v", stored.Config)
			}
			// Renaming a group preserves its stable references through a real save.
			policy.Groups[1].Name = "🏠 Renamed child"
			config, _ = json.Marshal(policy)
			raw, _ = json.Marshal(map[string]any{"name": channel.Name, "format": test.format, "config": json.RawMessage(config), "enabled": true})
			saved := authenticatedRequest(t, handler, http.MethodPut, "/api/v1/subscription/channels/"+channel.ID, string(raw), subscriptionETag(channel.UpdatedAt))
			if saved.Code != http.StatusOK {
				t.Fatal(saved.Code, saved.Body.String())
			}
			invalidConfig := strings.Replace(string(config), `"remote":{`, `"remote":{"update_interval":3600,`, 1)
			invalidRaw, _ := json.Marshal(map[string]any{"name": "Removed interval", "format": test.format, "config": json.RawMessage(invalidConfig), "enabled": true})
			invalid := authenticatedRequest(t, handler, http.MethodPost, "/api/v1/subscription/channels", string(invalidRaw), "")
			if invalid.Code != http.StatusUnprocessableEntity {
				t.Fatal("removed interval accepted", invalid.Code, invalid.Body.String())
			}
			preview := authenticatedRequest(t, handler, http.MethodPost, "/api/v1/subscription/channels/"+channel.ID+"/preview", "{}", "")
			if preview.Code != http.StatusOK {
				t.Fatal(preview.Code, preview.Body.String())
			}
			var rendered subscription.SubscriptionPreview
			if err := json.Unmarshal(preview.Body.Bytes(), &rendered); err != nil {
				t.Fatal(err)
			}
			public := publicSubscriptionRequest(handler, "/sub/"+key.Token+"/"+channel.ID)
			if public.Code != http.StatusOK || public.Body.String() != string(rendered.Result.Content) {
				t.Fatal("preview/delivery mismatch", public.Code, public.Body.String())
			}
			if !strings.Contains(public.Body.String(), "🏠 Renamed child") || (test.format != "loon" && !strings.Contains(public.Body.String(), "Named rules")) || strings.Contains(public.Body.String(), "update_interval") {
				t.Fatal(public.Body.String())
			}
			if test.format == "mihomo" && !strings.Contains(public.Body.String(), "proxies:\n  - ") {
				t.Fatal("preview/delivery YAML is not formatted", public.Body.String())
			}
		})
	}
}

func TestSubscriptionChannelAPIRejectsRemovedBindings(t *testing.T) {
	_, _, handler := newSubscriptionHTTPServices(t, "")
	response := authenticatedRequest(t, handler, http.MethodPost, "/api/v1/subscription/channels",
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
	key, err := app.Subscriptions.CreateSubscriptionToken(ctx, subscription.CreateSubscriptionTokenRequest{Label: "Default checks"})
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct{ format, file, empty string }{
		{"sing-box", "sing-box.json", "{}"},
		{"mihomo", "mihomo.yaml", "{}"},
		{"loon", "loon.conf", ""},
	} {
		t.Run(test.format, func(t *testing.T) {
			content, err := os.ReadFile("../subscription/templates/" + test.file)
			if err != nil {
				t.Fatal(err)
			}
			p := &subscription.ChannelConfig{
				IncompatibleNodes: "error",
				Groups:            []subscription.RuleGroup{}, DefaultExit: subscription.RouteExit{Kind: "direct"},
			}
			input := map[string]any{"name": test.format, "format": test.format, "enabled": true, "config": p}
			raw, _ := json.Marshal(input)
			created := authenticatedRequest(t, handler, http.MethodPost, "/api/v1/subscription/channels", string(raw), "")
			if created.Code != http.StatusCreated {
				t.Fatal(created.Code, created.Body.String())
			}
			var channel subscription.SubscriptionChannel
			if err := json.Unmarshal(created.Body.Bytes(), &channel); err != nil {
				t.Fatal(err)
			}
			url := "/api/v1/subscription/channels/" + channel.ID
			publicURL := "/sub/" + key.Token + "/" + channel.ID
			for _, empty := range []bool{false, true} {
				if empty {
					p.Template = &subscription.NativeTemplate{Format: subscription.RenderFormat(test.format), Content: test.empty}
					raw, _ = json.Marshal(input)
					saved := authenticatedRequest(t, handler, http.MethodPut, url, string(raw), subscriptionETag(channel.UpdatedAt))
					if saved.Code != http.StatusOK {
						t.Fatal(saved.Code, saved.Body.String())
					}
				}
				preview := authenticatedRequest(t, handler, http.MethodPost, url+"/preview", "{}", "")
				if preview.Code != http.StatusOK {
					t.Fatal(preview.Code, preview.Body.String())
				}
				var rendered subscription.SubscriptionPreview
				if err := json.Unmarshal(preview.Body.Bytes(), &rendered); err != nil {
					t.Fatal(err)
				}
				if !empty {
					draftPolicy := *p
					draftPolicy.Template = &subscription.NativeTemplate{Format: subscription.RenderFormat(test.format), Content: string(content)}
					draftRaw, _ := json.Marshal(map[string]any{"draft": map[string]any{"format": test.format, "config": draftPolicy}})
					draft := authenticatedRequest(t, handler, http.MethodPost, url+"/preview", string(draftRaw), "")
					var explicit subscription.SubscriptionPreview
					if draft.Code != http.StatusOK || json.Unmarshal(draft.Body.Bytes(), &explicit) != nil || string(explicit.Result.Content) != string(rendered.Result.Content) {
						t.Fatal("missing/default template preview diverged", draft.Code, draft.Body.String())
					}
				}
				// Exercise draft validation on a brand-new instance before the pool
				// has a node; public delivery still requires one.
				if test.format == "sing-box" && !empty {
					createSourceNodes(t, app, "unselected", "socks5://node.example.com:1080#Unselected\n")
				}
				public := publicSubscriptionRequest(handler, publicURL)
				if public.Code != http.StatusOK || public.Body.String() != string(rendered.Result.Content) {
					t.Fatal("preview/delivery mismatch", public.Code, public.Body.String())
				}
				if strings.Contains(public.Body.String(), "223.5.5.5") == empty {
					t.Fatal("base was not replaced", public.Body.String())
				}
				stored, err := app.Subscriptions.SubscriptionChannel(ctx, channel.ID)
				if err != nil {
					t.Fatal(err)
				}
				config := stored.Config
				if (empty && config.Template.Content != p.Template.Content) || (!empty && config.Template != nil) {
					t.Fatal("template bytes changed")
				}
			}
		})
	}
}

func TestChannelAPIRejectsRemovedConfiguration(t *testing.T) {
	_, services, handler := newSubscriptionHTTPServices(t, "")
	node := createSourceNodes(t, services, "source", "socks5://node.example.com:1080#Example\n")[0]
	created := authenticatedRequest(t, handler, http.MethodPost, "/api/v1/subscription/channels",
		`{"name":"Current","format":"sing-box","enabled":true,"config":{"groups":[],"default_exit":{"kind":"direct"}}}`, "")
	if created.Code != http.StatusCreated {
		t.Fatal(created.Code, created.Body.String())
	}
	for _, config := range []string{
		`{}`,
		`{"exclude_types":["socks"],"groups":[],"default_exit":{"kind":"direct"}}`,
		`{"policy":{"groups":[],"default_exit":{"kind":"direct"}}}`,
		`{"selection":{"ids":[],"excluded_ids":[],"new_node_policy":"include"},"groups":[],"default_exit":{"kind":"direct"}}`,
		`{"groups":[{"id":"g","name":"G","enabled":true,"type":"select","builtin_nodes":[],"node_ids":["` + node + `"],"rules":[]}],"default_exit":{"kind":"direct"}}`,
		`{"groups":[{"id":"g","name":"G","enabled":true,"type":"select","members":["node:` + node + `"],"rules":[]}],"default_exit":{"kind":"direct"}}`,
		`{"groups":[],"default_exit":null}`,
	} {
		rejected := authenticatedRequest(t, handler, http.MethodPost, "/api/v1/subscription/channels",
			`{"name":"Invalid","format":"sing-box","enabled":true,"config":`+config+`}`, "")
		if rejected.Code != http.StatusUnprocessableEntity {
			t.Fatalf("config %s accepted: %d %s", config, rejected.Code, rejected.Body.String())
		}
	}
}
