# systemd packaging

The panel and each node run as their own systemd service. Their unit, update
helper, sysusers and tmpfiles templates in
[`internal/sing-box-panel/systemd/templates`](../internal/sing-box-panel/systemd/templates)
and [`internal/sing-box-node/systemd/templates`](../internal/sing-box-node/systemd/templates)
are packaging inputs. CI validates source, build behavior, and generated units with `systemd-analyze verify`;
ordinary tests never create OS users or enable services. The explicitly opted-in
container test described below verifies real account creation and removal.

Each binary exposes its own templates through its `systemd` command group:
`sing-box-panel systemd` and `sing-box-node systemd`, with the same `install`,
`uninstall`, `status`, `start`, `stop`, `restart` and `logs` commands. Both
run one shared lifecycle, `internal/systemd`. Each service's state is its
settings file, named by `--config` in the unit's `ExecStart`, and the settings
select its data directory: the panel's database, or the node's enrollment,
`node.db` and installed cores. Those commands are Linux-only
and never invoke a shell. Every systemd scope is reported in command output;
`--scope=auto` resolves to `system` for root and to `user` otherwise.

Every service unit sets `SING_BOX_SYSTEMD_SCOPE` to its scope, `system` or
`user`, so the service knows how it is supervised when it updates itself.
Both services' units invoke `server start --config PATH` in the foreground and
set a marker of their own, `SING_BOX_PANEL_SUPERVISOR=systemd` or
`SING_BOX_NODE_SUPERVISOR=systemd`, to identify the service owner. Manual
terminal runs do not set that marker. They remain stoppable through
`server stop` even when the terminal inherited a generic systemd invocation
environment; the service itself is stopped through `systemd stop`.

```sh
# Dedicated system services, initially using the default data directories.
sudo /usr/local/bin/sing-box-panel systemd install --scope=system --now
sudo /usr/local/bin/sing-box-node systemd install --scope=system --now

# Ordinary per-user services. The current executable, state path, and data
# directory are rendered according to each directive's parsing rules.
sing-box-panel systemd install --scope=user --now
sing-box-node systemd install --scope=user --now
```

