// SPDX-License-Identifier: GPL-3.0-or-later

package runtime

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/rehuony/sing-box-panel/internal/coreartifact"
	"github.com/tailscale/hujson"
)

const maximumResponseBytes = 4 << 20

var (
	ErrInvalidClashConfig = errors.New("limited monitoring requires a loopback Clash API with a non-empty secret")
	ErrClashUnavailable   = errors.New("Clash API is unavailable")
)

type ClashEndpoint struct {
	BaseURL string
	Secret  string
}

type ClashSample struct {
	Memory        int64
	Connections   int64
	UploadTotal   int64
	DownloadTotal int64
}

// ParseClashEndpoint reads the final startup bytes without rewriting them. Both
// structured JSON and the JSONC accepted by sing-box are supported.
func ParseClashEndpoint(raw []byte) (ClashEndpoint, error) {
	parsed, err := hujson.Parse(raw)
	if err != nil {
		return ClashEndpoint{}, fmt.Errorf("%w: invalid JSONC", ErrInvalidClashConfig)
	}
	parsed.Standardize()
	var document struct {
		Experimental struct {
			ClashAPI struct {
				ExternalController string `json:"external_controller"`
				Secret             string `json:"secret"`
			} `json:"clash_api"`
		} `json:"experimental"`
	}
	decoder := json.NewDecoder(bytes.NewReader(parsed.Pack()))
	if err := decoder.Decode(&document); err != nil {
		return ClashEndpoint{}, fmt.Errorf("%w: invalid JSON", ErrInvalidClashConfig)
	}
	controller := strings.TrimSpace(document.Experimental.ClashAPI.ExternalController)
	secret := strings.TrimSpace(document.Experimental.ClashAPI.Secret)
	if controller == "" || secret == "" || strings.ContainsAny(secret, "\x00\r\n") {
		return ClashEndpoint{}, ErrInvalidClashConfig
	}
	host, port, err := net.SplitHostPort(controller)
	if err != nil || port == "" {
		return ClashEndpoint{}, ErrInvalidClashConfig
	}
	ip := net.ParseIP(strings.Trim(host, "[]"))
	if ip == nil || !ip.IsLoopback() {
		return ClashEndpoint{}, ErrInvalidClashConfig
	}
	return ClashEndpoint{BaseURL: "http://" + net.JoinHostPort(ip.String(), port), Secret: secret}, nil
}

type ClashClient struct {
	endpoint ClashEndpoint
	client   *http.Client
}

func NewClashClient(endpoint ClashEndpoint) (*ClashClient, error) {
	parsed, err := url.Parse(endpoint.BaseURL)
	if err != nil || parsed.Scheme != "http" || parsed.Host == "" || parsed.Path != "" ||
		parsed.User != nil || strings.TrimSpace(endpoint.Secret) == "" {
		return nil, ErrInvalidClashConfig
	}
	host, _, err := net.SplitHostPort(parsed.Host)
	if err != nil {
		return nil, ErrInvalidClashConfig
	}
	ip := net.ParseIP(strings.Trim(host, "[]"))
	if ip == nil || !ip.IsLoopback() {
		return nil, ErrInvalidClashConfig
	}
	return &ClashClient{
		endpoint: endpoint,
		client: &http.Client{
			Timeout:   5 * time.Second,
			Transport: &http.Transport{Proxy: nil},
		},
	}, nil
}

func (client *ClashClient) Version(ctx context.Context) (string, error) {
	var response struct {
		Version string `json:"version"`
	}
	if err := client.get(ctx, "/version", &response); err != nil {
		return "", err
	}
	version, err := normalizeVersion(response.Version)
	if err != nil {
		return "", fmt.Errorf("%w: version is missing", ErrClashUnavailable)
	}
	return version, nil
}

func normalizeVersion(raw string) (string, error) {
	value := strings.TrimSpace(raw)
	value = strings.TrimPrefix(value, "sing-box ")
	value = strings.TrimPrefix(value, "v")
	parsed, err := coreartifact.ParseExactVersion(value)
	if err != nil {
		return "", err
	}
	return parsed.String(), nil
}

func (client *ClashClient) Connections(ctx context.Context) (ClashSample, error) {
	var response struct {
		Memory        int64             `json:"memory"`
		UploadTotal   int64             `json:"uploadTotal"`
		DownloadTotal int64             `json:"downloadTotal"`
		Connections   []json.RawMessage `json:"connections"`
	}
	if err := client.get(ctx, "/connections", &response); err != nil {
		return ClashSample{}, err
	}
	if response.Memory < 0 || response.UploadTotal < 0 || response.DownloadTotal < 0 {
		return ClashSample{}, fmt.Errorf("%w: counters are invalid", ErrClashUnavailable)
	}
	return ClashSample{
		Memory: response.Memory, Connections: int64(len(response.Connections)),
		UploadTotal: response.UploadTotal, DownloadTotal: response.DownloadTotal,
	}, nil
}

func (client *ClashClient) get(ctx context.Context, path string, destination any) error {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, client.endpoint.BaseURL+path, nil)
	if err != nil {
		return fmt.Errorf("%w: construct request", ErrClashUnavailable)
	}
	request.Header.Set("Authorization", "Bearer "+client.endpoint.Secret)
	response, err := client.client.Do(request)
	if err != nil {
		return fmt.Errorf("%w: request failed", ErrClashUnavailable)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("%w: unexpected status %d", ErrClashUnavailable, response.StatusCode)
	}
	limited := io.LimitReader(response.Body, maximumResponseBytes+1)
	body, err := io.ReadAll(limited)
	if err != nil || len(body) > maximumResponseBytes {
		return fmt.Errorf("%w: invalid response body", ErrClashUnavailable)
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	if err := decoder.Decode(destination); err != nil {
		return fmt.Errorf("%w: invalid response JSON", ErrClashUnavailable)
	}
	return nil
}
