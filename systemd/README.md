# systemd templates

These files are packaging inputs. CI validates source, build behavior, and generated units with `systemd-analyze verify`;
ordinary tests never create OS users or enable services. The explicitly opted-in
container test described below verifies real account creation and removal.

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
the dedicated account/directories, assigns ownership of the
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
`systemctl`, `systemd-sysusers`, `systemd-tmpfiles`, `getent`, and `chown` are checked only
when required; the CLI never runs a package manager. `systemd-analyze` is a test
and CI dependency, not a runtime requirement.

`WorkingDirectory` is a scalar path: quotes and backslashes are literal, while
percent specifiers must be escaped. Execution arguments, path lists and tmpfiles
fields use their respective quoted forms. Unrepresentable paths fail preflight.

Installation without `--now` reads and validates only the configured data path;
`--now` also requires valid runtime settings before any installation starts.
Status uses only location fields for its optional settings report. Other
service operations do not load the CLI settings file. System uninstall reads
the installed service's own data locations before releasing its account.
Start/restart inspect the
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
at the built-in installer's audited destinations. Settings and data are retained.
System scope also removes the dedicated service user and its empty private group
by default, after transferring their ownership of the retained settings and data
to root. File contents and modes are preserved; ownership changes do not follow
symlinks, including links to externally stored certificates. User scope never
creates or removes OS accounts. It refuses unmanaged or changed files unless the
operator supplies `--force`; even with `--force`, it never deletes settings or
data. Missing or broken inactive units can be removed; manager, permission and
stop failures remain errors, with already removed paths preserved in results.
Even when all installed files are missing, uninstall checks the manager: a cached
running unit must have a matching fragment path and stop successfully before
uninstall can succeed. An absent inactive unit requires no changes.

The `sing-box-panel` name is reserved for the installer's dedicated identity:
the sysusers description, `/var/lib/sing-box-panel` home, `/usr/sbin/nologin`
shell, and private non-root UID/GID must match. Installation reuses a matching
account and refuses unrelated or shared identities before preparing resources.
Removal additionally requires the exact installer-owned sysusers declaration;
this also recognizes accounts created by earlier versions. A missing declaration
leaves the account untouched and reports why. Customized accounts/declarations,
shared IDs, or additional group members require `--keep-user` to uninstall service
files without changing accounts or data ownership. `--force` never bypasses those
account checks. The same retention option can be used for temporary service removal:

```sh
sudo /usr/local/bin/sing-box-panel systemd uninstall --scope=system
# Retain the account and existing ownership instead:
sudo /usr/local/bin/sing-box-panel systemd uninstall --scope=system --keep-user
```

Account cleanup requires `getent`, `chown`, `userdel`, and `groupdel` when needed.
Before stopping the service, it verifies that the loaded settings are known and
match the installed configuration. Nonempty drop-ins, unresolved settings, and
`NeedDaemonReload=yes` block account removal; resolve them first or use
`--keep-user`. The effective configuration and stopped state are checked again
before ownership transfer. Inactive orphaned installer files can still be removed
when their retained paths can be determined.

Settings writers and database owners are excluded with the existing locks. Before
any ownership transfer, the installer checks `/proc` thread credentials for the
service UID/GID, including supplementary groups. Busy or uninspectable processes
abort cleanup; unrelated processes are never killed. This check does not depend
on `userdel` being able to access another process's root directory. Administrators
must not concurrently launch new processes under the service identity during
uninstall. It never uses forced account deletion or `userdel --remove`. Ownership
transfer failures and failed account commands also preserve service files for a
retry. Results include `account_inspected`, `account_retained`, `group_retained`,
`account_removed`, `group_removed`, and an optional `account_note`, including on
partial failure. Presence is inspected even with `--keep-user` or a missing
sysusers declaration. Repeated uninstall reports absent accounts as absent;
failed inspection is unknown, not evidence of absence. A retry tolerates an
already removed user or private group.
Reinstalling creates the account again and restores its ownership of retained data.
Only the settings directory and known default, configured, installed and migration
data roots are reclaimed; manually assigned ownership elsewhere is outside this
lifecycle. Keep the account when external resources still depend on its UID/GID.
Unresolvable or unsafe data locations must be repaired, or use `--keep-user`.

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

The system template runs as the dedicated `sing-box-panel` user with
`CAP_DAC_READ_SEARCH` in both `CapabilityBoundingSet` and `AmbientCapabilities`.
This allows the panel and its child core process to read root-owned certificate
files and traverse their directories without changing certificate owners or modes.
The capability applies throughout the service's visible filesystem, not just
`/etc/letsencrypt`; it does not bypass normal file-write permission checks.
`ProtectSystem=strict`, `ProtectHome=true`, `NoNewPrivileges=true` and the other
sandbox restrictions remain enabled. Hidden paths and mandatory access controls
can still deny access. See [Linux capabilities](https://man7.org/linux/man-pages/man7/capabilities.7.html)
and [systemd execution settings](https://github.com/systemd/systemd/blob/main/man/systemd.exec.xml).
A packager should install the following files:

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
Its capability assignments add to the default read capability. An explicit empty
capability assignment in a local drop-in resets the corresponding set.

Host metrics keep `ProtectProc=invisible` and `ProcSubset=pid`. The system unit
bind-mounts only `/proc/stat`, `/proc/meminfo`, and `/proc/loadavg` read-only into
`/run/sing-box-panel/host-proc`, and sets `SING_BOX_PANEL_HOST_PROC` so the sampler
reads these live counters. Direct and user-service runs use `/proc` by default.
Missing counters remain unavailable; disk sampling is independent. Older system
units hide these files without providing the mounts. Older units also grant no
read capability, so upgrading the binary alone does not restore metrics or add
root-owned certificate access. Refresh the installed unit
using the upgraded binary, then restart the service. For an uncustomized generated
system unit:

```sh
sudo /usr/local/bin/sing-box-panel systemd install --scope=system --force
sudo /usr/local/bin/sing-box-panel systemd restart --scope=system
systemctl show sing-box-panel -p User -p Group -p CapabilityBoundingSet -p AmbientCapabilities
```

Review customized units before replacement. Keep all three bind mounts and the
environment setting together; do not expose the whole host `/proc` tree.
Check drop-ins that reset capabilities. Certificate files, including Certbot's
`live` symlinks and `archive` targets, keep their existing ownership and permissions.

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

## Account lifecycle verification

`go test ./internal/systemd ./internal/cli` covers account conflicts, retained data,
partial failures, retries and output contracts using the existing command runner.
`TestSystemAccountLifecycleInContainer` additionally requires Linux, root,
`/.dockerenv`, and `SING_BOX_PANEL_TEST_SYSTEM_ACCOUNTS=1`. Run it only in a disposable
Docker container with systemd tools, shadow account tools, GNU chown, getent and
setpriv installed, with `--cap-add=DAC_READ_SEARCH`. It exercises real creation,
read access, ownership transfer, removal and reinstallation against fixture data;
only the systemctl manager is simulated. Containers do not access host accounts.
`TestBusySystemAccountInContainer` uses the same opt-in and tools, without extra
container capabilities. It verifies that an independently running service user
blocks uninstall before ownership changes, and that cleanup succeeds after the
process exits.
