# systemd templates

These files are packaging inputs. CI validates source, build behavior, and generated units with `systemd-analyze verify`;
it never copies units, creates users, enables services, or performs installation.

The binary also exposes the same templates through `sing-box-panel systemd`.
Those commands are Linux-only and never invoke a shell. Every systemd scope is
reported in command output; `--scope=auto` resolves to `system` for root and to
`user` otherwise.

Units invoke `server start` in the foreground and set
`SING_BOX_PANEL_SUPERVISOR=systemd` to identify the panel's service owner.
Manual terminal runs do not set that marker. They remain stoppable through
`server stop` even when the terminal inherited a generic systemd invocation
environment; the service itself is stopped through `systemd stop`.

```sh
# Dedicated system service, initially using the default data directory.
sudo /usr/local/bin/sing-box-panel systemd install --scope=system --now

# Ordinary per-user service. The current executable, settings path, and data
# directory are rendered according to each directive's parsing rules.
sing-box-panel systemd install --scope=user --now
```

The built-in systemd installer writes to `/etc/systemd/system`,
`/etc/sysusers.d`, and `/etc/tmpfiles.d`; distro packages should continue to
use the `/usr/lib` destinations below. System scope requires root and the
executable `/usr/local/bin/sing-box-panel` and settings
`/etc/sing-box-panel/setting.json`. Data defaults to `/var/lib/sing-box-panel`;
a configured dedicated directory is rendered into the unit and tmpfiles rule. It creates
the dedicated account/directories, grants that account access only to the
settings and data paths, reloads systemd, and enables the unit. User scope
writes only the current user's XDG systemd unit.

Before writing files, installation checks required commands, manager access,
permissions, paths, pending migrations and conflicting service files. It creates
missing settings (0600, random token) and directories (0700), without overwriting
existing configuration or initializing the database. The server creates storage
when it starts. Existing differing unit files still require `--force`.
Configuration sidecars must be regular files when present; conflicts are rejected
before resource creation or account preparation. Existing data directories need
write access to the directory itself, not to an otherwise shared parent.
`systemctl`, `systemd-sysusers`, `systemd-tmpfiles`, and `chown` are checked only
when required; the CLI never runs a package manager. `systemd-analyze` is a test
and CI dependency, not a runtime requirement.

`WorkingDirectory` is a scalar path: quotes and backslashes are literal, while
percent specifiers must be escaped. Execution arguments, path lists and tmpfiles
fields use their respective quoted forms. Unrepresentable paths fail preflight.

Installation without `--now` reads and validates only the configured data path;
`--now` also requires valid runtime settings before any installation starts.
Status uses only location fields for its optional settings report. Other
service operations do not load the CLI settings file. Start/restart inspect the
installed unit's settings for data relocation and missing resources. Preparation
requires an unambiguous generated unit and never uses the CLI-selected config
in place of the installed one. Start/restart never installs a missing unit. A starting service
validates its own runtime configuration.
Start/restart read the exact managed destinations rather than the full resource
inventory; inaccessible unrelated systemd directories do not block control.
`system df` still reports inspection failures and the partial inventory.

The ownership marker embedded in installed files remains stable across CLI
renames so existing managed units stay recognizable.

