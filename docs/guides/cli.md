# CLI reference

The sing-box-panel CLI manages the panel settings file, exact sing-box artifacts,
runtime state, subscriptions, and operational evidence. Sing-box configuration
content is managed through the Web UI. Running the root command or a command
group without a leaf prints help.

## Command hierarchy

```text
sing-box-panel
├─ init | version | update
├─ server start | stop | status
├─ system df | prune
├─ systemd install | uninstall | status | start | stop | restart | logs
├─ core
│  ├─ catalog | refresh
│  ├─ list | show | install | import | remove
│  └─ enable | status | start | stop | restart | rollback
├─ config init | show | set | unset | verify
├─ channel list | show | create | update | delete | render
├─ source list | show | create | update | refresh | delete
├─ token list | create | rotate | revoke
├─ log list | show | tail | clear | delete
├─ metrics show | watch | history | period PERIOD_ID
└─ completion bash | zsh | fish
```

Every command is at most two words deep; each group adds one verb, and each
verb maps to one application operation. `channel` and `source` manage
subscription resources: channels rendered for public subscription clients and
upstream sources attached from third parties. `token` manages public
subscription access tokens; it never manages the panel management login
token. `server start` runs the panel in the current terminal. Background
operation belongs to the `systemd` service commands.

Service commands previously under `system` now use `systemd`. File inspection
uses `system df` (formerly `system files`); cleanup uses `system prune`
(formerly `system clean`). Update scripts and regenerate shell completions;
the old service paths, `system files`, and `system clean` are not aliases,
and there is no `prn` alias.

Use `sing-box-panel COMMAND --help` at any level for current flags and leaf
commands. Help lists usage, local flags, inherited global flags (when present),
and available commands, in that order. Root usage is shown on one line as
`sing-box-panel [flags] [command]`; command groups similarly combine flags and
subcommands. Global flags may appear before or after subcommands, and invoking
a group without a subcommand still displays help. Leaf usage retains its
command-specific arguments. The HTTP/Web management surface
additionally exposes subscription user profiles, grant matrices, and
source-version history.

Within Flags, `-h/--help` appears first, followed by the other flags in
alphabetical order. Global flags and available commands retain alphabetical
ordering.

## Global flags and output

- `-c, --config PATH` selects one settings file.
- `-o, --output=text|json|jsonl` selects human or machine-readable output.

Both `-o json` and `-o=json` select JSON.
It is the same persistent flag as `--output`, so either form may appear before
or after subcommands; `text` and `jsonl` work the same way.

`-c` and `--config` are short and long forms of the same flag. When omitted,
commands that need settings load the default path: root uses
`/etc/sing-box-panel/setting.json`; ordinary users use
`$XDG_CONFIG_HOME/sing-box-panel/setting.json`, or
`~/.config/sing-box-panel/setting.json` when XDG is unset. An explicit path
overrides that default; commands never silently load another file. `server start` and `systemd install`
create defaults at the selected path when it is missing. `config init` creates
only default settings explicitly. `config set --file`
can also create a selected file from validated input. Other commands that require settings
reject missing files; `config show` can display invalid text.
Repeated flags use the last supplied value.

Selecting a settings path does not load it. Help (including bare command groups),
`version`, shell completion, and `update` do not read
panel settings or open its database. `systemd uninstall`, `start`, `stop`,
`restart`, and `logs` operate on the selected service scope without loading the
CLI settings file. Start/restart can read the installed unit's own settings to
prepare a requested data relocation or restore missing resources. System uninstall
also reads that installation's data locations to protect retained data before
removing its dedicated account; `--keep-user` preserves the account and ownership.
See [service lifecycle and permissions](../../systemd/README.md).
`systemd status` also works with unavailable settings; its
optional location report marks unreadable files or invalid `data_dir` fields as
unavailable without hiding systemd's status.

