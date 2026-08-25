// SPDX-License-Identifier: GPL-3.0-or-later

// Package settings owns the process bootstrap configuration contract.
package settings

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	pathpkg "path"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"

	"github.com/rehuony/sing-box-panel/internal/jsonstrict"
)

const maxSettingsBytes = 1 << 20

var basePathPattern = regexp.MustCompile(`^/[A-Za-z0-9._~/-]+$`)

// Settings contains only process-bootstrap configuration. Mutable product state
// belongs in SQLite.
type Settings struct {
	Server       Server       `json:"server"`
	DataDir      string       `json:"data_dir"`
	Auth         Auth         `json:"auth"`
	GitHub       GitHub       `json:"github"`
	Traffic      Traffic      `json:"traffic"`
	Subscription Subscription `json:"subscription"`
	Logs         Logs         `json:"logs"`
}

type Server struct {
	Host     string `json:"host"`
	Port     int    `json:"port"`
	BasePath string `json:"base_path"`
}

type Auth struct {
	Token        string `json:"token"`
	SecureCookie bool   `json:"secure_cookie"`
}

type GitHub struct {
	Token           string `json:"token"`
	CatalogTTLHours int    `json:"catalog_ttl_hours"`
}

type Traffic struct {
	QuotaGiB     *int64 `json:"quota_gib"`
	PeriodMonths int    `json:"period_months"`
}

type Subscription struct {
	Author             string   `json:"author"`
	Provider           string   `json:"provider"`
	PrivateSourceCIDRs []string `json:"private_source_cidrs"`
}

type Logs struct {
	RetentionDays int `json:"retention_days"`
}

// Defaults returns safe defaults for the current effective user.
func Defaults(configPath string) Settings {
	dataDir := defaultDataDir()
	return Settings{
		Server:  Server{Host: "127.0.0.1", Port: 3000},
		DataDir: dataDir,
		GitHub:  GitHub{CatalogTTLHours: 12},
		Traffic: Traffic{PeriodMonths: 1},
		Subscription: Subscription{
			Author:             "reagin",
			Provider:           "ZgoCloud",
			PrivateSourceCIDRs: []string{},
		},
		Logs: Logs{RetentionDays: 7},
	}
}

// DefaultPath returns the root or XDG settings path.
func DefaultPath() string {
	if os.Geteuid() == 0 {
		return "/etc/sing-box-panel/setting.json"
	}
	configHome := strings.TrimSpace(os.Getenv("XDG_CONFIG_HOME"))
	if configHome == "" {
		home, err := os.UserHomeDir()
		if err != nil || home == "" {
			return "setting.json"
		}
		configHome = filepath.Join(home, ".config")
	}
	return filepath.Join(configHome, "sing-box-panel", "setting.json")
}

func defaultDataDir() string {
	if os.Geteuid() == 0 {
		return "/var/lib/sing-box-panel"
	}
	dataHome := strings.TrimSpace(os.Getenv("XDG_DATA_HOME"))
	if dataHome == "" {
		home, err := os.UserHomeDir()
		if err != nil || home == "" {
			return "data"
		}
		dataHome = filepath.Join(home, ".local", "share")
	}
	return filepath.Join(dataHome, "sing-box-panel")
}

// Load parses and validates one settings file.
func Load(path string) (Settings, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Settings{}, fmt.Errorf("read settings %q: %w", path, err)
	}
	var value Settings
	if err := jsonstrict.Decode(data, maxSettingsBytes, &value); err != nil {
		return Settings{}, fmt.Errorf("parse settings %q: %w", path, err)
	}
	if !filepath.IsAbs(value.DataDir) {
		value.DataDir = filepath.Join(filepath.Dir(path), value.DataDir)
	}
	value.DataDir = filepath.Clean(value.DataDir)
	if err := value.Validate(); err != nil {
		return Settings{}, fmt.Errorf("validate settings %q: %w", path, err)
	}
	return value, nil
}

