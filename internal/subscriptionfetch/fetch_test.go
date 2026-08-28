// SPDX-License-Identifier: GPL-3.0-or-later

package subscriptionfetch

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestFetchRejectsPrivateByDefaultAndAllowsExplicitCIDR(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("socks5://example.test:1080\n"))
	}))
	t.Cleanup(server.Close)
	if _, err := Fetch(context.Background(), server.URL, nil); !errors.Is(err, ErrAddressDenied) {
		t.Fatalf("default private address error=%v", err)
	}
	body, err := Fetch(context.Background(), server.URL, []string{"127.0.0.1/32"})
	if err != nil || string(body) != "socks5://example.test:1080\n" {
		t.Fatalf("allowed body=%q err=%v", body, err)
	}
}

func TestFetchRevalidatesRedirectTargetAndHidesCredentials(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		http.Redirect(w, request, "http://127.0.0.2:12345/private", http.StatusFound)
	}))
	t.Cleanup(server.Close)
	withCredentials := strings.Replace(server.URL, "http://", "http://user:super-secret@", 1)
	_, err := Fetch(context.Background(), withCredentials, []string{"127.0.0.1/32"})
	if !errors.Is(err, ErrAddressDenied) || strings.Contains(err.Error(), "super-secret") {
		t.Fatalf("redirect error=%v", err)
	}
}

func TestFetchBoundsRedirectsAndBody(t *testing.T) {
	t.Parallel()
	redirects := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		step := 0
		_, _ = fmt.Sscanf(strings.TrimPrefix(request.URL.Path, "/"), "%d", &step)
		http.Redirect(w, request, fmt.Sprintf("/%d", step+1), http.StatusFound)
	}))
	t.Cleanup(redirects.Close)
	if _, err := Fetch(context.Background(), redirects.URL+"/0", []string{"127.0.0.1/32"}); !errors.Is(err, ErrTooManyRedirects) {
		t.Fatalf("redirect error=%v", err)
	}

	large := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(bytes.Repeat([]byte{'x'}, MaximumBodyBytes+1))
	}))
	t.Cleanup(large.Close)
	if _, err := Fetch(context.Background(), large.URL, []string{"127.0.0.1/32"}); !errors.Is(err, ErrBodyTooLarge) {
		t.Fatalf("body limit error=%v", err)
	}
}
