// SPDX-License-Identifier: GPL-3.0-or-later

package artifacts

import (
	"bytes"
	"context"
	"crypto/sha256"
	"io"

	"github.com/rehuony/sing-box-panel/internal/coreartifact"
)

// Downloader produces archive bytes for the tests' official installs.
type Downloader interface {
	Download(ctx context.Context, rawURL string, destination io.Writer, maximumBytes int64) (int64, error)
}

// OfficialArchive describes an official release asset in the tests.
type OfficialArchive struct {
	RepositoryID int64
	ReleaseID    int64
	AssetID      int64
	DownloadURL  string
	Size         int64
	Version      coreartifact.ExactVersion
	Architecture coreartifact.Architecture
	Variant      coreartifact.Variant
}

// testStore installs official archives through Install, the way the node
// installs what the panel streams, with the digest the panel would supply.
type testStore struct {
	*Store
	downloader Downloader
}

func (store testStore) InstallOfficial(ctx context.Context, asset OfficialArchive) (Result, error) {
	source, err := coreartifact.NewOfficialSource(asset.RepositoryID, asset.ReleaseID, asset.AssetID)
	if err != nil {
		return Result{}, fail(StepPrepare, "source_identity", err)
	}
	// Download once, as the panel does into its cache, then replay the bytes
	// and the downloader's reported count and error to the node install.
	var archive bytes.Buffer
	reported, downloadErr := store.downloader.Download(ctx, asset.DownloadURL, &archive, store.limits.MaximumArchiveBytes)
	digest := coreartifact.NewSHA256(sha256.Sum256(archive.Bytes()))
	write := func(_ context.Context, destination io.Writer) (int64, error) {
		if _, err := destination.Write(archive.Bytes()); err != nil {
			return 0, err
		}
		return reported, downloadErr
	}
	return store.Install(ctx, Candidate{
		Source: source, SHA256: digest, Size: asset.Size, Version: asset.Version,
		Architecture: asset.Architecture, Variant: asset.Variant, Write: write,
	})
}
