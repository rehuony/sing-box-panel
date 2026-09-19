# systemd templates

These files are packaging inputs. CI validates source and build behavior only;
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
# Dedicated system service. These three paths are deliberately fixed.
sudo /usr/local/bin/sing-box-panel init
sudo /usr/local/bin/sing-box-panel systemd install --scope=system --now

# Ordinary per-user service. The current executable, settings path, and data
# directory are rendered as separately quoted unit arguments.
sing-box-panel init
sing-box-panel systemd install --scope=user --now
```

The built-in systemd installer writes to `/etc/systemd/system`,
`/etc/sysusers.d`, and `/etc/tmpfiles.d`; distro packages should continue to
use the `/usr/lib` destinations below. System scope requires root and the
release layout `/usr/local/bin/sing-box-panel`,
`/etc/sing-box-panel/setting.json`, and `/var/lib/sing-box-panel`. It creates
the dedicated account/directories, grants that account access only to the
settings and data paths, reloads systemd, and enables the unit. User scope
writes only the current user's XDG systemd unit.

Installation without `--now` reads and validates only the configured data path;
`--now` also requires valid runtime settings before any installation starts.
Status uses only location fields for its optional settings report. Other
service operations do not load the CLI settings file; a starting service
validates its own runtime configuration.

The ownership marker embedded in installed files remains stable across CLI
renames so existing managed units stay recognizable.

`systemd uninstall` stops and disables the exact scope and removes only files
at the built-in installer's audited destinations. Settings, data, and the
system account are retained. It refuses unmanaged or changed files unless the
operator supplies `--force`; even with `--force`, it never deletes settings or
data. `systemd status` reports systemd's fragment path, state, and
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
able to read that file, while no other user should be able to read its token:

```sh
chown root:sing-box-panel /etc/sing-box-panel/setting.json
chmod 0640 /etc/sing-box-panel/setting.json
```

The default unit is suitable for unprivileged proxy ports. TUN, transparent
proxy, raw sockets, and ports below 1024 require an explicit local review. The
`examples/tun-override.conf` file shows the smallest expected capability and
device override; it is deliberately outside the auto-loaded unit directory.

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
directory as the sole writable persistent location.
