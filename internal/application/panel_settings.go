// SPDX-License-Identifier: GPL-3.0-or-later

package application

import (
	"context"
	"errors"
	"fmt"
	"net"
	"slices"
	"strings"

	"github.com/rehuony/sing-box-panel/internal/auth"
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
		DataDir: value.DataDir, BasePath: value.Server.BasePath,
		CatalogRefreshIntervalHours: value.GitHub.CatalogRefreshIntervalHours, TrafficPeriodMonths: value.Traffic.PeriodMonths,
		SampleRetentionDays: value.Traffic.SampleRetentionDays, PrivateSourceCIDRs: append([]string{}, value.Subscription.PrivateSourceCIDRs...),
		CoreLogRetentionDays: &value.Logs.CoreRetentionDays, CoreLogMaxFiles: &value.Logs.CoreMaxFiles, CoreLogMaxFileSizeMiB: &value.Logs.CoreMaxFileSizeMiB,
	}
}

func (service PanelServiceSettings) apply(value *settings.Settings) {
	value.DataDir, value.Server.BasePath = service.DataDir, service.BasePath
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
	AdminEmail            string               `json:"admin_email"`
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
	Credentials      *CredentialsWrite     `json:"credentials,omitempty"`
}

type CredentialsWrite struct {
	Email       *string `json:"email,omitempty"`
	NewPassword string  `json:"new_password,omitempty"`
}

type PanelSettingsSaveResult struct {
	Settings                 PanelSettingsView `json:"settings"`
	ReauthenticationRequired bool              `json:"reauthentication_required"`
}

type storedPanelSettings struct {
	Preferences PanelPreferences `json:"preferences"`
	GitHubToken string           `json:"github_token"`
	Auth        settings.Auth    `json:"-"`
}

var ErrPanelSettingsInvalid = errors.New("panel settings are invalid")

// PanelAppearance returns only the public visual preferences needed before login.
func (app *Application) PanelAppearance(ctx context.Context) (AppearanceSettings, error) {
	value, _, err := app.currentSettings(ctx)
	return value.Panel.Appearance, err
}

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
	restartRequired := configuration.Server != loaded.Server || configuration.DataDir != loaded.DataDir
	return PanelSettingsView{
		AdminEmail: configuration.Auth.Email, Revision: revision, Preferences: p, Service: serviceSettings(configuration),
		GitHubTokenConfigured: value.GitHubToken != "",
		RestartRequired:       restartRequired,
	}
}

func (app *Application) SavePanelSettings(ctx context.Context, input PanelSettingsWrite) (PanelSettingsSaveResult, error) {
	if err := validatePanelSettings(input); err != nil {
		return PanelSettingsSaveResult{}, err
	}
	if app.settingsPath == "" {
		return PanelSettingsSaveResult{}, errors.New("panel settings file path is required")
	}
	lock, err := settings.Lock(ctx, app.settingsPath)
	if err != nil {
		return PanelSettingsSaveResult{}, err
	}
	defer lock.Close()
	if err := app.recoverSettingsFile(ctx); err != nil {
		return PanelSettingsSaveResult{}, err
	}
	before, err := settings.Read(app.settingsPath)
	if err != nil {
		return PanelSettingsSaveResult{}, err
	}
	configurationFile, err := settings.Parse(app.settingsPath, before)
	if err != nil {
		return PanelSettingsSaveResult{}, err
	}
	revision := settings.Revision(before)
	value := panelValues(configurationFile)
	if input.Revision != revision {
		return PanelSettingsSaveResult{}, store.ErrPanelSettingsConflict
	}
	value.Preferences = input.Preferences
	value.Preferences.Appearance.Color = strings.ToUpper(input.Preferences.Appearance.Color)
	if input.GitHubToken != "" {
		value.GitHubToken = input.GitHubToken
	}
	if input.ClearGitHubToken {
		value.GitHubToken = ""
	}
	credentialsChanged := false
	if credentials := input.Credentials; credentials != nil {
		if credentials.Email != nil {
			email, err := auth.NormalizeEmail(*credentials.Email)
			if err != nil {
				return PanelSettingsSaveResult{}, ErrPanelSettingsInvalid
			}
			credentialsChanged = email != value.Auth.Email
			value.Auth.Email = email
		}
		if credentials.NewPassword != "" {
			same, err := auth.VerifyPassword(ctx, credentials.NewPassword, value.Auth.PasswordHash)
			if err != nil {
				return PanelSettingsSaveResult{}, err
			}
			if !same {
				value.Auth.PasswordHash, err = auth.HashPassword(ctx, credentials.NewPassword)
				if err != nil {
					return PanelSettingsSaveResult{}, err
				}
				credentialsChanged = true
			}
		}
	}
	originalDataDir := configurationFile.DataDir
	applyPanelValues(&configurationFile, value)
	if input.Service != nil {
		input.Service.apply(&configurationFile)
	}
	if err := configurationFile.Validate(); err != nil {
		return PanelSettingsSaveResult{}, fmt.Errorf("%w: %s", ErrPanelSettingsInvalid, err)
	}
	after, err := encodeSettings(configurationFile, before, configurationFile.DataDir == originalDataDir)
	if err != nil {
		return PanelSettingsSaveResult{}, err
	}
	if err := app.commitSettingsFile(ctx, before, after, nil); err != nil {
		return PanelSettingsSaveResult{}, err
	}
	app.publishSettings(configurationFile)
	revision = settings.Revision(after)
	view := app.panelSettingsView(configurationFile, revision)
	if app.publicIP != nil {
		view.DetectedPublicIP = app.publicIP(ctx)
	}
	return PanelSettingsSaveResult{Settings: view, ReauthenticationRequired: credentialsChanged}, nil
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
	for _, secret := range []string{input.GitHubToken} {
		if len(secret) > 8192 || strings.ContainsAny(secret, "\x00\r\n") {
			return ErrPanelSettingsInvalid
		}
	}
	if credentials := input.Credentials; credentials != nil {
		if credentials.Email != nil {
			if _, err := auth.NormalizeEmail(*credentials.Email); err != nil {
				return ErrPanelSettingsInvalid
			}
		}
		if credentials.NewPassword != "" && auth.ValidatePassword(credentials.NewPassword) != nil {
			return ErrPanelSettingsInvalid
		}
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
