// SPDX-License-Identifier: GPL-3.0-or-later

// Package settings owns the panel configuration file contract.
package settings

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	pathpkg "path"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
)

// MaximumBytes limits settings documents read from disk or supplied by the CLI.
const MaximumBytes = 1 << 20

var basePathPattern = regexp.MustCompile(`^/[A-Za-z0-9._~/-]+$`)

// Settings is the shared file contract for panel configuration. Sing-box
// documents, subscriptions, tasks, and runtime evidence belong in SQLite.
type Settings struct {
	sourcePath   string
	Panel        Panel        `json:"panel"`
	Server       Server       `json:"server"`
	DataDir      string       `json:"data_dir"`
	Auth         Auth         `json:"auth"`
	GitHub       GitHub       `json:"github"`
	Traffic      Traffic      `json:"traffic"`
	Subscription Subscription `json:"subscription"`
	Logs         Logs         `json:"logs"`
}

type Server struct {
	Host           string `json:"host"`
	Port           int    `json:"port"`
	BasePath       string `json:"base_path"`
	ExternalOrigin string `json:"external_origin"`
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
	QuotaGiB            *int64 `json:"quota_gib"`
	PeriodMonths        int    `json:"period_months"`
	SampleRetentionDays int    `json:"sample_retention_days"`
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
func Defaults() Settings {
	dataDir := defaultDataDir()
	return Settings{
		Panel:   DefaultPanel(),
		Server:  Server{Host: "127.0.0.1", Port: 3000},
		DataDir: dataDir,
		GitHub:  GitHub{CatalogTTLHours: 12},
		Traffic: Traffic{PeriodMonths: 1, SampleRetentionDays: 90},
		Subscription: Subscription{
			Author:             "reagin",
			Provider:           "default",
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
	data, err := Read(path)
	if err != nil {
		return Settings{}, err
	}
	return parse(path, data)
}

func parse(path string, data []byte) (Settings, error) {
	absolutePath, err := filepath.Abs(path)
	if err != nil {
		return Settings{}, err
	}
	value := Settings{sourcePath: absolutePath, Panel: DefaultPanel()}
	if err := decodeSettings(path, data, &value); err != nil {
		return Settings{}, err
	}
	value.DataDir, err = resolveDataDir(path, value.DataDir)
	if err != nil {
		return Settings{}, err
	}
	if value.Server.ExternalOrigin != "" {
		origin, err := NormalizeOrigin(value.Server.ExternalOrigin)
		if err != nil {
			return Settings{}, fmt.Errorf("validate settings %q: server.external_origin: %w", path, err)
		}
		value.Server.ExternalOrigin = origin
	}
	if err := value.Validate(); err != nil {
		return Settings{}, fmt.Errorf("validate settings %q: %w", path, err)
	}
	return value, nil
}

// LoadOrInitialize loads settings, creating defaults only when the selected file
// is absent. Concurrent callers use the same atomically published settings.
func LoadOrInitialize(path string) (value Settings, created bool, err error) {
	value, err = LoadForStartup(path)
	if !errors.Is(err, os.ErrNotExist) {
		return value, false, err
	}
	value, err = Initialize(path, false)
	if errors.Is(err, os.ErrExist) {
		value, err = Load(path)
		return value, false, err
	}
	return value, err == nil, err
}

// Validate verifies the complete resolved settings contract.
func (value Settings) Validate() error {
	if err := value.Panel.Validate(); err != nil {
		return err
	}
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
	if value.Server.ExternalOrigin != "" {
		origin, err := NormalizeOrigin(value.Server.ExternalOrigin)
		if err != nil {
			return fmt.Errorf("server.external_origin: %w", err)
		}
		if origin != value.Server.ExternalOrigin {
			return errors.New("server.external_origin must be a normalized HTTP or HTTPS origin")
		}
		usesHTTPS := strings.HasPrefix(origin, "https://")
		if usesHTTPS != value.Auth.SecureCookie {
			return errors.New("auth.secure_cookie must be true exactly when server.external_origin uses HTTPS")
		}
	} else if value.Auth.SecureCookie {
		return errors.New("server.external_origin must be configured when auth.secure_cookie is true")
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
	if err := ValidateTrafficQuota(value.Traffic.QuotaGiB); err != nil {
		return err
	}
	if value.Traffic.PeriodMonths < 1 || value.Traffic.PeriodMonths > 120 {
		return errors.New("traffic.period_months must be between 1 and 120")
	}
	if value.Traffic.SampleRetentionDays < 1 || value.Traffic.SampleRetentionDays > 366 {
		return errors.New("traffic.sample_retention_days must be between 1 and 366")
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

// NormalizeOrigin validates an HTTP Origin value and returns its canonical
// scheme and authority. Paths, credentials, queries, and fragments are never
// part of an origin and are rejected instead of silently discarded.
func NormalizeOrigin(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", errors.New("origin must not be empty")
	}
	parsed, err := url.Parse(raw)
	if err != nil {
		return "", fmt.Errorf("parse origin: %w", err)
	}
	scheme := strings.ToLower(parsed.Scheme)
	if scheme != "http" && scheme != "https" {
		return "", errors.New("origin scheme must be http or https")
	}
	if parsed.Opaque != "" || parsed.User != nil || parsed.Host == "" || parsed.Path != "" ||
		parsed.RawPath != "" || parsed.ForceQuery || parsed.RawQuery != "" || parsed.Fragment != "" {
		return "", errors.New("origin must contain only a scheme and authority")
	}
	host := strings.ToLower(parsed.Hostname())
	if host == "" || strings.Contains(host, "%") {
		return "", errors.New("origin host is invalid")
	}
	port := parsed.Port()
	if port != "" {
		numericPort, err := strconv.Atoi(port)
		if err != nil || numericPort < 1 || numericPort > 65535 {
			return "", errors.New("origin port must be between 1 and 65535")
		}
		if (scheme == "http" && port == "80") || (scheme == "https" && port == "443") {
			port = ""
		}
	}
	if ip := net.ParseIP(host); ip != nil {
		host = ip.String()
	}
	authority := host
	if port != "" {
		authority = net.JoinHostPort(host, port)
	} else if strings.Contains(host, ":") {
		authority = "[" + host + "]"
	}
	return scheme + "://" + authority, nil
}

// Initialize writes a new settings file and creates its data directory.
func Initialize(path string, overwrite bool) (Settings, error) {
	return initialize(context.Background(), path, overwrite, true)
}

// InitializeFile creates default settings without creating the data directory
// or opening its database. Existing files require an explicit overwrite.
func InitializeFile(ctx context.Context, path string, overwrite bool) (Settings, error) {
	return initialize(ctx, path, overwrite, false)
}

func initialize(ctx context.Context, path string, overwrite, createDataDirectory bool) (Settings, error) {
	if err := ctx.Err(); err != nil {
		return Settings{}, err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return Settings{}, err
	}
	lock, err := Lock(ctx, path)
	if err != nil {
		return Settings{}, err
	}
	defer lock.Close()
	if err := CheckPending(path); err != nil {
		return Settings{}, err
	}
	if location, err := ReadDataLocation(path); err == nil && location.Move != nil {
		return Settings{}, errors.New("finish the data directory migration before initializing settings")
	} else if err != nil && !errors.Is(err, os.ErrNotExist) {
		return Settings{}, err
	}
	if info, err := os.Lstat(path); err == nil {
		if !overwrite {
			return Settings{}, fmt.Errorf("settings file %q already exists: %w", path, os.ErrExist)
		}
		if !info.Mode().IsRegular() {
			return Settings{}, fmt.Errorf("settings destination %q must be a regular file", path)
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return Settings{}, fmt.Errorf("inspect settings %q: %w", path, err)
	}
	value := Defaults()
	value.sourcePath, err = filepath.Abs(path)
	if err != nil {
		return Settings{}, err
	}
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
	if createDataDirectory {
		if err := os.MkdirAll(value.DataDir, 0o700); err != nil {
			return Settings{}, fmt.Errorf("create data directory: %w", err)
		}
	}
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return Settings{}, fmt.Errorf("encode settings: %w", err)
	}
	data = append(data, '\n')
	if err := ctx.Err(); err != nil {
		return Settings{}, err
	}
	if old, err := ConfiguredDataDir(path); err == nil {
		if err := RememberDataLocation(path, old); err != nil {
			return Settings{}, err
		}
	}
	if err := atomicWrite(path, data, 0o600, overwrite); err != nil {
		return Settings{}, err
	}
	if err := RememberDataLocation(path, value.DataDir); err != nil {
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

func atomicWrite(path string, data []byte, mode os.FileMode, overwrite bool) error {
	directory := filepath.Dir(path)
	temporary, err := os.CreateTemp(directory, ".setting-*.tmp")
	if err != nil {
		return fmt.Errorf("create temporary settings: %w", err)
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if err := preserveOwner(temporary, path); err != nil {
		temporary.Close()
		return err
	}
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
	if overwrite {
		if err := os.Rename(temporaryPath, path); err != nil {
			return fmt.Errorf("replace settings: %w", err)
		}
	} else if err := os.Link(temporaryPath, path); err != nil {
		// Publishing a complete file without replacement prevents concurrent
		// first starts from overwriting each other's authentication token.
		return fmt.Errorf("create settings: %w", err)
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

// Path identifies the file from which these settings were loaded.
func (value Settings) Path() string { return value.sourcePath }

// Parse validates settings bytes using path as the base for relative paths.
func Parse(path string, data []byte) (Settings, error) { return parse(path, data) }

// LoadForStartup lets the server locate its database before recovering a pending
// settings transaction. Other callers should use Load.
func LoadForStartup(path string) (Settings, error) {
	data, err := ReadRaw(path)
	if err != nil {
		return Settings{}, err
	}
	return parse(path, data)
}