`systemd uninstall` verifies the loaded state, stops active services, then disables
the exact scope and removes only files
at the built-in installer's audited destinations. Settings, data, and the
system account are retained. It refuses unmanaged or changed files unless the
operator supplies `--force`; even with `--force`, it never deletes settings or
data. Missing or broken inactive units can be removed; manager, permission and
stop failures remain errors, with already removed paths preserved in results.
Even when all installed files are missing, uninstall checks the manager: a cached
running unit must have a matching fragment path and stop successfully before
uninstall can succeed. An absent inactive unit requires no changes.
`systemd status` reports systemd's fragment path, state, and
`NeedDaemonReload`, plus two on-disk facts labeled as such: the `--config`
path in the single effective `[Service] ExecStart` line of the unit file on
disk, and the data directory, database, and configuration storage declared by
that settings file as it exists now. The CLI's own `--config` path is listed
separately. Drop-in overrides, unreadable units, several effective commands, or
unresolved specifiers leave the unit-file settings path unknown rather than
guessed, a unit edited after loading is flagged stale, and the running
process's settings are never inspected or claimed. `systemd logs --lines N
[--since VALUE]` performs one bounded journal query.

## System service

The system template runs as the dedicated `sing-box-panel` user and grants no
Linux capabilities by default. A packager should install the following files:

| Source | Destination |
| --- | --- |
| `system/sing-box-panel.service` | `/usr/lib/systemd/system/sing-box-panel.service` |
| `sysusers.d/sing-box-panel.conf` | `/usr/lib/sysusers.d/sing-box-panel.conf` |
| `tmpfiles.d/sing-box-panel.conf` | `/usr/lib/tmpfiles.d/sing-box-panel.conf` |

After creating the account and directories with the host's normal packaging
tools, initialize `/etc/sing-box-panel/setting.json`. The runtime user must be
able to atomically replace that file and create private lock/recovery sidecars
in its directory, while no other user should be able to read its token:

```sh
chown sing-box-panel:sing-box-panel /etc/sing-box-panel /etc/sing-box-panel/setting.json
chmod 0700 /etc/sing-box-panel
chmod 0600 /etc/sing-box-panel/setting.json
```

The default unit is suitable for unprivileged proxy ports. TUN, transparent
proxy, raw sockets, and ports below 1024 require an explicit local review. The
`examples/tun-override.conf` file shows the smallest expected capability and
device override; it is deliberately outside the auto-loaded unit directory.

Host metrics keep `ProtectProc=invisible` and `ProcSubset=pid`. The system unit
bind-mounts only `/proc/stat`, `/proc/meminfo`, and `/proc/loadavg` read-only into
`/run/sing-box-panel/host-proc`, and sets `SING_BOX_PANEL_HOST_PROC` so the sampler
reads these live counters. Direct and user-service runs use `/proc` by default.
Missing counters remain unavailable; disk sampling is independent. Older system
units hide these files without providing the mounts, so upgrading the binary
alone does not restore CPU, memory, or load readings. Refresh the installed unit
using the upgraded binary, then restart the service. For an uncustomized generated
system unit:

```sh
sudo /usr/local/bin/sing-box-panel systemd install --scope=system --force
sudo /usr/local/bin/sing-box-panel systemd restart --scope=system
```

Review customized units before replacement. Keep all three bind mounts and the
environment setting together; do not expose the whole host `/proc` tree.

## User service

The static packaging user unit expects:

- binary: `~/.local/bin/sing-box-panel`
- settings: `~/.config/sing-box-panel/setting.json`
- data: `~/.local/share/sing-box-panel`

Create and initialize those paths before enabling the unit. The user template
cannot provide TUN, transparent-proxy, raw-socket, or privileged-port access.
It is intended for ordinary user-owned proxy listeners.

Both services use restart-on-failure and stop the complete process group. Their
sandbox permits only Unix, IPv4, IPv6, and netlink sockets and keeps the data
and settings directories writable. The system template explicitly permits
`/etc/sing-box-panel`; rendered user units permit the selected settings and data
directories. After upgrading an older installation, rerun `systemd install` to
refresh the unit and ownership before saving preferences through the Web UI.
Use `--force` when replacing an older generated template after reviewing changes.

After editing `data_dir`, an explicit `systemd restart` coordinates a stopped
relocation and refreshes generated working-directory, sandbox and tmpfiles paths.
System services retain `ProtectHome=true`, so data destinations under `/home`,
`/root` or `/run/user` are rejected before stopping the service. Explicit data
and configuration directories under private temporary roots receive narrow bind
mounts; other sandbox restrictions stay enabled. Custom data roots also clear
the default `StateDirectory` declaration so it does not recreate the old root.
Automatic rewriting requires exact generated files and no drop-in overrides.
For customized units, stop the service and explicitly run `systemd install`
(with `--force` only when replacing those customizations is intended), then start
it. Installation also performs a pending move, refusing any active owner.
