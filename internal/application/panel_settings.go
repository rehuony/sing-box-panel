// SPDX-License-Identifier: GPL-3.0-or-later

package application

import (
	"context"
	"errors"
	"fmt"
	"net"
	"slices"
	"strings"
	"unicode"

	"github.com/rehuony/sing-box-panel/internal/settings"
	"github.com/rehuony/sing-box-panel/internal/store"
)

type AppearanceSettings = settings.Appearance

// PanelPreferences is safe to return to authenticated clients. Credentials are
// write-only and stored separately from this response projection.
type PanelPreferences struct {
	ListenHost      string             `json:"listen_host"`
	ListenPort      int                `json:"listen_port"`
	ExternalOrigin  string             `json:"external_origin"`
	PublicNodeHost  string             `json:"public_node_host"`
	TrafficQuotaGiB *int64             `json:"traffic_quota_gib"`
	Language        string             `json:"language"`
	Appearance      AppearanceSettings `json:"appearance"`
}

// PanelServiceSettings exposes every non-secret service option from setting.json.
// Writes are optional so existing clients preserve options they do not edit.
type PanelServiceSettings struct {
	DataDir                     string   `json:"data_dir"`
	BasePath                    string   `json:"base_path"`
	SecureCookie                bool     `json:"secure_cookie"`
	CatalogRefreshIntervalHours int      `json:"catalog_refresh_interval_hours"`
	TrafficPeriodMonths         int      `json:"traffic_period_months"`
	SampleRetentionDays         int      `json:"sample_retention_days"`
	PrivateSourceCIDRs          []string `json:"private_source_cidrs"`
	CoreLogRetentionDays        *int     `json:"core_log_retention_days,omitempty"`
	CoreLogMaxFiles             *int     `json:"core_log_max_files,omitempty"`
	CoreLogMaxFileSizeMiB       *int     `json:"core_log_max_file_size_mib,omitempty"`
}

func serviceSettings(value settings.Settings) PanelServiceSettings {
	return PanelServiceSettings{
		DataDir: value.DataDir, BasePath: value.Server.BasePath, SecureCookie: value.Auth.SecureCookie,
		CatalogRefreshIntervalHours: value.GitHub.CatalogRefreshIntervalHours, TrafficPeriodMonths: value.Traffic.PeriodMonths,
		SampleRetentionDays: value.Traffic.SampleRetentionDays, PrivateSourceCIDRs: append([]string{}, value.Subscription.PrivateSourceCIDRs...),
		CoreLogRetentionDays: &value.Logs.CoreRetentionDays, CoreLogMaxFiles: &value.Logs.CoreMaxFiles, CoreLogMaxFileSizeMiB: &value.Logs.CoreMaxFileSizeMiB,
	}
}

func (service PanelServiceSettings) apply(value *settings.Settings) {
	value.DataDir, value.Server.BasePath, value.Auth.SecureCookie = service.DataDir, service.BasePath, service.SecureCookie
	value.GitHub.CatalogRefreshIntervalHours = service.CatalogRefreshIntervalHours
	value.Traffic.PeriodMonths, value.Traffic.SampleRetentionDays = service.TrafficPeriodMonths, service.SampleRetentionDays
	value.Subscription.PrivateSourceCIDRs = slices.Clone(service.PrivateSourceCIDRs)
	if service.CoreLogRetentionDays != nil {
		value.Logs.CoreRetentionDays = *service.CoreLogRetentionDays
	}
	if service.CoreLogMaxFiles != nil {
		value.Logs.CoreMaxFiles = *service.CoreLogMaxFiles
	}
	if service.CoreLogMaxFileSizeMiB != nil {
		value.Logs.CoreMaxFileSizeMiB = *service.CoreLogMaxFileSizeMiB
	}
}

type PanelSettingsView struct {
	Service               PanelServiceSettings `json:"service"`
	DetectedPublicIP      string               `json:"detected_public_ip,omitempty"`
	Revision              int64                `json:"revision"`
	Preferences           PanelPreferences     `json:"preferences"`
	GitHubTokenConfigured bool                 `json:"github_token_configured"`
	RestartRequired       bool                 `json:"restart_required"`
}

type PanelSettingsWrite struct {
	Service          *PanelServiceSettings `json:"service,omitempty"`
	Revision         int64                 `json:"revision"`
	Preferences      PanelPreferences      `json:"preferences"`
	GitHubToken      string                `json:"github_token,omitempty"`
	ClearGitHubToken bool                  `json:"clear_github_token,omitempty"`
	ManagementToken  string                `json:"management_token,omitempty"`
}

type storedPanelSettings struct {
	Preferences     PanelPreferences `json:"preferences"`
	GitHubToken     string           `json:"github_token"`
	ManagementToken string           `json:"management_token"`
}

var ErrPanelSettingsInvalid = errors.New("panel settings are invalid")

func (app *Application) PanelSettings(ctx context.Context) (PanelSettingsView, error) {
	value, revision, err := app.currentSettings(ctx)
	if err != nil {
		return PanelSettingsView{}, err
	}
	view := app.panelSettingsView(value, revision)
	if app.publicIP != nil {
		view.DetectedPublicIP = app.publicIP(ctx)
	}
	return view, nil
}

