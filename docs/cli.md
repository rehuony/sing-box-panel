# CLI reference

The sing-box-panel CLI manages the one saved sing-box configuration file that
it shares with the Web UI, exact sing-box artifacts, runtime state,
subscriptions, and operational evidence. Running the root command or a command
group without a leaf prints help.

## Command hierarchy

```text
sing-box-panel
├─ init | verify | version | update
├─ server start | stop | status
├─ system files | prune
├─ systemd install | uninstall | status | start | stop | restart | logs
├─ core
│  ├─ catalog | refresh
│  ├─ list | show | install | import | remove | quarantine | revoke
│  └─ enable | status | start | stop | restart | rollback
├─ config
│  ├─ show | export | import | validate
│  ├─ get | set | unset
│  └─ check | apply
├─ channel list | show | create | update | delete | render
├─ source list | show | create | update | refresh | delete
├─ token list | create | rotate | revoke
├─ task list | show | wait | cancel
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
remains `system files`; cleanup is now `system prune` (formerly `system clean`).
Update scripts and regenerate shell completions; the old service paths and
`system clean` are not aliases, and there is no `prn` alias.

Use `sing-box-panel COMMAND --help` at any level for current flags and leaf
commands. Help lists usage, local flags, inherited global flags (when present),
and available commands, in that order. Root usage is shown on one line as
`sing-box-panel [flags] [command]`; command groups similarly combine flags and
subcommands. Global flags may appear before or after subcommands, and invoking
a group without a subcommand still displays help. Leaf usage retains its
command-specific arguments. The HTTP/Web management surface
additionally exposes subscription user profiles, grant matrices, and
source-version history.

## Global flags and output

- `-c, --config PATH` selects one settings file.
- `--output=text|json|jsonl` selects human or machine-readable output.

`-c` and `--config` are short and long forms of the same flag. When omitted,
commands that need settings load the default path: root uses
`/etc/sing-box-panel/setting.json`; ordinary users use
`$XDG_CONFIG_HOME/sing-box-panel/setting.json`, or
`~/.config/sing-box-panel/setting.json` when XDG is unset. An explicit path
overrides that default; a missing or invalid selected file is an error rather
than silently loading another file. Repeated flags use the last supplied value.

Selecting a settings path does not load it. Help (including bare command groups),
`version`, shell completion, `update`, and `config validate --file` do not read
panel settings or open its database. `systemd uninstall`, `start`, `stop`,
`restart`, and `logs` operate on the selected service scope without loading the
CLI settings file. `systemd status` also works with unavailable settings; its
optional location report marks unreadable files or invalid `data_dir` fields as
unavailable without hiding systemd's status.

`system files`, `system prune`, `server status/stop`, `systemd install` without
`--now`, and local database operations read only `data_dir` to locate the
instance. They reject missing, empty, wrongly typed, or ambiguous paths and
malformed JSON; unrelated runtime fields such as `traffic.sample_retention_days`
do not block them. Relative data paths resolve against the settings file.
File ownership, database identity, symlink, locking, and cleanup-scope checks
still apply. Metrics read and validate `traffic.quota_gib` only when needed,
with persisted panel preferences taking precedence over the bootstrap value.

`verify`, `server start`, and `systemd install --now` require the complete valid
runtime configuration. Starting an existing unit through `systemd start/restart`
delegates to systemd; the service validates its own configured file on startup.
`init` creates settings explicitly and refuses to overwrite an existing file
unless `--force` is supplied. No command silently repairs a damaged file.

Results are written to stdout. Progress, warnings, and terminal errors are
written to stderr, allowing scripts to redirect them independently. JSON and
JSONL errors contain `code`, `message`, and `exit_code`; underlying causes are
not serialized because they may expose filesystem or upstream details.

`version` prints only the program name and version, such as `sing-box-panel
v1.2.3`. Development builds, including Go module pseudo-versions, display
`sing-box-panel dev`. Use `version --output=json` or `--output=jsonl` for the
full, unchanged `version`, `commit`, and `date` metadata.

Complete sing-box configuration documents, subscription source definitions, and other bulk
or secret-bearing values use `--file PATH` or `--file -` for stdin. Do not
place secrets in command arguments. Exported configuration and
subscription source details may contain credentials and must be handled as
secret-bearing output.

## The saved configuration file

`config show`, `config export`, and `config import` operate on the exact text
of the one saved configuration, the same file the Web editor saves through
`GET/PUT /api/v1/config/file`. Its logical name is `config.json`; the bytes are
stored in the `configuration_file` table of `panel.db` inside `data_dir`, not
at a separate filesystem path. `show` and `export` return the stored text
byte-for-byte, including whitespace, large numbers, and unfinished JSON.
`import` uses the numeric file revision as its compare-and-swap base:

```sh
sing-box-panel config show --output json        # revision, syntax_valid, canonical_revision_id
sing-box-panel config import --file ./config.json --revision 0   # first save
sing-box-panel config import --file ./config.json --revision 7   # later save
```

An import that is not valid JSON is still stored as a draft. It reports
`syntax_valid: false`, clears `canonical_revision_id`, and blocks `check`,
`apply`, `start`, and `restart` until the text is corrected; older valid
content is never substituted silently.

`config get` reads one JSON-pointer value of the current **valid** file.
`config set` and `unset` edit a value and require `--base-revision`, the `canonical_revision_id`
shown by `config show --output json`. This ID identifies the immutable valid
snapshot, whereas `--revision` on `import` is the numeric file revision that
also counts invalid drafts. Field edits refuse
to run while the saved file is invalid, and their output reports their own
canonical revision. Read a fresh `config show --output json` result before a
subsequent whole-file import to obtain its numeric revision and current text.
These revision values prevent concurrent edits from overwriting each other;
they do not expose a configuration history workflow. The CLI edits one saved
file and has no configuration history, historical diff, or restore commands.
Internal immutable records remain for validation, runtime identity, and
activation recovery.

`config validate --file` checks a local file without saving anything.

## Exact core selection

Executable configuration is always the current valid saved file. `config
check` and `config apply` accept `--core CORE_ARTIFACT_ID`, an immutable
installed and verified artifact, and default to the currently applied core.
Before any core has been applied, `--core` is required; no surface guesses
from a version string, uses the newest catalog release, or falls back to a
nearby patch. Switching cores never merges, fills, migrates, or rewrites the
saved JSON: the selected binary must accept an execution snapshot of that
configuration with `sing-box check`. Snapshot formatting may differ from the
saved text; field names and values are preserved, and the saved text is untouched.

A missing JSON Schema disables only structured editing. Raw JSON check, apply,
enable, start, restart, and rollback remain available, with the selected
binary's `sing-box check` as the final gate:

```sh
sing-box-panel config check                       # applied core
sing-box-panel config check --core CORE_ARTIFACT_ID
sing-box-panel config apply                       # checked restart with the applied core
sing-box-panel config apply --core CORE_ARTIFACT_ID
sing-box-panel core enable CORE_ARTIFACT_ID       # same as apply --core
```

`check` snapshots the valid file and runs the check as a durable maintenance
task without touching the live core. `apply` and `core enable` snapshot the
file for preflight in the serialized runtime lane and replace the running
process only after that check succeeds; a failed preflight leaves the live
core and the saved file unchanged.

The CLI does not expose the internal startup-artifact and activation-bundle
steps; the HTTP API still exposes them for the Web UI. The monitoring tier is
not a CLI input either: a checked restart derives the evidence tier from the
saved configuration itself, `limited` when the file already exposes a usable
Clash API and otherwise `process_only`; nothing is injected into the file to
create that endpoint. Rollback uses the previous immutable bundle and its own
configuration and binary evidence.

## Durable tasks and cancellation

Core download and verification, catalog refresh, configuration checks,
checked restarts, source refresh, and child-process control are durable tasks.
Core, catalog, configuration, and runtime commands wait by default and expose
`--detach` where applicable. `source refresh` instead returns the
queued task immediately, because that command has no local waiting mode.

```sh
sing-box-panel task list --lane runtime
sing-box-panel task show TASK_ID
sing-box-panel task wait TASK_ID
sing-box-panel task cancel TASK_ID
```

Canceling queued work is immediate. Canceling a running task requests
cancellation at its next safe boundary. Interrupting a local wait also attempts
to record a cancellation request for that durable task before exiting.

## Foreground panel control

```sh
sing-box-panel server start --config ./setting.json
# In another terminal, using the same settings/data directory:
sing-box-panel server status --config ./setting.json
sing-box-panel server stop --config ./setting.json --timeout 30s
```

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
requires the complete valid settings. Keep that bootstrap path unchanged while
running. The private
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
sing-box-panel system files                       # uses the default settings
sing-box-panel system files -c ./setting.json --output json
sing-box-panel system prune -c ./setting.json      # preview only
sing-box-panel system prune -c ./setting.json --yes
```