`system df`, `system prune`, `server status/stop`, `systemd install` without
`--now`, and local database operations read only `data_dir` to locate the
instance. When settings exist, these commands reject empty, wrongly typed, or ambiguous paths and
malformed JSON; unrelated runtime fields such as `traffic.sample_retention_days`
do not block them. Relative data paths resolve against the settings file. After a directory edit,
status/stop and database commands keep using the recorded current location until
the next explicit start completes relocation.
If the settings file itself is missing, `system df` and the `system prune`
preview still report known paths. A valid `.location` record can identify current
data when configuration is missing. Cleanup history only discovers old paths;
it cannot authorize deleting them. Repeated cleanup with no current targets succeeds.
Service ownership, symlink, locking, and cleanup-scope checks still apply.
Database identity restricts storage operations, but does not prevent confirmed
full-directory cleanup. Metrics read and validate `traffic.quota_gib` only when needed,
directly from the shared settings file.

`config verify`, `server start`, and `systemd install --now` require the complete
valid panel settings. Startup checks the runtime environment and database;
`config verify` reads the settings file alone. `server start` first creates default
settings if the selected file is absent, including parent directories, the default data directory,
and a random management token. The settings file uses mode `0600`; new directories
use `0700`. This also applies to an explicit `--config` path. Concurrent first
starts cannot replace each other's settings. A damaged, unreadable, or dangling
symlink file is never replaced. Database initialization remains part of startup.
Installing or starting an existing generated unit prepares missing settings and
directories before systemd starts it. It validates required external commands and
manager access before initialization, preserves existing settings and tokens, and
never creates the database during installation. Customized or ambiguous units
require explicit attention; start/restart does not install a missing unit.
`init` explicitly creates settings and initializes storage. It refuses to overwrite an existing file
unless `--force` is supplied. No command silently repairs a damaged file.

When `server start` creates settings, it prints `sing-box-panel settings is created`
to stderr, followed by aligned `Default URL`, `Default Token`, `Default Settings`,
and `Default Data Dir` rows. This reports initialization, not HTTP readiness.
It does not repeat the summary when the file already exists. Once the server is
ready, it prints `sing-box-panel is running` with aligned `Panel URL`, `Settings`,
and `Data Dir` rows, followed by the stop shortcut and stored-log command.
Color is limited to text on a terminal
and respects `NO_COLOR` and `TERM`. In JSON/JSONL mode, stderr receives one event
with `event: "settings_initialized"`, `settings_path`, `data_dir`,
`default_panel_url`, and `login_token`; stdout remains free of startup guidance.
First-run output contains the newly generated token in both terminal and redirected
output, including JSON/JSONL.

Results are written to stdout. Progress, warnings, and terminal errors are
written to stderr, allowing scripts to redirect them independently. JSON and
JSONL errors contain `code`, `message`, and `exit_code`; underlying causes are
not serialized because they may expose filesystem or upstream details.

`version` prints only the program name and version, such as `sing-box-panel
v1.2.3`. Release, prerelease, and Go module pseudo-versions are shown unchanged,
including a `+dirty` suffix when present. A build without a usable version
number displays `sing-box-panel unknown`. Use `version --output=json` or
`--output=jsonl` for the full, unchanged `version`, `commit`, and `date` metadata,
including any original placeholder values.

Local `make build` uses Go's module and VCS metadata without injecting a build
timestamp. A pseudo-version's timestamp identifies the source commit, not the
time the binary was compiled; `+dirty` records uncommitted source changes.

Complete panel settings documents, subscription source definitions, and other bulk
or secret-bearing values use `--file PATH` or `--file -` for stdin. Do not
place secrets in command arguments. Panel settings output and
subscription source details may contain credentials and must be handled as
secret-bearing output.

## Panel settings

`config` manages the panel's `setting.json` selected by `-c/--config`.

```sh
sing-box-panel config init --config ./setting.json
sing-box-panel config show --config ./setting.json
sing-box-panel config verify --config ./setting.json
sing-box-panel config unset server.port /github/catalog_refresh_interval_hours --config ./setting.json
sing-box-panel config set --config ./setting.json --file ./new-setting.json
sing-box-panel config set --config ./setting.json --file - < ./new-setting.json
```

- `init` generates a default settings file with a random login token, without
  creating the data directory, opening SQLite, or starting a service. New settings
  directories use `0700` and the file uses `0600`. Existing files are preserved;
  `--force` explicitly replaces a regular file and generates a new token.
  Symlinks and pending settings/data relocation recovery are rejected even with
  `--force`. Location metadata is retained for the next startup relocation.
  Text output displays the file path and login token; JSON/JSONL returns
  `initialized: true`, `settings_path`, and `login_token`. Top-level `init` retains
  its broader responsibility of also initializing storage.
