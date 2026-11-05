// SPDX-License-Identifier: GPL-3.0-or-later

//go:build !unix

package atomicfile

import "os"

// PreserveOwner keeps the writer's owner where file ownership is not Unix.
func PreserveOwner(*os.File, string) error { return nil }