`files` shows the executable, selected settings, database, core logs, installed
core files, runtime configuration, uploads, process-control socket and lock.
It lists actual files beneath the managed directories and labels missing,
unrecognized, linked, and retained entries. The database contains the saved
configuration and panel logs; a filename for each of those is not invented.
No settings or database is initialized or migrated by this inspection.
An incomplete WAL without its shared-memory file is reported as unverified;
inspection does not repair database recovery state or create missing sidecars.
`--scope auto|user|system` selects which systemd installation files to inspect.

`prune` without `--yes` is a read-only preview. With `--yes`, it permanently
removes this instance's settings, database, configuration, logs, installed
cores, uploads and runtime files. It uninstalls a matching managed systemd
service, or asks a manually started instance to shut down, then acquires the
runtime and database-directory locks before deletion. Active CLI database
owners prevent cleanup. Ambiguous/changed service settings, an unverified
database, linked managed roots, and unsafe directory selections are refused.

The executable, original archives imported from outside managed directories,
unrecognized entries alongside managed data, other instances, OS accounts and
system journal records are retained. Files placed inside the reserved
`artifacts`, `runtime`, `imports`, or `logs/core` directories belong to the
cleanup scope. Symlinks inside those directories are removed as links; their
targets are not followed. Shared parent directories are never recursively
deleted. The data directory is removed only when empty; the conventional
`sing-box-panel` settings directory is also removed only when empty.

Cleanup reports completed removals if a filesystem failure interrupts it;
the service may already be stopped or uninstalled when a later lock or file
operation fails. It is not a reversible operation. A new instance can be created with `init`
after successful cleanup. Inspect and prune apply to the selected settings
and service scope, not every instance that may exist on the host.

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
can stop at their defined boundaries. A task that reached a terminal failed,
canceled, or superseded state is reported as a command failure rather than as
a successful wait.

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
[Release process](release.md) for the signing and publication procedure.
