// SPDX-License-Identifier: GPL-3.0-or-later

// Package releases owns the panel's releases: the node release the panel
// distributes to its nodes, which is the release of the panel's own version,
// and updates of the panel itself.
package releases

import (
	"context"
	"crypto/ed25519"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/rehuony/sing-box-panel/internal/buildinfo"
	releasecontract "github.com/rehuony/sing-box-panel/internal/release"
	"github.com/rehuony/sing-box-panel/internal/selfupdate"
	"github.com/rehuony/sing-box-panel/internal/sing-box-panel/settings"
	"github.com/rehuony/sing-box-panel/internal/sing-box-panel/telemetry"
)

// CacheDir in the data directory holds the verified node release of the
// panel's version.
const CacheDir = "release-cache"

var (
	// ErrUnavailable answers every node update of a panel that is not a
	// release build: it has no release to distribute.
	ErrUnavailable = errors.New("node updates need a release build of the panel; this panel is a development build")
	// ErrPlatformUnsupported reports a node platform the release has no
	// binary for.
	ErrPlatformUnsupported = errors.New("the panel's release has no node binary for this platform")
)

// Options adjusts where releases come from; the zero value uses the
// repository's GitHub releases and the release key embedded at build time.
type Options struct {
	PublicKey   ed25519.PublicKey
	ReleasesURL string
	HTTPClient  *http.Client
	// Executable is the panel's own executable; nil is the running one.
	Executable func() (string, error)
	// Restart queues a restart of the panel's systemd unit in the scope
	// that runs it; without it an installed update waits for a manual
	// restart.
	Restart func(context.Context) error
	Now     func() time.Time
}

// Service distributes the node release and updates the panel.
type Service struct {
	build    buildinfo.Info
	dataDir  string
	settings *settings.Service
	records  *telemetry.Service
	options  Options

	cacheMu sync.Mutex
	// cached is the version whose cache directory was verified.
	cached string

	mu     sync.Mutex
	update updateState
	// lifetime bounds background updates; Run sets it.
	lifetime context.Context
}

// New serves releases from the data directory dataDir of a panel of build.
func New(build buildinfo.Info, dataDir string, panelSettings *settings.Service, records *telemetry.Service, options Options) *Service {
	if options.Now == nil {
		options.Now = time.Now
	}
	return &Service{
		build: build, dataDir: dataDir, settings: panelSettings, records: records, options: options,
		lifetime: context.Background(), update: updateState{state: StateIdle},
	}
}

// updater returns an updater of binary with the current GitHub token.
func (service *Service) updater(binary selfupdate.Binary) *selfupdate.Updater {
	token := ""
	if service.settings != nil {
		token = service.settings.Value().GitHub.Token
	}
	return selfupdate.New(selfupdate.Options{
		Binary: binary, HTTPClient: service.options.HTTPClient, ReleasesURL: service.options.ReleasesURL,
		Token: token, Executable: service.options.Executable, PublicKey: service.options.PublicKey,
	})
}

// NodeVersion is the version of the node release the panel distributes:
// its own.
func (service *Service) NodeVersion() (string, error) {
	if releasecontract.ValidateVersion(service.build.Version) != nil {
		return "", ErrUnavailable
	}
	return service.build.Version, nil
}

// NodeRelease is the node binary of the panel's release for one platform,
// with the signed checksum manifest that proves it.
type NodeRelease struct {
	Version   string
	Path      string
	Size      int64
	SHA256    string
	Checksums []byte
	Signature []byte
}

func nodeAssets() []string {
	return []string{selfupdate.Node.AssetName("linux", "amd64"), selfupdate.Node.AssetName("linux", "arm64")}
}

// NodeRelease returns the node release for a platform from the cache,
// downloading and verifying the release of the panel's version first when
// the cache does not hold it, and removing other versions' caches.
func (service *Service) NodeRelease(ctx context.Context, goos, goarch string) (NodeRelease, error) {
	version, err := service.NodeVersion()
	if err != nil {
		return NodeRelease{}, err
	}
	if !selfupdate.SupportedPlatform(goos, goarch) {
		return NodeRelease{}, fmt.Errorf("%w: %s/%s", ErrPlatformUnsupported, goos, goarch)
	}
	service.cacheMu.Lock()
	defer service.cacheMu.Unlock()
	root := filepath.Join(service.dataDir, CacheDir)
	dir := filepath.Join(root, version)
	updater := service.updater(selfupdate.Node)
	if service.cached != version {
		if _, _, _, err := updater.VerifyFiles(dir, version, nodeAssets()); err != nil {
			if err := service.download(ctx, updater, root, version); err != nil {
				return NodeRelease{}, err
			}
		}
		service.cached = version
		service.removeOtherVersions(root, version)
	}
	name := selfupdate.Node.AssetName(goos, goarch)
	checksums, signature, digests, err := updater.VerifyFiles(dir, version, []string{name})
	if err != nil {
		service.cached = ""
		return NodeRelease{}, err
	}
	info, err := os.Lstat(filepath.Join(dir, name))
	if err != nil {
		return NodeRelease{}, err
	}
	digest := digests[name]
	return NodeRelease{
		Version: version, Path: filepath.Join(dir, name), Size: info.Size(), SHA256: hex.EncodeToString(digest[:]),
		Checksums: checksums, Signature: signature,
	}, nil
}

// download fetches the node binaries of version into a new directory,
// verifies them and moves them into the cache.
func (service *Service) download(ctx context.Context, updater *selfupdate.Updater, root, version string) error {
	if err := os.MkdirAll(root, 0o700); err != nil {
		return err
	}
	incoming, err := os.MkdirTemp(root, ".incoming-*")
	if err != nil {
		return err
	}
	defer func() { _ = os.RemoveAll(incoming) }()
	if err := updater.Fetch(ctx, version, nodeAssets(), incoming); err != nil {
		return err
	}
	if _, _, _, err := updater.VerifyFiles(incoming, version, nodeAssets()); err != nil {
		return err
	}
	dir := filepath.Join(root, version)
	if err := os.RemoveAll(dir); err != nil {
		return err
	}
	return os.Rename(incoming, dir)
}

func (service *Service) removeOtherVersions(root, keep string) {
	entries, err := os.ReadDir(root)
	if err != nil {
		return
	}
	for _, entry := range entries {
		if entry.Name() != keep && (strings.HasPrefix(entry.Name(), ".incoming-") || releasecontract.ValidateVersion(entry.Name()) == nil) {
			_ = os.RemoveAll(filepath.Join(root, entry.Name()))
		}
	}
}
