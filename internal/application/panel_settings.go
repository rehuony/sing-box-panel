// SPDX-License-Identifier: GPL-3.0-or-later

package application

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/netip"
	"regexp"
	"strings"
	"unicode"

	"github.com/rehuony/sing-box-panel/internal/settings"
	"github.com/rehuony/sing-box-panel/internal/store"
)

type AppearanceSettings struct {
	Theme  string `json:"theme"`
	Color  string `json:"color"`
	Radius int    `json:"radius"`
}

// PanelPreferences is safe to return to authenticated clients. Credentials are
// write-only and stored separately from this response projection.
type PanelPreferences struct {
	ListenHost      string             `json:"listen_host"`
	ListenPort      int                `json:"listen_port"`
	ExternalOrigin  string             `json:"external_origin"`
	PublicNodeHost  string             `json:"public_node_host"`
	IdentityName    string             `json:"identity_name"`
	TrafficQuotaGiB *int64             `json:"traffic_quota_gib"`
	Language        string             `json:"language"`
	Appearance      AppearanceSettings `json:"appearance"`
}

type PanelSettingsView struct {
	DetectedPublicIP      string           `json:"detected_public_ip,omitempty"`
	Revision              int64            `json:"revision"`
	Preferences           PanelPreferences `json:"preferences"`
	GitHubTokenConfigured bool             `json:"github_token_configured"`
	IdentityKeyConfigured bool             `json:"identity_key_configured"`
	RestartRequired       bool             `json:"restart_required"`
}

type PanelSettingsWrite struct {
	Revision         int64            `json:"revision"`
	Preferences      PanelPreferences `json:"preferences"`
	GitHubToken      string           `json:"github_token,omitempty"`
	ClearGitHubToken bool             `json:"clear_github_token,omitempty"`
	IdentityKey      string           `json:"identity_key,omitempty"`
	ManagementToken  string           `json:"management_token,omitempty"`
}

type storedPanelSettings struct {
	Preferences     PanelPreferences `json:"preferences"`
	GitHubToken     string           `json:"github_token"`
	IdentityKey     string           `json:"identity_key"`
	ManagementToken string           `json:"management_token"`
}

var ErrPanelSettingsInvalid = errors.New("panel settings are invalid")
var panelColor = regexp.MustCompile(`^#[0-9a-fA-F]{6}$`)
var hostnameLabel = regexp.MustCompile(`^[a-zA-Z0-9](?:[a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?$`)

func (app *Application) storedPanelSettings(ctx context.Context) (storedPanelSettings, int64, error) {
	document, revision, err := app.database.PanelSettings(ctx)
	if err != nil {
		return storedPanelSettings{}, 0, err
	}
	value := storedPanelSettings{
		Preferences: PanelPreferences{
			ListenHost: app.settings.Server.Host, ListenPort: app.settings.Server.Port,
			ExternalOrigin: app.settings.Server.ExternalOrigin, TrafficQuotaGiB: app.settings.Traffic.QuotaGiB,
			Language: "zh-CN", Appearance: AppearanceSettings{Theme: "light", Color: "#6D4ED1", Radius: 24},
		},
		GitHubToken: app.settings.GitHub.Token, ManagementToken: app.settings.Auth.Token,
	}
	if value.Preferences.ListenHost == "" {
		value.Preferences.ListenHost = "127.0.0.1"
	}
	if value.Preferences.ListenPort == 0 {
		value.Preferences.ListenPort = 3000
	}
	if document != nil {
		if err := json.Unmarshal(document, &value); err != nil {
			return storedPanelSettings{}, 0, errors.New("stored panel settings are invalid")
		}
	}
	return value, revision, nil
}

func (app *Application) PanelSettings(ctx context.Context) (PanelSettingsView, error) {
	value, revision, err := app.storedPanelSettings(ctx)
	if err != nil {
		return PanelSettingsView{}, err
	}
	view := app.panelSettingsView(value, revision)
	if app.publicIP != nil {
		view.DetectedPublicIP = app.publicIP(ctx)
	}
	return view, nil
}

func (app *Application) panelSettingsView(value storedPanelSettings, revision int64) PanelSettingsView {
	p := value.Preferences
	return PanelSettingsView{
		Revision: revision, Preferences: p,
		GitHubTokenConfigured: value.GitHubToken != "", IdentityKeyConfigured: value.IdentityKey != "",
		RestartRequired: revision > 0 && (p.ListenHost != app.settings.Server.Host || p.ListenPort != app.settings.Server.Port || p.ExternalOrigin != app.settings.Server.ExternalOrigin),
	}
}

