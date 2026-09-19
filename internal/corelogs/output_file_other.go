//go:build !unix

// SPDX-License-Identifier: GPL-3.0-or-later

package corelogs

import "os"

func openOutputFile(string) (*os.File, os.FileInfo, error) { return nil, nil, ErrInvalidFile }
