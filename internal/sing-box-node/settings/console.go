// SPDX-License-Identifier: GPL-3.0-or-later

package settings

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"os"
	"path/filepath"
	"strconv"

	"github.com/rehuony/sing-box-panel/internal/atomicfile"
	"github.com/rehuony/sing-box-panel/internal/jsonstrict"
	"github.com/rehuony/sing-box-panel/internal/webauth"
)

const (
	// ConsoleFile in the data directory holds the local console's settings.
	ConsoleFile = "console.json"
	// DefaultConsoleListen is where an enabled console listens unless told
	// otherwise.
	DefaultConsoleListen = "127.0.0.1:3001"
)

// Console is the node's local console. It is off unless enabled, listens
// only on a loopback address and admits whoever knows its password.
type Console struct {
	Enabled bool
	// Listen is the console's loopback IP address and port.
	Listen string
	// PasswordHash is the Argon2id hash of the console password.
	PasswordHash string
}

type consoleFile struct {
	Enabled      bool   `json:"enabled"`
	Listen       string `json:"listen"`
	PasswordHash string `json:"password_hash"`
}

// Validate checks the console settings.
func (console Console) Validate() error {
	if err := ValidateConsoleListen(console.Listen); err != nil {
		return fmt.Errorf("listen: %w", err)
	}
	if console.PasswordHash != "" {
		if err := webauth.ValidateHash(console.PasswordHash); err != nil {
			return fmt.Errorf("password_hash: %w", err)
		}
	}
	if console.Enabled && console.PasswordHash == "" {
		return errors.New("an enabled console needs a password")
	}
	return nil
}

// ValidateConsoleListen requires a loopback IP address and a port, so the
// console is never reachable from another host.
func ValidateConsoleListen(listen string) error {
	host, port, err := net.SplitHostPort(listen)
	if err != nil {
		return err
	}
	if number, err := strconv.Atoi(port); err != nil || number < 1 || number > 65535 {
		return errors.New("the port must be between 1 and 65535")
	}
	if address, err := netip.ParseAddr(host); err != nil || !address.IsLoopback() || address.Zone() != "" {
		return errors.New("the console listens only on a loopback IP address such as 127.0.0.1 or ::1")
	}
	return nil
}

// LoadConsole reads the console settings in dataDir; without a file the
// console is disabled. The file must be private to its owner because it
// holds the password hash.
func LoadConsole(dataDir string) (Console, error) {
	path := filepath.Join(dataDir, ConsoleFile)
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return Console{Listen: DefaultConsoleListen}, nil
	}
	if err != nil {
		return Console{}, err
	}
	if !info.Mode().IsRegular() || info.Mode().Perm()&0o077 != 0 {
		return Console{}, fmt.Errorf("%s must be a regular file readable only by its owner (mode 0600)", path)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return Console{}, err
	}
	var file consoleFile
	if err := jsonstrict.Decode(raw, 16<<10, &file); err != nil {
		return Console{}, fmt.Errorf("read %s: %w", path, err)
	}
	console := Console{Enabled: file.Enabled, Listen: file.Listen, PasswordHash: file.PasswordHash}
	if err := console.Validate(); err != nil {
		return Console{}, fmt.Errorf("%s is invalid: %w", path, err)
	}
	return console, nil
}

// SaveConsole writes the console settings to dataDir.
func SaveConsole(dataDir string, console Console) error {
	if err := console.Validate(); err != nil {
		return err
	}
	if err := os.MkdirAll(dataDir, 0o700); err != nil {
		return fmt.Errorf("create node data directory: %w", err)
	}
	raw, err := json.MarshalIndent(consoleFile{Enabled: console.Enabled, Listen: console.Listen, PasswordHash: console.PasswordHash}, "", "  ")
	if err != nil {
		return err
	}
	return atomicfile.Write(filepath.Join(dataDir, ConsoleFile), append(raw, '\n'), 0o600, atomicfile.Options{})
}
