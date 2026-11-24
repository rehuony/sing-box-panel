// SPDX-License-Identifier: GPL-3.0-or-later

//go:build linux || darwin

package systemd

import (
	"fmt"
	"os"
	"os/user"
	"strconv"
	"syscall"
)

// CheckRootStart refuses to start a service as root against files another
// account owns, such as those of the system service, which runs as its own
// account: a root process would leave files that account cannot use, and
// the node would run its cores from a directory that account can change.
// name is the service's executable; paths are the files to check, in the
// order they are reported. Missing paths are skipped.
func CheckRootStart(name string, paths ...string) error {
	if os.Geteuid() != 0 {
		return nil
	}
	for _, path := range paths {
		info, err := os.Lstat(path)
		if err != nil {
			continue
		}
		stat, ok := info.Sys().(*syscall.Stat_t)
		if !ok || stat.Uid == 0 {
			continue
		}
		account := "uid " + strconv.FormatUint(uint64(stat.Uid), 10)
		if found, err := user.LookupId(strconv.FormatUint(uint64(stat.Uid), 10)); err == nil {
			account = found.Username
		}
		return fmt.Errorf("%s belongs to the %s account, so %s must run as that account, not as root\n"+
			"Next: sudo %s systemd start\n"+
			"Inspect: sudo %s systemd status",
			path, account, name, name, name)
	}
	return nil
}