// Validate verifies the complete resolved settings contract.
func (value Settings) Validate() error {
	if net.ParseIP(value.Server.Host) == nil && value.Server.Host != "localhost" {
		return errors.New("server.host must be an IP address or localhost")
	}
	if value.Server.Port < 1 || value.Server.Port > 65535 {
		return errors.New("server.port must be between 1 and 65535")
	}
	if value.Server.BasePath != "" {
		if !basePathPattern.MatchString(value.Server.BasePath) ||
			strings.HasSuffix(value.Server.BasePath, "/") ||
			strings.Contains(value.Server.BasePath, "//") ||
			pathpkg.Clean(value.Server.BasePath) != value.Server.BasePath {
			return errors.New("server.base_path must be empty or a normalized URL path containing only unreserved characters")
		}
	}
	if value.DataDir == "" || !filepath.IsAbs(value.DataDir) {
		return errors.New("data_dir must resolve to an absolute path")
	}
	if strings.TrimSpace(value.Auth.Token) == "" {
		return errors.New("auth.token must not be empty")
	}
	if value.GitHub.CatalogTTLHours < 1 || value.GitHub.CatalogTTLHours > 24*30 {
		return errors.New("github.catalog_ttl_hours must be between 1 and 720")
	}
	if value.Traffic.QuotaGiB != nil && *value.Traffic.QuotaGiB < 0 {
		return errors.New("traffic.quota_gib must be null or non-negative")
	}
	if value.Traffic.PeriodMonths < 1 || value.Traffic.PeriodMonths > 120 {
		return errors.New("traffic.period_months must be between 1 and 120")
	}
	if strings.TrimSpace(value.Subscription.Author) == "" || strings.TrimSpace(value.Subscription.Provider) == "" {
		return errors.New("subscription.author and subscription.provider must not be empty")
	}
	for _, raw := range value.Subscription.PrivateSourceCIDRs {
		if _, _, err := net.ParseCIDR(raw); err != nil {
			return fmt.Errorf("subscription.private_source_cidrs contains invalid CIDR %q", raw)
		}
	}
	if value.Logs.RetentionDays < 1 || value.Logs.RetentionDays > 3650 {
		return errors.New("logs.retention_days must be between 1 and 3650")
	}
	if runtime.GOOS == "windows" {
		return errors.New("Windows is not supported")
	}
	return nil
}

// Initialize writes a new settings file and creates its data directory.
func Initialize(path string, overwrite bool) (Settings, error) {
	if !overwrite {
		if _, err := os.Stat(path); err == nil {
			return Settings{}, fmt.Errorf("settings file %q already exists", path)
		} else if !errors.Is(err, os.ErrNotExist) {
			return Settings{}, fmt.Errorf("inspect settings %q: %w", path, err)
		}
	}
	value := Defaults(path)
	token, err := randomToken(32)
	if err != nil {
		return Settings{}, err
	}
	value.Auth.Token = token
	if err := value.Validate(); err != nil {
		return Settings{}, err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return Settings{}, fmt.Errorf("create settings directory: %w", err)
	}
	if err := os.MkdirAll(value.DataDir, 0o700); err != nil {
		return Settings{}, fmt.Errorf("create data directory: %w", err)
	}
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return Settings{}, fmt.Errorf("encode settings: %w", err)
	}
	data = append(data, '\n')
	if err := atomicWrite(path, data, 0o600); err != nil {
		return Settings{}, err
	}
	return value, nil
}

func randomToken(size int) (string, error) {
	raw := make([]byte, size)
	if _, err := rand.Read(raw); err != nil {
		return "", fmt.Errorf("generate token: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(raw), nil
}

func atomicWrite(path string, data []byte, mode os.FileMode) error {
	directory := filepath.Dir(path)
	temporary, err := os.CreateTemp(directory, ".setting-*.tmp")
	if err != nil {
		return fmt.Errorf("create temporary settings: %w", err)
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if err := temporary.Chmod(mode); err != nil {
		temporary.Close()
		return fmt.Errorf("set temporary settings permissions: %w", err)
	}
	if _, err := temporary.Write(data); err != nil {
		temporary.Close()
		return fmt.Errorf("write temporary settings: %w", err)
	}
	if err := temporary.Sync(); err != nil {
		temporary.Close()
		return fmt.Errorf("sync temporary settings: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("close temporary settings: %w", err)
	}
	if err := os.Rename(temporaryPath, path); err != nil {
		return fmt.Errorf("replace settings: %w", err)
	}
	dir, err := os.Open(directory)
	if err != nil {
		return fmt.Errorf("open settings directory: %w", err)
	}
	defer dir.Close()
	if err := dir.Sync(); err != nil {
		return fmt.Errorf("sync settings directory: %w", err)
	}
	return nil
}
