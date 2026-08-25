# CLI reference

The sing-box-panel CLI uses a Docker- and sing-box-style hierarchy. Running the
root command or any command group without a leaf prints help.

## Command hierarchy

```text
sing-box-panel
├─ init | verify | version
├─ server run
├─ core
│  ├─ catalog list | refresh
│  ├─ capability status | pack | refresh | inspect | upgrade | quarantine
│  ├─ list | show | install | import | remove | quarantine | revoke
│  └─ check | activate | rollback | status | start | stop | restart
├─ config show | get | set | unset | replace | export | import
│  ├─ render | validate | diff | apply
│  ├─ manual list | show | detach | preview | replace | discard | reattach
│  └─ revision list | show | diff | restore
├─ node list | show | create | update | delete | enable | disable | move | check | import
├─ rule list | show | create | update | delete | enable | disable | move
├─ subscription channel | source | token
├─ task list | show | wait | cancel
├─ log list | show | tail | clear | delete
├─ metrics show | watch
├─ traffic status | period
├─ system install | uninstall | status | start | stop | restart | logs
└─ completion bash | zsh | fish
```

Use `sing-box-panel COMMAND --help` at any level for current flags and leaf
commands.

## Global flags and output

- `-c, --config PATH` selects one settings file.
- `--output=text|json|jsonl` selects human or machine-readable output.

Results are written to stdout. Progress, warnings, and terminal errors are
written to stderr, allowing scripts to redirect them independently. JSON and
JSONL errors contain `code`, `message`, and `exit_code`; underlying causes are
not serialized because they may expose filesystem or upstream details.

Configuration values, manual JSON, capability generations, subscription
definitions and snapshots, and other bulk or secret-bearing values use
`--file PATH` or `--file -` for stdin. Do not place secrets in command
arguments. `config manual show` writes the exact raw configuration and must be
handled as secret-bearing output.

## Exact core-version resolution

When a version-scoped command documents `--core-version` as optional, omission
means: resolve the exact version once from the live, OS-verified sing-box
process. If no verified core is running, the command fails. It does not fall
back to the newest catalog release, a desired or applied bundle, or a version
edited previously.

An explicit exact version has the form `MAJOR.MINOR.PATCH`, without a leading
`v`, prerelease suffix, or build metadata. `core list` and
`core catalog list` are exceptions because `--core-version` is a filter;
omitting it returns all matching versions. Version-scoped HTTP operations use
the same live-identity rule unless their OpenAPI description defines the value
as a collection filter.

## Durable tasks and cancellation

Core download and verification, catalog refresh, configuration checks and
activation, and child-process control are durable tasks. Commands that create
them wait by default and may expose `--detach` to return the task immediately.

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
