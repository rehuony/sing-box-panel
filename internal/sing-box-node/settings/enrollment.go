// SPDX-License-Identifier: GPL-3.0-or-later

package settings

import (
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/rehuony/sing-box-panel/internal/atomicfile"
	"github.com/rehuony/sing-box-panel/internal/jsonstrict"
	"github.com/rehuony/sing-box-panel/internal/nodeapi"
)

// EnrollmentFile in the data directory holds the node's enrollment.
const EnrollmentFile = "enrollment.json"

// ErrNotEnrolled reports a node without a panel; it waits for an enrollment
// from its console or from sing-box-node enroll.
var ErrNotEnrolled = errors.New("this node is not enrolled; enroll it from its console or with sing-box-node enroll --code <code>")

// Enrollment is the panel a node is enrolled with and the node's identity
// key. The key never leaves the node.
type Enrollment struct {
	// URL is the panel's base URL, which nodeapi.ValidPanelURL accepts.
	URL     string
	PanelID string
	NodeID  string
	Key     ed25519.PrivateKey
}

type enrollmentFile struct {
	URL     string `json:"url"`
	PanelID string `json:"panel_id"`
	NodeID  string `json:"node_id"`
	// Key is the Ed25519 private key seed in unpadded base64url.
	Key string `json:"key"`
}

// LoadEnrollment reads the enrollment in dataDir. The file must be private to its
// owner because it holds the identity key.
func LoadEnrollment(dataDir string) (Enrollment, error) {
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
		return Enrollment{}, fmt.Errorf("read %s: %w; enroll the node again", path, err)
	}
	seed, err := base64.RawURLEncoding.DecodeString(file.Key)
	if err != nil || len(seed) != ed25519.SeedSize || nodeapi.ValidPanelURL(file.URL) != nil || file.PanelID == "" || file.NodeID == "" {
		return Enrollment{}, fmt.Errorf("%s is invalid; enroll the node again", path)
	}
	return Enrollment{URL: file.URL, PanelID: file.PanelID, NodeID: file.NodeID, Key: ed25519.NewKeyFromSeed(seed)}, nil
}

// SaveEnrollment writes enrollment to dataDir, replacing any previous enrollment.
func SaveEnrollment(dataDir string, enrollment Enrollment) error {
	if err := os.MkdirAll(dataDir, 0o700); err != nil {
		return fmt.Errorf("create node data directory: %w", err)
	}
	raw, err := json.MarshalIndent(enrollmentFile{
		URL: enrollment.URL, PanelID: enrollment.PanelID, NodeID: enrollment.NodeID,
		Key: base64.RawURLEncoding.EncodeToString(enrollment.Key.Seed()),
	}, "", "  ")
	if err != nil {
		return err
	}
	return writePrivate(filepath.Join(dataDir, EnrollmentFile), append(raw, '\n'))
}

// RemoveEnrollment deletes the enrollment in dataDir, if any.
func RemoveEnrollment(dataDir string) error {
	err := os.Remove(filepath.Join(dataDir, EnrollmentFile))
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("remove the enrollment: %w", err)
	}
	return atomicfile.SyncDir(dataDir)
}

// writePrivate publishes a file readable only by its owner. A root command
// keeps the owner of the data directory, so a service running as its own
// account can still read what the command wrote.
func writePrivate(path string, data []byte) error {
	return atomicfile.Write(path, data, 0o600, atomicfile.Options{
		Prepare: func(file *os.File) error { return atomicfile.PreserveOwner(file, path) },
	})
}