The built-in systemd installer writes to `/etc/systemd/system`,
`/etc/sysusers.d`, and `/etc/tmpfiles.d`; distro packages should continue to
use the `/usr/lib` destinations below. System scope requires root and fixes the
executable and state: `/usr/local/bin/sing-box-panel` with settings
`/etc/sing-box-panel/setting.json`, and `/usr/local/bin/sing-box-node` with
settings `/etc/sing-box-node/setting.json`. Data defaults to
`/var/lib/sing-box-panel` and `/var/lib/sing-box-node`;
a configured dedicated directory is rendered into the unit, the [update
helper](#update-helper)'s units and the tmpfiles rule. It creates
the dedicated account/directories, assigns ownership of the
state and data paths, reloads systemd, enables the unit, and enables and starts
the update helper's path unit. User scope
writes only the current user's XDG systemd unit.

Installing either service creates missing settings and prints the generated
password once, so `install --now` starts it at once. A node starts unenrolled
and waits to be enrolled from its [console](panel-node-architecture.md#node-console)
or with `sudo sing-box-node enroll`. Commands an administrator runs as root
against a service's files, such as `sudo sing-box-node enroll` or
`sudo sing-box-node config set`, keep the files they write owned by the file's
or data directory's owner, so the service account can still read them.
`server start` run as root refuses settings or data owned by another account,
such as the service account, and names `systemd start` instead: the service runs
as that account, and a root process would leave files it cannot use.

Before writing files, installation checks required commands, manager access,
permissions, paths, pending migrations and conflicting service files. It creates
missing settings (0600, administrator email and hashed random password) and directories (0700), without overwriting
existing configuration or initializing the database. The server creates storage
when it starts. Existing differing unit files still require `--force`.
Configuration sidecars must be regular files when present; conflicts are rejected
before resource creation or account preparation. Existing data directories need
write access to the directory itself, not to an otherwise shared parent.
`systemctl`, `systemd-sysusers`, `systemd-tmpfiles`, `getent`, and `chown` are checked only
when required; the CLI never runs a package manager. `systemd-analyze` is a test
and CI dependency, not a runtime requirement.

When the panel's settings are created, the CLI prints the same aligned URL, email, initial
password and paths as `config init` and `server start`, even if later service
activation fails. Existing settings do not regenerate or redisplay a password.
To recover access to the panel's system service, run:

```sh
sudo /usr/local/bin/sing-box-panel --config /etc/sing-box-panel/setting.json config set auth.password --stdin < ./password.txt
```

Supply a new password containing 12–128 Unicode characters in `password.txt`.
The command updates only its hash without echoing the password in output. It preserves the
service account's ownership and private file permissions. No restart or database
access is required; old sessions are rejected at their next authentication check.
For a user service, run the command as that user with its selected `--config` path.

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

The ownership marker embedded in installed files, `# Managed by NAME system
install.`, remains stable across CLI renames so existing managed units stay
recognizable.

`systemd uninstall` verifies the loaded state, disables and stops the update
helper's units in system scope, stops active services, then disables
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

Each service's name, `sing-box-panel` or `sing-box-node`, is reserved for its
installer's dedicated identity: the sysusers description, `/var/lib/NAME` home,
`/usr/sbin/nologin` shell, and private non-root UID/GID must match. Installation reuses a matching
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
Before stopping the service, it verifies that the loaded state path is known and
matches the installed one. Nonempty drop-ins, unresolved settings, and
`NeedDaemonReload=yes` block account removal; resolve them first or use
`--keep-user`. The effective configuration and stopped state are checked again
before ownership transfer. Disabling a unit refreshes systemd's enablement state
before that account check, so uninstall does not reject its own changes as stale.
Files are checked for replacement and in-place edits before account removal;
concurrent changes still interrupt uninstall. If disable removes enablement links
but its reload fails, those completed removals remain in the result.
Inactive orphaned installer files can still be removed
when their retained paths can be determined.

For a pre-existing `NeedDaemonReload=yes`, inspect the unit with
`systemctl cat NAME.service`, review its configuration, then run
`systemctl daemon-reload` before retrying. Diagnostics include these commands
and distinguish stale state from an unknown or mismatched state path.

The panel's settings writers and both services' database owners are excluded with the existing locks. Before
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
Only the panel's settings directory and each service's known default, configured,
installed and migration data roots are reclaimed; manually assigned ownership
elsewhere is outside this lifecycle. Keep the account when external resources still depend on its UID/GID.
Unresolvable or unsafe data locations must be repaired, or use `--keep-user`.

`systemd status` reports systemd's fragment path, state, and
`NeedDaemonReload`, plus on-disk facts labeled as such: the state path in the
single effective `[Service] ExecStart` line of the unit file on disk (its
`--config`) and the data directory that settings file selects, plus, for the
panel, the database and configuration storage declared by that settings file
as it exists now. The panel CLI's own `--config` path is listed separately. In
system scope it also reports the update helper's two units. Nonempty drop-ins,
including global `service.d/` ones that some container hosts add to every
service, unreadable units, several effective commands, or
unresolved specifiers leave the unit-file settings path unknown rather than
guessed, a unit edited after loading is flagged stale, and the running
process's settings are never inspected or claimed. `systemd logs --lines N
[--since VALUE]` performs one bounded journal query.

## System service

The panel's system template runs as the dedicated `sing-box-panel` user with an
empty `CapabilityBoundingSet` and `AmbientCapabilities`: the panel runs no
sing-box and reads no certificate files. `ProtectSystem=strict`,
`ProtectHome=true`, `NoNewPrivileges=true`, `ProtectProc=invisible` and the
other sandbox restrictions stay enabled.

The node's system template runs as the dedicated `sing-box-node` user with
`CAP_DAC_READ_SEARCH` in both `CapabilityBoundingSet` and `AmbientCapabilities`.
This allows the node and its child core process to read root-owned certificate
files and traverse their directories without changing certificate owners or modes.
The capability applies throughout the service's visible filesystem, not just
`/etc/letsencrypt`; it does not bypass normal file-write permission checks.
The same sandbox restrictions remain enabled. Hidden paths and mandatory access
controls can still deny access. See [Linux capabilities](https://man7.org/linux/man-pages/man7/capabilities.7.html)
and [systemd execution settings](https://github.com/systemd/systemd/blob/main/man/systemd.exec.xml).
A packager should install the following templates, relative to each service's
templates directory above:

| Source | Destination |
| --- | --- |
| `system/NAME.service` | `/usr/lib/systemd/system/NAME.service` |
| `system/NAME-update.path` | `/usr/lib/systemd/system/NAME-update.path` |
| `system/NAME-update.service` | `/usr/lib/systemd/system/NAME-update.service` |
| `sysusers.d/NAME.conf` | `/usr/lib/sysusers.d/NAME.conf` |
| `tmpfiles.d/NAME.conf` | `/usr/lib/tmpfiles.d/NAME.conf` |

For the panel, after creating the account and directories with the host's
normal packaging tools, initialize `/etc/sing-box-panel/setting.json`. The
runtime user must be able to atomically replace that file and create private
lock/recovery sidecars in its directory, while no other user should be able to
read its credentials:

```sh
chown sing-box-panel:sing-box-panel /etc/sing-box-panel /etc/sing-box-panel/setting.json
chmod 0700 /etc/sing-box-panel
chmod 0600 /etc/sing-box-panel/setting.json
```

For a node, enroll it or enable its console against `/var/lib/sing-box-node`
and give that directory to the `sing-box-node` account before starting it.

The node's default unit is suitable for unprivileged proxy ports. TUN,
transparent proxy, raw sockets, and ports below 1024 require an explicit local
review. The [`tun-override.conf` example](../internal/sing-box-node/systemd/templates/examples/tun-override.conf)
shows the smallest expected capability and device override; it is deliberately
outside the auto-loaded unit directory. Its capability assignments add to the
default read capability. An explicit empty capability assignment in a local
drop-in resets the corresponding set.

Host metrics are sampled by the node. Its system unit keeps
`ProtectProc=invisible` and `ProcSubset=pid`, bind-mounts only `/proc/stat`,
`/proc/meminfo`, and `/proc/loadavg` read-only into
`/run/sing-box-node/host-proc`, and sets `SING_BOX_NODE_HOST_PROC` so the
sampler reads these live counters. Direct and user-service runs use `/proc` by
default. Missing counters remain unavailable; disk sampling is independent.
Keep all three bind mounts and the environment setting together; do not expose
the whole host `/proc` tree. Check drop-ins that reset capabilities.
Certificate files, including Certbot's `live` symlinks and `archive` targets,
keep their existing ownership and permissions. Refresh an installed unit with
the binary that ships it, then restart the service:

```sh
sudo /usr/local/bin/sing-box-node systemd install --scope=system --force
sudo /usr/local/bin/sing-box-node systemd restart --scope=system
systemctl show sing-box-node -p User -p Group -p CapabilityBoundingSet -p AmbientCapabilities
```

Review customized units before replacement.

## Update helper

A system service runs as its dedicated account under `ProtectSystem=strict`, so
it cannot replace its own executable in `/usr/local/bin`. Each system
installation therefore keeps two companion units beside its service unit, which
`systemd install`, `uninstall` and `status` cover with it:

- `NAME-update.path` watches `updates/request` in the service's data directory. Installation enables and starts it whether or not the service starts; uninstallation disables and stops it before the service, so no update starts while the service is removed.
- `NAME-update.service` is a root oneshot that the path unit starts. It runs `sing-box-node update --apply-staged --data-dir DIR` or `sing-box-panel update --apply-staged --data-dir DIR`, with the data directory written into the unit at installation, never read from the service's settings.

The service downloads and verifies a release itself, stages it in
`updates/VERSION/` (the binary for its platform, `SHA256SUMS` and
`SHA256SUMS.sig`) and then writes `updates/request`, holding only the version.
The helper treats everything in the data directory as the service account's:

- It removes the request before any work, so a failed attempt does not start again, and accepts its content only as one strict release version, which it compares with the entries of `updates/` and never uses as a path.
- It opens the data directory and everything below it without following a symbolic link, reads only regular files within their size bounds, and copies the staged files into a new directory beside the executable, which only root writes, before reading them.
- It verifies the copies with the release key embedded in the installed binary, which it runs as: the signature over the version and checksums, the binary's checksum, its Go command, module and platform, and a version newer than its own. An equal or older version is refused.
- It installs under the executable's update lock, keeping the replaced binary as `NAME.previous`, restarts the service and waits up to 30 seconds for systemd to report it active for 5 seconds in a row. Otherwise it restores `NAME.previous` and restarts the service again.
- A service whose request is still there two minutes after it staged the release concludes that the helper did not start, for example because its path unit is disabled or hit systemd's start limit: it withdraws the request and the staged release and records a failed attempt with the code `helper_unavailable`, so the panel's update returns to idle and a node reports the attempt. A request that is gone means the helper took it, and the service waits for its result.
- It writes `updates/result` (version, success or a short failure code, and time) through the directory's descriptor, as a new file renamed into place, before each restart, so the service that starts reads the outcome; a refused release is recorded without a restart, and the running service picks the result up within seconds. The panel shows it and records it in its log, and a node reports it in its state report.

The helper unit is hardened like the service units: `NoNewPrivileges`,
`ProtectSystem=strict`, `ProtectHome`, `PrivateTmp`, `PrivateDevices`,
`ProtectProc=invisible`, the kernel, clock, hostname and control-group
protections, `RestrictNamespaces`, `RestrictSUIDSGID`, `MemoryDenyWriteExecute`
and `LockPersonality`. It has no network (`PrivateNetwork=true`) and may open
only Unix sockets, which it needs to ask systemd to restart the service. Its
only capability is `CAP_DAC_OVERRIDE`, to read and write the service account's
private updates directory, and it may write only the executable's directory and
the data directory (`ReadWritePaths`). Its `UMask` is `0022` and it gives the
result mode 0644, so the service can read it.

A user service and a process in the foreground replace their executable
themselves when they may write it; a user service then asks its manager to
restart its unit. The `update` commands do the same for an operator who runs
them with write access to the executable.

## User service

The static packaging user units expect:

| Service | Binary | State | Data |
| --- | --- | --- | --- |
| Panel | `~/.local/bin/sing-box-panel` | settings `~/.config/sing-box-panel/setting.json` | `~/.local/share/sing-box-panel` |
| Node | `~/.local/bin/sing-box-node` | its data directory | `~/.local/share/sing-box-node` |

Create and initialize those paths before enabling a unit. The user templates
grant no capability and cannot provide TUN, transparent-proxy, raw-socket, or
privileged-port access. They are intended for ordinary user-owned proxy
listeners.

Both services use restart-on-failure and stop the complete process group. Both
set `NoNewPrivileges`, `UMask=0077` and `RemoveIPC`, may open only Unix, IPv4,
IPv6, and netlink sockets, and filter system calls with
`SystemCallArchitectures`, `RestrictNamespaces`, `RestrictRealtime`,
`RestrictSUIDSGID`, `MemoryDenyWriteExecute` and `LockPersonality`.

The panel's user unit also empties its `CapabilityBoundingSet` and confines the
file system with `ProtectSystem=strict`, `ProtectHome=read-only`, `PrivateTmp`,
`PrivateDevices` and the kernel, clock, hostname and control-group protections;
only the selected settings and data directories are writable. The panel's
system template explicitly permits `/etc/sing-box-panel`.

The node's user unit does not confine the file system or the kernel interfaces,
and its bounding set is not emptied. A user manager applies those settings only
by running the service in a user namespace that maps only its user, and there
`/`, `/home` and every other root-owned directory appear owned by `nobody`. The
node's artifact store requires every ancestor of its data directory to be owned
by root or the node's user and not writable by others, so in that namespace it
refuses to run a core; and outside one, an empty `CapabilityBoundingSet` cannot
be applied and the unit fails to start. The node's user unit therefore keeps
only the settings that apply without a namespace, listed above. A user-scope
node runs with its user's full file access: it, its cores and anything they run
can read and write wherever that user can. Use the system service where the
node must be confined to its data directory.

After upgrading an older installation, rerun `systemd install` to refresh the
unit and ownership before saving preferences through the Web UI. Use `--force`
when replacing an older generated template after reviewing changes.

After editing the panel's `data_dir`, an explicit `systemd restart` coordinates a stopped
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

`go test ./internal/systemd ./internal/sing-box-panel/systemd ./internal/sing-box-node/systemd ./internal/sing-box-panel/cli`
covers account conflicts, retained data, partial failures, retries and output
contracts using the existing command runner, through the panel's service and,
for its runnable-state check and ownership, the node's.
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