func (app *Application) panelSettingsView(configuration settings.Settings, revision int64) PanelSettingsView {
	value := panelValues(configuration)
	p := value.Preferences
	loaded := app.settings
	restartRequired := configuration.Server != loaded.Server || configuration.DataDir != loaded.DataDir ||
		configuration.Auth.SecureCookie != loaded.Auth.SecureCookie
	return PanelSettingsView{
		Revision: revision, Preferences: p, Service: serviceSettings(configuration),
		GitHubTokenConfigured: value.GitHubToken != "",
		RestartRequired:       restartRequired,
	}
}

func (app *Application) SavePanelSettings(ctx context.Context, input PanelSettingsWrite) (PanelSettingsView, error) {
	if err := validatePanelSettings(input); err != nil {
		return PanelSettingsView{}, err
	}
	if app.settingsPath == "" {
		return PanelSettingsView{}, errors.New("panel settings file path is required")
	}
	lock, err := settings.Lock(ctx, app.settingsPath)
	if err != nil {
		return PanelSettingsView{}, err
	}
	defer lock.Close()
	if err := app.recoverSettingsFile(ctx); err != nil {
		return PanelSettingsView{}, err
	}
	before, err := settings.Read(app.settingsPath)
	if err != nil {
		return PanelSettingsView{}, err
	}
	configurationFile, err := settings.Parse(app.settingsPath, before)
	if err != nil {
		return PanelSettingsView{}, err
	}
	revision := settings.Revision(before)
	value := panelValues(configurationFile)
	if input.Revision != revision {
		return PanelSettingsView{}, store.ErrPanelSettingsConflict
	}
	value.Preferences = input.Preferences
	value.Preferences.Appearance.Color = strings.ToUpper(input.Preferences.Appearance.Color)
	if input.GitHubToken != "" {
		value.GitHubToken = input.GitHubToken
	}
	if input.ClearGitHubToken {
		value.GitHubToken = ""
	}
	if input.ManagementToken != "" {
		value.ManagementToken = input.ManagementToken
	}
	originalDataDir := configurationFile.DataDir
	applyPanelValues(&configurationFile, value)
	if input.Service != nil {
		input.Service.apply(&configurationFile)
	}
	if err := configurationFile.Validate(); err != nil {
		return PanelSettingsView{}, fmt.Errorf("%w: %s", ErrPanelSettingsInvalid, err)
	}
	after, err := encodeSettings(configurationFile, before, configurationFile.DataDir == originalDataDir)
	if err != nil {
		return PanelSettingsView{}, err
	}
	if err := app.commitSettingsFile(ctx, before, after, nil); err != nil {
		return PanelSettingsView{}, err
	}
	app.publishSettings(configurationFile)
	revision = settings.Revision(after)
	view := app.panelSettingsView(configurationFile, revision)
	if app.publicIP != nil {
		view.DetectedPublicIP = app.publicIP(ctx)
	}
	return view, nil
}

func validatePanelSettings(input PanelSettingsWrite) error {
	p := input.Preferences
	if err := panelFields(p).Validate(); err != nil {
		return ErrPanelSettingsInvalid
	}
	if input.Revision < 0 || (net.ParseIP(p.ListenHost) == nil && p.ListenHost != "localhost") || p.ListenPort < 1 || p.ListenPort > 65535 ||
		settings.ValidateTrafficQuota(p.TrafficQuotaGiB) != nil {
		return ErrPanelSettingsInvalid
	}
	if p.ExternalOrigin != "" {
		origin, err := settings.NormalizeOrigin(p.ExternalOrigin)
		if err != nil || origin != p.ExternalOrigin {
			return ErrPanelSettingsInvalid
		}
	}
	if p.PublicNodeHost != "" && !settings.ValidPublishedHost(p.PublicNodeHost) {
		return ErrPanelSettingsInvalid
	}
	for _, secret := range []string{input.GitHubToken, input.ManagementToken} {
		if len(secret) > 8192 || strings.ContainsAny(secret, "\x00\r\n") {
			return ErrPanelSettingsInvalid
		}
	}
	// Login clients trim whitespace, including the JavaScript BOM character.
	// Reject ambiguous replacements before persisting or invalidating sessions.
	token := input.ManagementToken
	trimmedToken := strings.TrimFunc(token, func(r rune) bool { return unicode.IsSpace(r) || r == '\ufeff' })
	if token != "" && (len(token) < 8 || token != trimmedToken) {
		return ErrPanelSettingsInvalid
	}
	if input.GitHubToken != "" && input.ClearGitHubToken {
		return ErrPanelSettingsInvalid
	}
	return nil
}

// EffectiveSettings reads the shared file. The running listener remains pinned
// until restart; operation-boundary consumers see current credentials and quota.
func (app *Application) EffectiveSettings(ctx context.Context) (settings.Settings, error) {
	value, _, err := app.currentSettings(ctx)
	return value, err
}
