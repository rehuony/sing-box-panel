// SPDX-License-Identifier: GPL-3.0-or-later

// Package settings owns the node's settings file, which places its console
// and its data directory and holds the console password's hash, and the
// node's enrollment in that data directory: the panel it is enrolled with
// and its identity key.
package settings

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"

	"github.com/rehuony/sing-box-panel/internal/atomicfile"
	"github.com/rehuony/sing-box-panel/internal/settingsfile"
	"github.com/rehuony/sing-box-panel/internal/webauth"
)

// SystemPath is the settings file of the system service.
const SystemPath = "/etc/sing-box-node/setting.json"

// Settings is the node's settings file.
type Settings struct {
	sourcePath string
	// InitialPassword is the generated console password, set only by the
	// call that created the file. It is never saved.
	InitialPassword string `json:"-"`
	Server          Server `json:"server"`
	DataDir         string `json:"data_dir"`
	Auth            Auth   `json:"auth"`
}

// Server is where the console listens.
type Server struct {
	Host string `json:"host"`
	Port int    `json:"port"`
	// ExternalOrigin, when set, is the only origin the console accepts
	// logins and changes from, and HTTPS makes its cookie Secure.
	ExternalOrigin string `json:"external_origin"`
}

// Auth holds the console password's Argon2id hash.
type Auth struct {
	PasswordHash string `json:"password_hash"`
}

// Defaults returns the settings of a new file for the current effective user.
func Defaults() Settings {
	return Settings{Server: Server{Host: "127.0.0.1", Port: 8000}, DataDir: DefaultDataDir()}
}

// DefaultPath returns the root or XDG settings path.
func DefaultPath() string {
	if os.Geteuid() == 0 {
		return SystemPath
	}
	configHome := strings.TrimSpace(os.Getenv("XDG_CONFIG_HOME"))
	if configHome == "" {
		home, err := os.UserHomeDir()
		if err != nil || home == "" {
			return "setting.json"
		}
		configHome = filepath.Join(home, ".config")
	}
	return filepath.Join(configHome, "sing-box-node", "setting.json")
}

// DefaultDataDir returns the root or XDG data directory.
func DefaultDataDir() string {
	if os.Geteuid() == 0 {
		return "/var/lib/sing-box-node"
	}
	dataHome := strings.TrimSpace(os.Getenv("XDG_DATA_HOME"))
	if dataHome == "" {
		home, err := os.UserHomeDir()
		if err != nil || home == "" {
			return "data"
		}
		dataHome = filepath.Join(home, ".local", "share")
	}
	return filepath.Join(dataHome, "sing-box-node")
}

// Path identifies the file these settings were loaded from.
func (value Settings) Path() string { return value.sourcePath }

// Listen is the console's listen address.
func (value Settings) Listen() string {
	return net.JoinHostPort(value.Server.Host, strconv.Itoa(value.Server.Port))
}

// ConsoleURL is where an operator opens the console: the external origin,
// else the listen address, with an unspecified host shown as loopback.
func (value Settings) ConsoleURL() string {
	if value.Server.ExternalOrigin != "" {
		return value.Server.ExternalOrigin + "/"
	}
	host := value.Server.Host
	if ip := net.ParseIP(host); ip != nil && ip.IsUnspecified() {
		host = "127.0.0.1"
		if ip.To4() == nil {
			host = "::1"
		}
	}
	return "http://" + net.JoinHostPort(host, strconv.Itoa(value.Server.Port)) + "/"
}

// Validate checks the complete settings.
func (value Settings) Validate() error {
	if net.ParseIP(value.Server.Host) == nil && value.Server.Host != "localhost" {
		return errors.New("server.host must be an IP address or localhost")
	}
	if value.Server.Port < 1 || value.Server.Port > 65535 {
		return errors.New("server.port must be between 1 and 65535")
	}
	if value.Server.ExternalOrigin != "" {
		origin, err := webauth.NormalizeOrigin(value.Server.ExternalOrigin)
		if err != nil {
			return fmt.Errorf("server.external_origin: %w", err)
		}
		if origin != value.Server.ExternalOrigin {
			return errors.New("server.external_origin must be a normalized HTTP or HTTPS origin")
		}
	}
	if value.DataDir == "" || !filepath.IsAbs(value.DataDir) {
		return errors.New("data_dir must resolve to an absolute path")
	}
	if err := webauth.ValidateHash(value.Auth.PasswordHash); err != nil {
		return fmt.Errorf("auth.password_hash: %w; use config set auth.password --stdin", err)
	}
	return nil
}

