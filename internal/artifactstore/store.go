// SPDX-License-Identifier: GPL-3.0-or-later

package artifactstore

import (
	"errors"
	"os"
	"path/filepath"
)

type Store struct {
	root       string
	downloader Downloader
	inspector  VersionInspector
	limits     Limits
}

func New(options Options) (*Store, error) {
	if options.Root == "" || !filepath.IsAbs(options.Root) || filepath.Clean(options.Root) != options.Root || options.Root == string(filepath.Separator) {
		return nil, fail(StepPrepare, "invalid_root", nil)
	}
	if options.Limits == (Limits{}) {
		options.Limits = DefaultLimits()
	}
	if err := options.Limits.validate(); err != nil {
		return nil, fail(StepPrepare, "invalid_limits", err)
	}
	lexicalParent := filepath.Dir(options.Root)
	if err := verifyTrustedLexicalPath(lexicalParent); err != nil {
		return nil, fail(StepPrepare, "unsafe_root_ancestors", err)
	}
	resolvedParent, err := filepath.EvalSymlinks(lexicalParent)
	if err != nil {
		return nil, fail(StepPrepare, "root_parent_resolve", err)
	}
	options.Root = filepath.Join(resolvedParent, filepath.Base(options.Root))
	if err := os.Mkdir(options.Root, 0o700); err != nil && !errors.Is(err, os.ErrExist) {
		return nil, fail(StepPrepare, "root_create", err)
	}
	if err := verifyTrustedAncestors(options.Root); err != nil {
		return nil, fail(StepPrepare, "unsafe_root_ancestors", err)
	}
	if err := verifyStoreDirectory(options.Root, 0o700); err != nil {
		return nil, fail(StepPrepare, "root_mode", err)
	}
	if options.Downloader == nil {
		options.Downloader, err = NewSafeDownloader(SafeDownloaderOptions{Timeout: options.Limits.DownloadTimeout})
		if err != nil {
			return nil, err
		}
	}
	if options.Inspector == nil {
		options.Inspector = ExecVersionInspector{Timeout: options.Limits.VersionTimeout}
	}
	return &Store{root: options.Root, downloader: options.Downloader, inspector: options.Inspector, limits: options.Limits}, nil
}
