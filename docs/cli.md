# CLI reference

The sing-box-panel CLI manages one global canonical configuration, exact
sing-box artifacts, runtime state, subscriptions, and operational evidence.
Running the root command or a command group without a leaf prints help.

## Command hierarchy

```text
sing-box-panel
├─ init | verify | version | update
├─ server run
├─ core
│  ├─ catalog list | refresh
│  ├─ list | show | install | import | remove | quarantine | revoke
│  └─ check | activate | rollback | status | start | stop | restart
├─ config
│  ├─ show | get | set | unset | replace | export | import | validate
│  ├─ compile | apply | diff
│  └─ revision list | show | diff | restore
├─ subscription
│  ├─ channel list | show | create | update | delete | render
│  ├─ source list | show | create | update | refresh | delete
│  └─ token list | create | rotate | revoke
├─ task list | show | wait | cancel
├─ log list | show | tail | clear | delete
├─ metrics show | watch
├─ traffic status | period list | show
├─ system install | uninstall | status | start | stop | restart | logs
└─ completion bash | zsh | fish
```

Use `sing-box-panel COMMAND --help` at any level for current flags and leaf
commands. The HTTP/Web management surface additionally exposes subscription
user profiles, grant matrices, and source-version history.

## Global flags and output

- `-c, --config PATH` selects one settings file.
- `--output=text|json|jsonl` selects human or machine-readable output.

Results are written to stdout. Progress, warnings, and terminal errors are
written to stderr, allowing scripts to redirect them independently. JSON and
JSONL errors contain `code`, `message`, and `exit_code`; underlying causes are
not serialized because they may expose filesystem or upstream details.

Complete canonical documents, subscription source definitions, and other bulk
or secret-bearing values use `--file PATH` or `--file -` for stdin. Do not
place secrets in command arguments. Exported canonical configuration and
subscription source details may contain credentials and must be handled as
secret-bearing output.

## Exact artifact and adapter selection

Executable configuration is always derived from the single global canonical
revision. `config compile` requires an immutable installed artifact ID; the
panel resolves the complete verified binary profile and then selects one exact
compiled adapter. The Web and HTTP surfaces expose the same projection as a
non-persisting preview. No surface guesses from a version string, uses the
newest catalog release, or falls back to a nearby patch.

An artifact without a compiled adapter remains installable and inspectable,
but preview, compilation, check, Apply, Start, Restart, and Rollback fail
closed when they would depend on that artifact. If preview reports ignored
fields, compilation requires the exact current diagnostic digest:

```sh
sing-box-panel config compile \
  --artifact CORE_ARTIFACT_ID \
  --accept-ignored IGNORED_DIGEST
```

The ignored fields remain in the global revision and become effective again
when a selected adapter supports them.

## Durable tasks and cancellation

Core download and verification, catalog refresh, configuration checks and
activation, source refresh, and child-process control are durable tasks. Core,
catalog, configuration, and runtime commands wait by default and expose
`--detach` where applicable. `subscription source refresh` instead returns the
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

Completion generation opens no settings, database, or network connection.

```bash
source <(sing-box-panel completion bash)
```

```zsh
source <(sing-box-panel completion zsh)
```

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
the selected binary digest before an atomic replacement. Any missing or
invalid evidence leaves the running executable unchanged.

The invoking user must be able to write the executable's directory. Replacing
the file does not restart an already-running systemd service. See
[Release process](release.md) for the signing and publication procedure.