// Parse validates settings bytes read from path, resolving a relative
// data_dir against the file's directory.
func Parse(path string, data []byte) (Settings, error) {
	absolutePath, err := filepath.Abs(path)
	if err != nil {
		return Settings{}, err
	}
	value := Settings{sourcePath: absolutePath}
	if err := settingsfile.Decode(path, data, &value); err != nil {
		return Settings{}, err
	}
	if value.DataDir, err = settingsfile.ResolveDataDir(path, value.DataDir); err != nil {
		return Settings{}, err
	}
	if value.Server.ExternalOrigin != "" {
		origin, err := webauth.NormalizeOrigin(value.Server.ExternalOrigin)
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

// Load reads and validates the settings file.
func Load(path string) (Settings, error) {
	data, err := settingsfile.Read(path)
	if err != nil {
		return Settings{}, err
	}
	return Parse(path, data)
}

// LoadDataDir reads only data_dir, so an unrelated invalid field cannot keep
// a command from finding the node's data.
func LoadDataDir(path string) (string, error) {
	data, err := settingsfile.Read(path)
	if err != nil {
		return "", err
	}
	return settingsfile.DataDirOf(path, data)
}

// LoadOrInitialize loads the settings, first creating defaults with a
// generated console password when the file is absent; created reports that.
func LoadOrInitialize(ctx context.Context, path string) (value Settings, created bool, err error) {
	value, created, err = EnsureFile(ctx, path, "")
	if err != nil || created {
		return value, created, err
	}
	value, err = Load(path)
	return value, false, err
}

// EnsureFile creates default settings with dataDir, or the default data
// directory when it is empty, only when the file is absent.
func EnsureFile(ctx context.Context, path, dataDir string) (Settings, bool, error) {
	if info, err := os.Lstat(path); err == nil {
		if !info.Mode().IsRegular() {
			return Settings{}, false, fmt.Errorf("settings %q must be a regular file", path)
		}
		return Settings{}, false, nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return Settings{}, false, err
	}
	value, err := initialize(ctx, path, false, dataDir)
	if errors.Is(err, os.ErrExist) && value.InitialPassword == "" {
		return Settings{}, false, nil
	}
	return value, value.InitialPassword != "", err
}

// Initialize writes default settings with a generated console password.
// An existing file is replaced only with overwrite.
func Initialize(ctx context.Context, path string, overwrite bool) (Settings, error) {
	return initialize(ctx, path, overwrite, "")
}

func initialize(ctx context.Context, path string, overwrite bool, dataDir string) (Settings, error) {
	if err := ctx.Err(); err != nil {
		return Settings{}, err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return Settings{}, fmt.Errorf("create settings directory: %w", err)
	}
	lock, err := settingsfile.Lock(ctx, path)
	if err != nil {
		return Settings{}, err
	}
	defer lock.Close()
	if info, err := os.Lstat(path); err == nil {
		if !overwrite {
			return Settings{}, fmt.Errorf("settings file %q already exists: %w", path, os.ErrExist)
		}
		if !info.Mode().IsRegular() {
			return Settings{}, fmt.Errorf("settings %q must be a regular file", path)
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return Settings{}, fmt.Errorf("inspect settings %q: %w", path, err)
	}
	value := Defaults()
	if dataDir != "" {
		value.DataDir = dataDir
	}
	if value.sourcePath, err = filepath.Abs(path); err != nil {
		return Settings{}, err
	}
	password, err := webauth.GeneratePassword()
	if err != nil {
		return Settings{}, err
	}
	if value.Auth.PasswordHash, err = webauth.HashPassword(ctx, password); err != nil {
		return Settings{}, err
	}
	if value.DataDir, err = settingsfile.ResolveDataDir(path, value.DataDir); err != nil {
		return Settings{}, err
	}
	if err := value.Validate(); err != nil {
		return Settings{}, err
	}
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return Settings{}, fmt.Errorf("encode settings: %w", err)
	}
	data = append(data, '\n')
	if err := ctx.Err(); err != nil {
		return Settings{}, err
	}
	if err := write(path, data, overwrite); err != nil {
		// A directory sync can fail after publication; keep the password
		// when the file holds it.
		if saved, readErr := settingsfile.Read(path); readErr == nil && bytes.Equal(saved, data) {
			value.InitialPassword = password
		}
		return value, err
	}
	value.InitialPassword = password
	return value, nil
}

// GetField returns a stored field or section named by dotted JSON keys,
// without validating the file or applying defaults.
func GetField(ctx context.Context, path, name string) (json.RawMessage, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if name == "auth.password" {
		return nil, errors.New("auth.password is write-only; plaintext passwords are not stored")
	}
	field, err := settingsfile.FieldOf(reflect.TypeFor[Settings](), name)
	if err != nil {
		return nil, err
	}
	data, err := settingsfile.Read(path)
	if err != nil {
		return nil, err
	}
	return field.Get(data)
}

// SetField updates one field or section named by dotted JSON keys. Strings
// are literal, other values JSON. The write-only auth.password takes a
// plaintext password and saves only its hash.
func SetField(ctx context.Context, path, name, raw string) error {
	if name == "auth.password" {
		hash, err := webauth.HashPassword(ctx, raw)
		if err != nil {
			return err
		}
		name, raw = "auth.password_hash", hash
	}
	field, err := settingsfile.FieldOf(reflect.TypeFor[Settings](), name)
	if err != nil {
		return err
	}
	value, err := field.Encode(raw)
	if err != nil {
		return err
	}
	lock, err := settingsfile.Lock(ctx, path)
	if err != nil {
		return err
	}
	defer lock.Close()
	before, err := settingsfile.Read(path)
	if err != nil {
		return err
	}
	after, err := field.Set(before, value)
	if err != nil {
		return err
	}
	if _, err := Parse(path, after); err != nil {
		// Validation errors can quote the supplied value.
		return errors.New("update would invalidate the node settings; check the field type and allowed values")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	return write(path, after, true)
}

// Replace validates and atomically saves a complete settings document.
func Replace(ctx context.Context, path string, data []byte) error {
	if _, err := Parse(path, data); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	lock, err := settingsfile.Lock(ctx, path)
	if err != nil {
		return err
	}
	defer lock.Close()
	if info, err := os.Lstat(path); err == nil && !info.Mode().IsRegular() {
		return fmt.Errorf("settings %q must be a regular file", path)
	}
	return write(path, data, true)
}

// write publishes settings bytes privately. A root writer keeps the owner of
// the file it replaces, so the service account can still read it.
func write(path string, data []byte, overwrite bool) error {
	return atomicfile.Write(path, data, 0o600, atomicfile.Options{
		Prepare:   func(file *os.File) error { return atomicfile.PreserveOwner(file, path) },
		Exclusive: !overwrite,
	})
}