- `show` returns exact file bytes, even when the JSON or settings are invalid.
  JSON/JSONL returns `settings_path` and a `content` string. The content includes
  credentials; the Web UI reads and writes this same file.
- `verify` validates strict JSON and the complete panel settings contract.
  It returns `valid: true` and `settings_path` on success. It never creates or
  migrates a database, checks directory availability, or runs sing-box.
- `set --file FILE|-` replaces the complete document. Validation finishes before
  the destination is changed; invalid input leaves the existing file intact.
  Relative `data_dir` paths resolve against the destination settings file.
  The write is atomic with mode `0600`; new settings directories use `0700`.
  Symlinks and other non-regular destinations are rejected. The data directory
  and database are untouched. JSON/JSONL returns `saved: true` and `settings_path`.
- `unset FIELD [FIELD...]` restores selected fields or entire sections from the
  same defaults as `init`. Dotted names (`server.port`) and slash paths
  (`/server/port`) select the same field. Arrays reset as a whole. Multiple
  fields are reset in one atomic write, so related values such as
  `server.external_origin` and `auth.secure_cookie` can be reset together.
  The complete result must validate: unknown fields and required values without
  defaults, including `auth.token` or the whole `auth` section, are rejected
  without changing the file. Unselected values, including relative paths and
  credentials, are preserved. Missing files are not initialized. JSON/JSONL
  returns `saved: true`, `settings_path`, and `reset_fields` without their values.
  Resetting `data_dir` selects the current effective user's default directory;
  the old directory remains active until the existing startup relocation runs.