func (app *Application) SavePanelSettings(ctx context.Context, input PanelSettingsWrite) (PanelSettingsView, error) {
	if err := validatePanelSettings(input); err != nil {
		return PanelSettingsView{}, err
	}
	value, revision, err := app.storedPanelSettings(ctx)
	if err != nil {
		return PanelSettingsView{}, err
	}
	if input.Revision != revision {
		return PanelSettingsView{}, store.ErrPanelSettingsConflict
	}
	identityChanged := input.Preferences.IdentityName != value.Preferences.IdentityName || (input.IdentityKey != "" && input.IdentityKey != value.IdentityKey)
	value.Preferences = input.Preferences
	value.Preferences.Appearance.Color = strings.ToUpper(input.Preferences.Appearance.Color)
	if input.GitHubToken != "" {
		value.GitHubToken = input.GitHubToken
	}
	if input.ClearGitHubToken {
		value.GitHubToken = ""
	}
	if input.IdentityKey != "" {
		value.IdentityKey = input.IdentityKey
	}
	if input.ManagementToken != "" {
		value.ManagementToken = input.ManagementToken
	}
	document, err := json.Marshal(value)
	if err != nil {
		return PanelSettingsView{}, errors.New("encode panel settings failed")
	}
	var configuration *store.ConfigurationFileUpdate
	if identityChanged {
		configuration, err = app.identityConfigurationUpdate(ctx, value)
		if err != nil {
			return PanelSettingsView{}, err
		}
	}
	revision, err = app.database.SavePanelSettings(ctx, document, revision, configuration)
	if err != nil {
		return PanelSettingsView{}, err
	}
	view := app.panelSettingsView(value, revision)
	if app.publicIP != nil {
		view.DetectedPublicIP = app.publicIP(ctx)
	}
	return view, nil
}

func validatePanelSettings(input PanelSettingsWrite) error {
	p := input.Preferences
	if input.Revision < 0 || (net.ParseIP(p.ListenHost) == nil && p.ListenHost != "localhost") || p.ListenPort < 1 || p.ListenPort > 65535 ||
		!panelColor.MatchString(p.Appearance.Color) || p.Appearance.Radius < 0 || p.Appearance.Radius > 32 ||
		(p.Appearance.Theme != "light" && p.Appearance.Theme != "dark" && p.Appearance.Theme != "system") ||
		(p.Language != "zh-CN" && p.Language != "en") || len(p.IdentityName) > 128 || strings.ContainsAny(p.IdentityName, "\x00\r\n") ||
		(p.TrafficQuotaGiB != nil && (*p.TrafficQuotaGiB < 0 || *p.TrafficQuotaGiB > 1_000_000_000)) {
		return ErrPanelSettingsInvalid
	}
	if p.ExternalOrigin != "" {
		origin, err := settings.NormalizeOrigin(p.ExternalOrigin)
		if err != nil || origin != p.ExternalOrigin {
			return ErrPanelSettingsInvalid
		}
	}
	if p.PublicNodeHost != "" && !validPublishedHost(p.PublicNodeHost) {
		return ErrPanelSettingsInvalid
	}
	for _, secret := range []string{input.GitHubToken, input.IdentityKey, input.ManagementToken} {
		if len(secret) > 8192 || strings.ContainsAny(secret, "\x00\r\n") {
			return ErrPanelSettingsInvalid
		}
	}
	// Login clients trim whitespace, including the JavaScript BOM character.
	// Reject ambiguous replacements before persisting or invalidating sessions.
	token := input.ManagementToken
	trimmedToken := strings.TrimFunc(token, func(r rune) bool { return unicode.IsSpace(r) || r == '\ufeff' })
	if token != "" && (len(token) < 32 || token != trimmedToken) {
		return ErrPanelSettingsInvalid
	}
	if input.GitHubToken != "" && input.ClearGitHubToken {
		return ErrPanelSettingsInvalid
	}
	return nil
}

func validPublishedHost(host string) bool {
	if address, err := netip.ParseAddr(strings.Trim(host, "[]")); err == nil {
		return address.Zone() == "" && address.IsGlobalUnicast() && !address.IsLoopback() && !address.IsPrivate()
	}
	if len(host) > 253 || strings.EqualFold(host, "localhost") {
		return false
	}
	for _, label := range strings.Split(strings.TrimSuffix(host, "."), ".") {
		if !hostnameLabel.MatchString(label) {
			return false
		}
	}
	return strings.Contains(host, ".")
}

// EffectiveSettings overlays persisted settings without mutating shared process
// configuration. Listener and origin changes take effect on panel restart;
// credentials and quota can be read by their consumers at operation boundaries.
func (app *Application) EffectiveSettings(ctx context.Context) (settings.Settings, error) {
	value, _, err := app.storedPanelSettings(ctx)
	if err != nil {
		return settings.Settings{}, err
	}
	result := app.settings
	result.Server.Host = value.Preferences.ListenHost
	result.Server.Port = value.Preferences.ListenPort
	result.Server.ExternalOrigin = value.Preferences.ExternalOrigin
	result.Auth.SecureCookie = strings.HasPrefix(result.Server.ExternalOrigin, "https://")
	result.GitHub.Token = value.GitHubToken
	result.Auth.Token = value.ManagementToken
	result.Traffic.QuotaGiB = value.Preferences.TrafficQuotaGiB
	return result, nil
}
