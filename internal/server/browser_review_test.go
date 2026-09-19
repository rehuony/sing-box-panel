// SPDX-License-Identifier: GPL-3.0-or-later

package server

import (
	"context"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"testing"
	"time"

	"github.com/rehuony/sing-box-panel/internal/application"
	"github.com/rehuony/sing-box-panel/internal/buildinfo"
	"github.com/rehuony/sing-box-panel/internal/httpapi"
	"github.com/rehuony/sing-box-panel/internal/settings"
	"github.com/rehuony/sing-box-panel/internal/store"
)

// TestBrowserReview serves the real HTTP boundary and built web assets against
// an isolated database for manual browser verification on development hosts.
// It deliberately has no runtime executor: it cannot validate process lifecycle.
// Run with SBP_BROWSER_REVIEW=1 go test ./internal/server -run '^TestBrowserReview$' -v -timeout 30m.
func TestBrowserReview(t *testing.T) {
	if os.Getenv("SBP_BROWSER_REVIEW") != "1" {
		t.Skip("opt-in browser review")
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	db, err := store.Open(ctx, filepath.Join(t.TempDir(), "panel.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	configuration := settings.Defaults()
	configuration.Auth.Token = "local-review-fixture-token-2026-09-19"
	configuration.Server.Host = "127.0.0.1"
	configuration.Server.Port = 3337
	configuration.Server.ExternalOrigin = "http://127.0.0.1:3337"
	configuration.Auth.SecureCookie = false
	commands := application.FromStoreWithSettings(db, configuration)
	for _, value := range []string{
		`{"type":"socks","tag":"Tokyo","server":"tokyo.example.com","server_port":1080}`,
		`{"type":"socks","tag":"Hong Kong","server":"hk.example.com","server_port":1080}`,
	} {
		if _, err := commands.CreateSubscriptionNode(ctx, []byte(value)); err != nil {
			t.Fatal(err)
		}
	}
	build := buildinfo.Info{Version: "browser-review"}
	handler := httpapi.NewHandler(httpapi.HandlerOptions{
		Settings: configuration, Build: build, Commands: commands,
		Status: &statusProvider{database: db, commands: commands, build: build},
		Assets: os.DirFS("../../web/dist"),
	})
	listener, err := net.Listen("tcp", "127.0.0.1:3337")
	if err != nil {
		t.Fatal(err)
	}
	server := &http.Server{Handler: handler, ReadHeaderTimeout: 5 * time.Second}
	defer server.Close()
	go func() { <-ctx.Done(); _ = server.Close() }()
	t.Log("Browser review: http://127.0.0.1:3337/panel (isolated database; no runtime executor)")
	if err := server.Serve(listener); err != nil && err != http.ErrServerClosed {
		t.Fatal(err)
	}
}
