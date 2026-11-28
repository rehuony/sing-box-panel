#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-or-later

set -euo pipefail

# The release's binaries, in the order the notes list them.
readonly notes_binaries=(
  sing-box-panel-linux-amd64
  sing-box-panel-linux-arm64
  sing-box-node-linux-amd64
  sing-box-node-linux-arm64
)

notes_usage() {
  cat <<'EOF'
usage: release-notes.sh --version vX.Y.Z --commit SHA --protocol MAJOR.MINOR
                        --repository OWNER/NAME DIRECTORY

Print the Markdown that opens a release's notes: its assets with their sizes
and SHA-256 digests, how to install or update it, and how to verify it by hand.
DIRECTORY holds the six release assets, with SHA256SUMS already written.
EOF
}

notes_error() {
  printf 'release-notes.sh: %s\n' "$1" >&2
  exit 2
}

# notes_describe prints what one asset is.
notes_describe() {
  case "$1" in
  sing-box-panel-linux-amd64) printf 'Panel service and CLI, Linux x86-64' ;;
  sing-box-panel-linux-arm64) printf 'Panel service and CLI, Linux ARM64' ;;
  sing-box-node-linux-amd64) printf 'Node service and CLI, Linux x86-64' ;;
  sing-box-node-linux-arm64) printf 'Node service and CLI, Linux ARM64' ;;
  SHA256SUMS) printf 'SHA-256 digests of the four binaries' ;;
  SHA256SUMS.sig) printf 'Ed25519 signature over the version and `SHA256SUMS`' ;;
  esac
}

# notes_size prints a file size in bytes as people read it: bytes below one
# KiB, otherwise MiB to one decimal.
notes_size() {
  local bytes
  bytes="$(wc -c <"$1" | tr -d '[:space:]')"
  if ((bytes < 1024)); then
    printf '%d B' "${bytes}"
  else
    awk -v bytes="${bytes}" 'BEGIN { printf "%.1f MiB", bytes / 1048576 }'
  fi
}

# notes_digest prints the SHA-256 that SHA256SUMS records for a binary.
notes_digest() {
  local digest name
  while read -r digest name; do
    if [[ "${name#\*}" == "$2" ]]; then
      printf '%s' "${digest}"
      return 0
    fi
  done <"$1"
  notes_error "SHA256SUMS has no digest for $2"
}

notes_main() {
  local version="" commit="" protocol="" repository="" directory=""
  while (($# > 0)); do
    case "$1" in
    --version | --commit | --protocol | --repository)
      (($# >= 2)) || notes_error "$1 needs a value"
      case "$1" in
      --version) version="$2" ;;
      --commit) commit="$2" ;;
      --protocol) protocol="$2" ;;
      --repository) repository="$2" ;;
      esac
      shift 2
      ;;
    -h | --help)
      notes_usage
      return 0
      ;;
    -*) notes_error "unknown option $1" ;;
    *)
      [[ -z "${directory}" ]] || notes_error "only one directory is accepted"
      directory="$1"
      shift
      ;;
    esac
  done
  [[ "${version}" =~ ^v[0-9]+\.[0-9]+\.[0-9]+$ ]] || notes_error "--version must be vMAJOR.MINOR.PATCH"
  [[ "${commit}" =~ ^[0-9a-f]{40}$ ]] || notes_error "--commit must be a full lowercase commit SHA"
  [[ "${protocol}" =~ ^[0-9]+\.[0-9]+$ ]] || notes_error "--protocol must be MAJOR.MINOR"
  [[ "${repository}" =~ ^[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+$ ]] || notes_error "--repository must be OWNER/NAME"
  [[ -n "${directory}" && -d "${directory}" ]] || notes_error "a release directory is required"
  local asset
  for asset in "${notes_binaries[@]}" SHA256SUMS SHA256SUMS.sig; do
    [[ -f "${directory}/${asset}" ]] || notes_error "${asset} is missing from ${directory}"
  done

  # Every row is worked out before anything is printed, so a release that
  # cannot be described prints nothing.
  local rows=() digest size
  for asset in "${notes_binaries[@]}"; do
    digest="$(notes_digest "${directory}/SHA256SUMS" "${asset}")"
    size="$(notes_size "${directory}/${asset}")"
    rows+=("$(printf '| `%s` | %s | %s | `%s` |' "${asset}" "$(notes_describe "${asset}")" "${size}" "${digest}")")
  done
  for asset in SHA256SUMS SHA256SUMS.sig; do
    size="$(notes_size "${directory}/${asset}")"
    rows+=("$(printf '| `%s` | %s | %s | — |' "${asset}" "$(notes_describe "${asset}")" "${size}")")
  done

  local source="https://github.com/${repository}"
  cat <<EOF
## Downloads

Each binary is a static Linux build of [\`${commit:0:12}\`](${source}/commit/${commit}).
It reports version \`${version}\` and speaks panel–node protocol ${protocol}.
A panel and its nodes run one release: an enrolled node updates to its panel's
release, which the panel serves it.

| File | What it is | Size | SHA-256 |
| --- | --- | ---: | --- |
EOF
  printf '%s\n' "${rows[@]}"
  cat <<EOF

## Install or update

Install the panel on its host first, then a node on each proxy host. The
installer verifies the signature and the binary's digest before it replaces
anything:

\`\`\`sh
curl -fsSL https://raw.githubusercontent.com/${repository}/main/scripts/installer.sh | sudo bash -s -- --component panel --version ${version}
curl -fsSL https://raw.githubusercontent.com/${repository}/main/scripts/installer.sh | sudo bash -s -- --component node --version ${version}
\`\`\`

To update an installation, update the panel from its settings or its host, then
each node from its own settings or host:

\`\`\`sh
sudo sing-box-panel update && sudo sing-box-panel systemd restart
sudo sing-box-node update --restart
\`\`\`

## Verify by hand

\`SHA256SUMS.sig\` signs the line \`sing-box-panel release checksums v1\`, the
version and \`SHA256SUMS\` with the key in
[\`.github/keypair/release-signing-public-key\`](${source}/blob/${version}/.github/keypair/release-signing-public-key).
Check the digests of what you downloaded, then the signature from a checkout of
\`${version}\`:

\`\`\`sh
sha256sum --check --ignore-missing SHA256SUMS
go tool sign-release verify --public-key .github/keypair/release-signing-public-key \\
  --version ${version} --checksums SHA256SUMS --signature SHA256SUMS.sig
\`\`\`
EOF
}

notes_main "$@"
