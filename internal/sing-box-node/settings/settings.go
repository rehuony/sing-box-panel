// SPDX-License-Identifier: GPL-3.0-or-later

// Package settings owns the node's local settings: its data directory and
// the panel it is enrolled with, including its identity key.
package settings

import (
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/rehuony/sing-box-panel/internal/atomicfile"
	"github.com/rehuony/sing-box-panel/internal/jsonstrict"
	"github.com/rehuony/sing-box-panel/internal/nodeapi"
)

// EnrollmentFile in the data directory holds the node's enrollment.
const EnrollmentFile = "enrollment.json"

var ErrNotEnrolled = errors.New("this node is not enrolled; run sing-box-node enroll --code <code>, or enable the console with sing-box-node console enable and enroll from it")

// Enrollment is the panel a node is enrolled with and the node's identity
// key. The key never leaves the node.
type Enrollment struct {
	// Endpoint is the panel's address as host:port.
	Endpoint string
	// Path is the URL path prefix of the panel's node service.
	Path string
	// Pin is the panel certificate's public key pin; empty uses the system's
	// trusted certificate authorities.
	Pin     string
	PanelID string
	NodeID  string
	Key     ed25519.PrivateKey
}

type enrollmentFile struct {
	Endpoint string `json:"endpoint"`
	Path     string `json:"path"`
	Pin      string `json:"pin"`
	PanelID  string `json:"panel_id"`
	NodeID   string `json:"node_id"`
	// Key is the Ed25519 private key seed in unpadded base64url.
	Key string `json:"key"`
}

// DefaultDataDir returns the root or XDG data directory of the node.
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

// Load reads the enrollment in dataDir. The file must be private to its
// owner because it holds the identity key.
func Load(dataDir string) (Enrollment, error) {
	path := filepath.Join(dataDir, EnrollmentFile)
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return Enrollment{}, ErrNotEnrolled
	}
	if err != nil {
		return Enrollment{}, err
	}
	if !info.Mode().IsRegular() || info.Mode().Perm()&0o077 != 0 {
		return Enrollment{}, fmt.Errorf("%s must be a regular file readable only by its owner (mode 0600)", path)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return Enrollment{}, err
	}
	var file enrollmentFile
	if err := jsonstrict.Decode(raw, 64<<10, &file); err != nil {
		return Enrollment{}, fmt.Errorf("read %s: %w", path, err)
	}
	seed, err := base64.RawURLEncoding.DecodeString(file.Key)
	if err != nil || len(seed) != ed25519.SeedSize || nodeapi.ValidEndpoint(file.Endpoint) != nil ||
		(file.Path != "" && nodeapi.ValidPathPrefix(file.Path) != nil) || file.PanelID == "" || file.NodeID == "" {
		return Enrollment{}, fmt.Errorf("%s is invalid; enroll the node again", path)
	}
	return Enrollment{
		Endpoint: file.Endpoint, Path: file.Path, Pin: file.Pin, PanelID: file.PanelID, NodeID: file.NodeID,
		Key: ed25519.NewKeyFromSeed(seed),
	}, nil
}

// Save writes enrollment to dataDir, replacing any previous enrollment.
func Save(dataDir string, enrollment Enrollment) error {
	if err := os.MkdirAll(dataDir, 0o700); err != nil {
		return fmt.Errorf("create node data directory: %w", err)
	}
	raw, err := json.MarshalIndent(enrollmentFile{
		Endpoint: enrollment.Endpoint, Path: enrollment.Path, Pin: enrollment.Pin, PanelID: enrollment.PanelID, NodeID: enrollment.NodeID,
		Key: base64.RawURLEncoding.EncodeToString(enrollment.Key.Seed()),
	}, "", "  ")
	if err != nil {
		return err
	}
	return atomicfile.Write(filepath.Join(dataDir, EnrollmentFile), append(raw, '\n'), 0o600, atomicfile.Options{})
}

// Remove deletes the enrollment in dataDir, if any.
func Remove(dataDir string) error {
	err := os.Remove(filepath.Join(dataDir, EnrollmentFile))
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("remove the enrollment: %w", err)
	}
	return atomicfile.SyncDir(dataDir)
}
