// SPDX-License-Identifier: GPL-3.0-or-later

//go:build !unix

package artifactstore

import "os"

// This store executes Linux binaries and cannot prove equivalent ownership
// semantics on non-Unix hosts. Fail closed instead of guessing from mode bits.
func fileOwner(os.FileInfo) (uint64, bool) { return 0, false }

func effectiveUserID() (uint64, bool) { return 0, false }