Settings documents are limited to 1 MiB. `set`, `unset`, and
`verify` reject unknown fields, duplicate keys, trailing JSON, and invalid
settings values.
`set` can replace an invalid existing file or create a missing file; it does not
merge fields or restart a running panel. Restart to reload startup fields such
as the listener and data path. Tokens, Web preferences and quota are read from
this file at operation boundaries. File edits invalidate an older Web form's
revision, so its next save returns a conflict instead of overwriting the edit.
See [the field mapping and effect timing](configuration-and-runtime.md#shared-settings-file).

Writers coordinate through a private `setting.json.lock` beside the selected
file. A temporary `setting.json.pending` journal protects Web settings saves
and atomic restoration of settings and sing-box configuration backups. While recovery is pending, file commands
fail closed; start the panel to finish recovery before editing. A private
`setting.json.location` also records the established data directory and any
pending move. These sidecars appear in `system df`; cleanup removes idle metadata
and refuses an unfinished relocation.
Do not remove recovery material to bypass a conflict.

The top-level `verify` and duplicate `config check` commands are removed; use
`config verify`. Update scripts that used `config check`.
The former sing-box configuration interfaces and their revision and core
flags remain removed. The current `config verify`, `set`, and `unset`
operate only on panel settings. Move sing-box editing, validation, and Apply
workflows to the Web UI, and regenerate shell completions after upgrading.
The Web editor continues to
store exact sing-box text in SQLite with revision conflict detection; this
change does not create a separately editable sing-box file on disk.

## Exact core selection

Executable sing-box configuration is always the current valid document saved
through the Web UI. Select an exact installed artifact there for
Check or Apply. The CLI retains explicit core switching:

```sh
sing-box-panel core enable 1.13.21
```

`install`, `show`, `enable` and `remove` take a complete version or `v` tag.
The default is this machine's architecture and musl; `--arch` is explicit.
When multiple installations match, copy `BUILD` from `core list` and pass
`--build`. The old asset/artifact ID positional arguments, `catalog --installable`
and import `--sha256` are removed. Full IDs remain in machine output and HTTP
management paths. See [core commands and migration](core-versions.md#cli-and-http-migration)
for examples and refresh/cache behavior.

`core enable` preserves the stopped/running state: a stopped core is selected
without launching it, while a running core is restarted after validation.
`core status` retains `enabled_core` when stopped.
`core enable` carries the saved JSON forward unchanged. It never merges, fills,
migrates, or rewrites fields for another version. The selected binary must
accept an execution snapshot with `sing-box check` before the running process
is replaced. Snapshot formatting may differ, but field names and values are
preserved. A failed preflight leaves the live core and saved document unchanged.

Missing JSON Schema disables only structured editing in the Web UI. Raw JSON
check and Apply there, and core enable/start/restart/rollback in the CLI, retain
the selected binary's native check as the final gate. Version selection requires an exact match and never silently selects a nearby release.

The CLI does not expose the internal startup-artifact and activation-bundle
steps; the HTTP API still exposes them for the Web UI. The monitoring tier is
not a CLI input either: a checked restart derives the evidence tier from the
saved configuration itself, `limited` when the file already exposes a usable
Clash API and otherwise `process_only`; nothing is injected into the file to
create that endpoint. Rollback uses the previous immutable bundle and its own
configuration and binary evidence.

## Operation completion and cancellation

Core download/import, catalog refresh and source refresh execute directly and
return completed results without requiring the server. Configuration checks and
core process controls require the active server and use its private Unix socket;
that server serializes them under the existing process lease. Interrupting the
caller cancels work at its next safe boundary. Logs record completed outcomes.
There is no generic queue, operation polling command, or `--detach` flag.

## Foreground panel control

```sh
sing-box-panel server start --config ./setting.json
# In another terminal, using the same settings/data directory:
sing-box-panel server status --config ./setting.json
sing-box-panel server stop --config ./setting.json --timeout 30s
```

After the listener is ready, `start` prints the actual panel URL, settings and data
paths and log location, then streams sanitized panel logs. Color is enabled only
for a terminal and can be disabled with `NO_COLOR`; JSON output remains structured.

`start` occupies the current terminal until `Ctrl+C`, `SIGTERM`, or a separate
`server stop` request shuts it down. It never detaches or creates a background
child. A second panel cannot run against the same data directory. The bare
`server` group prints help; there is no `server run` alias.

`status` reports the live process state (`starting`, `ready`, or `stopping`),
settings path, data directory, listener, and whether it belongs to a terminal
or systemd. JSON also includes the process ID, start time, and version. An
absent or stale control endpoint reports `stopped`; permission, timeout, and
protocol failures remain errors rather than implying the process stopped.

`stop` requests graceful shutdown through a private Unix socket. Success means
HTTP requests, workers, the managed core, and database cleanup have completed
and the data-directory lease has been released. It does not signal a stored
process ID. A timeout ends the caller's wait without force-killing the panel;
check `server status` or wait again with `server stop`. Stopping an already
stopped panel succeeds. A systemd-managed panel directs manual stop callers
to `systemd stop` instead.

Status and stop require readable settings with a valid `data_dir`, so an
unrelated invalid runtime setting cannot prevent stopping the panel. Startup
requires the complete valid settings. Keep the selected settings path unchanged while running. A `data_dir` edit is
persisted immediately but relocates storage only during the next explicit start;
stop/status still find the original instance. The private
`panel-control.sock` path inside `data_dir` must fit the platform's Unix socket
path limit. A stale socket is replaced only after acquiring the runtime lease;
regular files and symlinks at that path are never replaced.

For unattended/background operation, use `systemd install` and `systemd start`.
The installed unit invokes the same foreground `server start` entry point;
systemd owns its background lifecycle and restart policy.
The unit sets a panel-specific supervisor marker; an invocation environment
inherited from a terminal service does not make a manual panel systemd-managed.

## Metrics and traffic periods

`metrics show` displays one metrics snapshot; `metrics watch` refreshes it
until interrupted. Both include current traffic-period start/end times and
the period's cumulative traffic when reliable evidence is available. Missing
traffic evidence is shown as unavailable rather than zero usage.
`metrics history` lists saved periods and `metrics period PERIOD_ID` shows
one period's details. There is no separate `traffic` command group.

## Instance files and cleanup

```sh
sing-box-panel system df                          # uses the default settings
sing-box-panel system df -c ./setting.json --output json
sing-box-panel system prune -c ./setting.json      # preview only
sing-box-panel system prune -c ./setting.json --yes
```

`df` reports the executable, selected settings, service files, and every
existing entry beneath the selected data directory, including unknown files and
old-version residue. It does not initialize or migrate settings or databases.
Database identity is diagnostic in JSON/JSONL; an unknown or older identity does
not exclude data from confirmed cleanup. Inspection leaves incomplete WAL
recovery state untouched. `--scope auto|user|system` selects the systemd files.

After cleanup, or before initialization, `df` works without the selected settings
file. It reports actual existing resources without creating anything. The config
summary is marked missing; JSON retains a settings entry with `state: "missing"`.
A valid `.location` supplies current data when settings are missing. Otherwise
`data_dir` is empty, with no guessed default. Invalid or unreadable resources
produce a nonzero exit status and a partial report with `warnings`.

Before destructive cleanup, the CLI atomically saves private discovery history:
root uses `/var/lib/sing-box-panel-state/cleanup-history.json`; other users use
`$XDG_STATE_HOME/sing-box-panel/cleanup-history.json` (default
`~/.local/state/sing-box-panel/cleanup-history.json`). Its directory is 0700 and
file 0600. Writers lock the directory. Records contain normalized instance paths,
deduplicated data/service roots, scope, time, outcome/counts and service-account
cleanup outcomes, never tokens or
configuration contents. Corrupt, linked or unwritable history blocks destruction;
a failure to update the final outcome is reported as an incomplete operation.
Data roots containing the history directory are rejected. This protection also
applies to both ends of a data migration, so migration cannot move the fixed
history location along with instance data.

History and the binary are retained and displayed. `df` and previews never
rewrite history; `prune --yes` with no targets also leaves it untouched and exits
successfully. Text says `No removable resources remain for this instance.`
Historical paths that reappear are labeled as requiring ownership confirmation,
inspected without descending into them and retained. History alone never grants
delete authority or influences initialization, migration or configuration choice.
Continue selecting custom instances with the same `--config` path. Reinstallation
uses normal initialization rules rather than restoring historical configuration.

Text output starts with aligned `Config` and `Executable` paths:
the selected settings file (including a custom `-c` path) and the full path of
the running binary reported by inspection. There is no Data summary row, whether
the data directory exists, is missing, or cannot be determined. Existing data
contents still appear in the tree, and JSON/JSONL retain the `data_dir` field.
`Config` includes the filename and adds `(missing)` when the file is absent;
there is no separate `Settings` summary line. The `prune` preview uses the same format.
The tree retains the actual filenames and groups real paths in sorted order
under a visible filesystem root (`/` on Unix). Settings, service, executable and
data paths share this root instead of appearing as separate trees. The root
remains visible even for a single branch; unlabeled parent chains below it are
compressed. Cleanup results use the same rooted layout.
It shows no expected-but-missing data files
and marks retained resources explicitly. Directories end in `/`; `empty` comes
from actually reading the directory, never inferred from missing report children.
Symlinks (including dangling links) show `link`. Link targets are never traversed.
A missing data directory has no tree entry or separate status message. Parent
grouping nodes are structural, not separately inspected entries. Summary paths
inside the current user's home may use `~`; similar prefixes remain unchanged.
Tree paths retain their actual directory names beneath the filesystem root.

Existing service files appear in the same tree, labeled with their resolved
`user` or `system` scope. Inspection includes the installer's unit and auxiliary
files even when the unit is missing, matching enablement links, exact unit drop-in
contents and the existing system runtime directory. Related custom content is
retained; inventory does not expand deletion authority.
Missing service files are hidden; files not both managed and matched to the
selected settings are labeled `outside scope`. Unsupported platforms say that
systemd is unsupported, rather than implying no service is installed.

The tree uses color only for terminal text output: blue directories, dim tree
branches, cyan links or retained results, green completed removals, and red
interrupted-cleanup headings. Only the `prune` preview ends with a blank line
and a one-line yellow reminder to pass `--yes` to stop the instance and permanently
delete its settings and all data when removable resources exist.
Pipes, redirected output, an unset or
`dumb` `TERM`, and nonempty `NO_COLOR` disable all ANSI styling. JSON/JSONL remain
uncolored and keep their report field names and absolute paths; `-o` is shorthand
for `--output`.

`prune` without `--yes` is a read-only preview. **Move everything you want to keep
outside the selected data directory before confirming.** With `--yes`, cleanup
stops the selected instance, uninstalls its matching managed service, removes the
selected settings, and completely removes the data directory and its contents.
This includes unknown files, unrecognized/old databases, logs, cores, uploads,
and arbitrary nested directories. Files inside that directory are no longer
retained based on a known-name list or database identity. The JSON `cleanup`
values reflect this scope: data entries use `remove` or `remove_link`, and the
data directory uses `remove`. Missing preset children are no longer synthesized.

Filesystem roots, home/shared directories and their aliases, linked data roots,
linked settings files, and a data directory containing the panel executable are
refused. Ambiguous/changed service settings and another service's overlapping
data directory also prevent cleanup. Cleanup also refuses to remove paths that
another inspected service uses for its settings or data, including parent aliases.
Runtime and database-directory locks must
be acquired before deletion; active panel/CLI owners or unusable lock paths
prevent it. Settings and directory identities are checked again under those
locks. Each nested directory stays locked while its contents are removed, and an
existing runtime lease is checked too; another active panel or CLI owner blocks
that branch. Symlinks are removed without traversing their targets. New entries
that appear after enumeration prevent directory removal instead of being deleted
without ownership checks. Failure to remove all contents is an error, not a
successful partial wipe.

For a manually started instance, cleanup waits for the panel's shutdown result.
The panel stops and joins its managed core, closes its database, and releases its
runtime lease before acknowledging success. A matching systemd installation is
stopped and disabled before its service files are removed. A failed or timed-out
stop aborts data cleanup; unrelated instances are never stopped to clear a lock.

The executable and files outside the selected data directory remain outside the
cleanup scope, except for the selected settings and matching managed service.
Uninstalling a matching managed system service also cleans up its dedicated
account under the [systemd lifecycle rules](../../systemd/README.md); unrelated
OS accounts and journal records are retained. The conventional
`sing-box-panel` settings directory is removed only if empty; shared parent
directories are never recursively removed.

Execution results preserve `removed` and `retained` arrays and add `remaining`
and `warnings`. Remaining means a deletion target still exists; retained means
policy intentionally preserves it. When a system service is uninstalled,
`service_account` records `user`, `group` and an optional `note` in JSON and cleanup
history; text output reports the same outcome. Each identity is `removed`,
`absent`, `retained`, or `unknown`. Retained or unverified identities produce a
warning and a nonzero incomplete-cleanup exit status, even if file cleanup
succeeds. This optional history field is additive; existing records without it
remain readable and do not imply any account state. A final read-only inventory uses both current
and pre-cleanup paths, even after configuration has been removed. Each command
writes one complete result to stdout; diagnostics go to stderr. Deleted paths remain visible in results.
On interruption, text says `Cleanup interrupted; confirmed results only`, and the
command returns its error. The service may already be stopped or uninstalled and
some files removed when a later operation fails. When settings are inside a
subdirectory, other contents of that branch are removed before the database and
settings. A failure in those contents preserves the settings and database so the
operator can fix the cause and retry. This is not a transactional rollback;
failures during final removal can still leave partial results.
Cleanup is irreversible; a new
instance can be created with `systemd install --now`, `server start`, or `init` afterward. Scope is limited to the selected
settings and data directory, not every instance on the host.

## System service status

`systemd status` combines systemd's view of the unit with two facts read from
disk, each labeled with its source. It does not inspect the running process,
so it never claims which settings that process started with:

```sh
sing-box-panel systemd status --scope=user
sing-box-panel systemd status --scope=system --output json
```

The `service` object carries systemd's own answers: load, active, sub, and
enablement state, `main_pid`, `unit_path` (the fragment path), and
`need_daemon_reload`. `unit_file` (`source: "unit file on disk"`) describes
the file currently at that path. `unit_file.settings_path` is the `--config`
argument of its single effective `[Service] ExecStart` line, honoring empty
resets, systemd quoting, the literal `%%`/`$$` escapes the installer writes,
and the server's last-value semantics for a repeated flag. It is omitted and
`unit_file.settings_state` is `unknown` when the unit is unreadable, uses
drop-in overrides, has no or several effective commands, or references
specifiers or variables that only systemd can resolve. `unit_file.stale` is
true when systemd reports the file changed since it was loaded; the path then
describes the file on disk, not the command line systemd will run until
`daemon-reload` and a restart. `cli_settings_path` is the `--config` value of
the current command, reported separately; `unit_file.matches_cli_settings`
compares the two when both are known. `live_settings_state` is always
`unknown`.

`settings_file` (`source: "settings file on disk"`) reads the file at
`unit_file.settings_path` as it exists now. `state: loaded` means its location
was read successfully, and supplies `data_dir`, `database_path`, and
`configuration.database_path`; it does not certify the full runtime settings.
`unavailable` means the file or its location could not be read or validated;
`unknown` means no path was determined. Neither case hides the unit state,
and the command never prints
settings content or parse details. `configuration` always names the logical
`config.json` and its storage: the `configuration_file` table of the service
database.

## Exit codes and signals

The process uses stable high-level exit categories:

| Code | Meaning |
| ---: | --- |
| `0` | Success |
| `1` | Domain failure or unclassified internal failure |
| `2` | Command or flag usage error |
| `3` | Invalid settings, input, or validation result |
| `4` | Revision, identity, or state conflict |
| `5` | Permission failure |
| `6` | Required application or runtime state is unavailable |
| `130` | Interrupted or canceled, including `SIGINT` |
| `143` | Terminated by `SIGTERM` |

`SIGINT` and `SIGTERM` cancel the command context first so active operations
can stop at their defined boundaries. Failed operations return a nonzero exit
code; successful mutations return the resulting resource or observed runtime state.

## Shell completion

Completion generation opens no settings, database, or network connection. The
supported shells are Bash, Zsh, and Fish.

### Bash

Bash completion includes command descriptions and requires the
`bash-completion` package. Install and load that package before loading the
generated script. For Debian and Ubuntu containers:

```bash
apt-get update
apt-get install -y bash-completion
source /usr/share/bash-completion/bash_completion
source <(sing-box-panel completion bash)
```

Use `source <(...)` as shown above; unquoted `eval "$(...)"` does not preserve
the generated script's line breaks. With the default Readline behavior, press Tab
twice to list matching commands with their descriptions. Readline controls the
column layout, and long descriptions may be shortened to fit the terminal.

After upgrading `sing-box-panel`, run the `source <(...)` command again in each
active Bash session. If generated output is installed in a completion directory,
regenerate that file with the upgraded binary before starting a new shell.

### Zsh

```zsh
source <(sing-box-panel completion zsh)
```

### Fish

```fish
sing-box-panel completion fish | source
```

Generated output may instead be installed in the shell's normal completion
directory.

## Binary self-update

`update` replaces a release build with the newest published, non-prerelease
GitHub Release for the current architecture:

```sh
sing-box-panel update
```

Text mode reports version lookup, release signature verification, preparation,
download, binary verification, and installation on stderr. Interactive terminals
show a live download percentage bar and downloaded/total MiB; redirected output
uses occasional plain progress lines. Unknown download sizes show transferred
MiB until completion. The percentage measures the binary download only: a 100%
download still needs verification and installation. Success is reported on
stdout only after the update finishes; failures and cancellation end the progress
line before the error is printed. `--output=json` and `--output=jsonl` suppress
progress and preserve the existing structured result and error formats.

It is available only to strict v-prefixed release builds on Linux amd64 and
arm64. The selected release must attach both platform binaries, `SHA256SUMS`,
and `SHA256SUMS.sig`. The command verifies the embedded Ed25519 trust root and
the selected binary digest, then requires the staged file to carry the expected
Go command, module, Linux target, architecture, and disabled-CGO build identity.
Updates for one executable are serialized through an adjacent process lock;
the current executable digest is rechecked after waiting and immediately before
the atomic replacement, and the latest release tag is confirmed again while
the lock is held. Cancellation is honored until that replacement commit point.
Any missing or invalid evidence, concurrent replacement, changing release, or
prior cancellation leaves the running executable unchanged.

The invoking user must be able to write the executable's directory. Replacing
the file does not restart an already-running systemd service. See
[Release process](../development/release.md) for the signing and publication procedure.
