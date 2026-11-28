#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-or-later

set -euo pipefail

readonly installer_repository="rehuony/sing-box-panel"
readonly installer_release_origin="https://github.com/${installer_repository}"
readonly installer_public_key_url="https://raw.githubusercontent.com/${installer_repository}/main/.github/keypair/release-signing-public-key"
readonly installer_cleanup_url="https://raw.githubusercontent.com/${installer_repository}/main/scripts/cleanup.sh"
readonly installer_signature_domain="sing-box-panel release checksums v1"
readonly installer_ed25519_spki_prefix="MCowBQYDK2VwAyEA"

installer_requested_version=""
installer_requested_component=""
installer_component=""
installer_binary_name=""
installer_show_help=false
installer_assume_yes=false
installer_architecture=""
installer_binary_path=""
installer_settings_path=""
installer_default_data_dir=""
installer_service_scope=""
installer_expected_digest=""
installer_temporary_directory=""

installer_usage() {
  cat <<'EOF'
usage: installer.sh [--component panel|node] [--version vMAJOR.MINOR.PATCH] [--yes]

Install one component of a signed sing-box-panel Linux release for the
current effective user:

  panel  sing-box-panel, the management service and Web UI
  node   sing-box-node, the agent that runs sing-box on a proxy host

options:
  --component NAME   install panel or node; without it the installer asks
  --version VERSION  install one exact published release (for example v0.1.0)
  -y, --yes          replace an existing binary without prompting
  -h, --help         show this help

Without --version, the installer selects the latest stable GitHub Release.
An explicit version must name a stable release, such as v1.2.3.

The installer first reports which components are installed for this user and
the state of their systemd services. It verifies the signed checksum manifest,
installs the selected binary, and does not read, validate, or initialize
settings or data. It does not install, start, stop, or restart a systemd
service; the installed binary's systemd commands do that. scripts/cleanup.sh
finds what earlier releases left behind and removes an installation.
EOF
}

# installer_label prints a level label, colored on a terminal, padded so
# every message starts in the same column.
installer_label() {
  local level="$1" color="$2"
  if [[ -t 2 && -z "${NO_COLOR:-}" && "${TERM:-}" != dumb ]]; then
    printf '\033[%sm%-7s\033[0m ' "${color}" "${level}" >&2
  else
    printf '%-7s ' "${level}" >&2
  fi
}

# installer_log prints one message on stderr after its level label.
installer_log() {
  installer_label "$1" "$2"
  shift 2
  printf '%s\n' "$*" >&2
}

installer_error() { installer_log ERROR 31 "$*"; }
installer_warn() { installer_log WARN 33 "$*"; }
installer_info() { installer_log INFO 36 "$*"; }
installer_success() { installer_log OK 32 "$*"; }

# A curl | bash invocation reads the script from stdin. Prompts read the
# actual terminal instead; unattended installations pass their choices.
installer_has_tty() { { : </dev/tty; } 2>/dev/null; }

installer_read_tty() { IFS= read -r "$1" </dev/tty; }

# installer_ask prints a question and reads its answer on the same line.
# An interrupted read ends the line so later output starts on a new one.
installer_ask() {
  local question="$1"
  installer_label CONFIRM 33
  printf '%s ' "${question}" >&2
  installer_read_tty "$2" && return 0
  printf '\n' >&2
  return 1
}

installer_confirm_replacement() {
  local answer
  [[ ! -e "${installer_binary_path}" ]] && return 0
  [[ "${installer_assume_yes}" == true ]] && return 0
  if ! installer_has_tty; then
    installer_error "${installer_binary_path} already exists; rerun with --yes to replace it"
    return 1
  fi
  installer_ask "Replace ${installer_binary_path}? [y/N]" answer || return 1
  case "${answer}" in
    y | Y | yes | YES) return 0 ;;
    *) installer_info "Installation canceled; existing binary kept."; return 2 ;;
  esac
}

installer_validate_component() {
  [[ "$1" == panel || "$1" == node ]]
}

# installer_choose_component selects the requested component or asks for
# one. It never picks a default; without a terminal --component is required.
installer_choose_component() {
  local answer

  if [[ -n "${installer_requested_component}" ]]; then
    installer_component="${installer_requested_component}"
    return 0
  fi
  if ! installer_has_tty; then
    installer_error "choose the component to install with --component panel or --component node"
    return 1
  fi
  printf '%s\n' 'Select the component to install:' \
    '  1) panel  sing-box-panel, the management service and Web UI' \
    '  2) node   sing-box-node, the agent that runs sing-box on this host' >&2
  while true; do
    if ! installer_ask "Enter 1 or 2 (panel or node), or press Enter to cancel:" answer; then
      installer_info "Installation canceled."
      return 2
    fi
    case "${answer}" in
      1 | panel) installer_component=panel; return 0 ;;
      2 | node) installer_component=node; return 0 ;;
      "") installer_info "Installation canceled."; return 2 ;;
      *) installer_error "unknown choice: ${answer}" ;;
    esac
  done
}

installer_parse_args() {
  installer_requested_version=""
  installer_requested_component=""
  installer_show_help=false
  installer_assume_yes=false

  while [[ $# -gt 0 ]]; do
    case "$1" in
    --version)
      if [[ $# -lt 2 || -z "$2" ]]; then
        installer_error "--version requires a value"
        return 2
      fi
      installer_requested_version="$2"
      shift 2
      ;;
    --component)
      if [[ $# -lt 2 ]] || ! installer_validate_component "$2"; then
        installer_error "--component requires panel or node"
        return 2
      fi
      installer_requested_component="$2"
      shift 2
      ;;
    -y | --yes)
      installer_assume_yes=true
      shift
      ;;
    -h | --help)
      installer_show_help=true
      shift
      ;;
    *)
      installer_error "unknown argument: $1"
      return 2
      ;;
    esac
  done
}

installer_validate_version() {
  local version="$1"
  local stable_pattern='^v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$'

  [[ "${version}" =~ ${stable_pattern} ]]
}

installer_resolve_architecture() {
  local operating_system="$1"
  local machine="$2"

  if [[ "${operating_system}" != "Linux" ]]; then
    installer_error "only Linux is supported (detected ${operating_system})"
    return 1
  fi
  case "${machine}" in
  x86_64 | amd64)
    installer_architecture="amd64"
    ;;
  aarch64 | arm64)
    installer_architecture="arm64"
    ;;
  *)
    installer_error "unsupported Linux architecture: ${machine}"
    return 1
    ;;
  esac
}

installer_require_absolute_directory_base() {
  local name="$1"
  local value="$2"

  if [[ -z "${value}" || "${value}" != /* ]]; then
    installer_error "${name} must be an absolute path"
    return 1
  fi
}

# installer_resolve_layout selects the paths of component for the effective
# user: its binary, its settings file and its default data directory.
installer_resolve_layout() {
  local component="$1"
  local effective_uid="$2"
  local home_directory="${3:-}"
  local config_home="${4:-}"
  local data_home="${5:-}"

  installer_binary_name="sing-box-${component}"
  if [[ "${effective_uid}" == "0" ]]; then
    installer_binary_path="/usr/local/bin/${installer_binary_name}"
    installer_settings_path="/etc/${installer_binary_name}/setting.json"
    installer_default_data_dir="/var/lib/${installer_binary_name}"
    installer_service_scope="system"
    return 0
  fi

  installer_require_absolute_directory_base "HOME" "${home_directory}" || return
  if [[ -z "${config_home}" ]]; then
    config_home="${home_directory}/.config"
  else
    installer_require_absolute_directory_base "XDG_CONFIG_HOME" "${config_home}" || return
  fi
  if [[ -z "${data_home}" ]]; then
    data_home="${home_directory}/.local/share"
  else
    installer_require_absolute_directory_base "XDG_DATA_HOME" "${data_home}" || return
  fi

  installer_binary_path="${home_directory}/.local/bin/${installer_binary_name}"
  installer_settings_path="${config_home}/${installer_binary_name}/setting.json"
  installer_default_data_dir="${data_home}/${installer_binary_name}"
  installer_service_scope="user"
}

# installer_installed_version prints the version a component's binary
# reports, or fails when it reports none. Releases up to v0.5.5 select JSON
# with --output instead of --format, and nodes up to v0.5.1 take
# version --json.
installer_installed_version() {
  local component="$1"
  local binary_path="$2"
  local metadata

  if ! metadata="$("${binary_path}" --format json version 2>/dev/null)" &&
    ! metadata="$("${binary_path}" --output json version 2>/dev/null)"; then
    [[ "${component}" == node ]] || return 1
    metadata="$("${binary_path}" version --json 2>/dev/null)" || return 1
  fi
  [[ "${metadata}" =~ \"version\":\"([^\"]+)\" ]] || return 1
  printf '%s\n' "${BASH_REMATCH[1]}"
}

# installer_service_state describes a component's systemd unit in the
# effective user's scope.
installer_service_state() {
  local component="$1"
  local unit="sing-box-${component}.service"
  local scope_args=()
  local active enabled

  if ! command -v systemctl >/dev/null 2>&1; then
    printf 'systemd unavailable\n'
    return 0
  fi
  [[ "${installer_service_scope}" == user ]] && scope_args=(--user)
  active="$(systemctl "${scope_args[@]}" is-active "${unit}" 2>/dev/null)" || true
  enabled="$(systemctl "${scope_args[@]}" is-enabled "${unit}" 2>/dev/null)" || true
  if [[ -z "${enabled}" || "${enabled}" == not-found ]] && [[ "${active}" != active ]]; then
    printf 'no %s service\n' "${installer_service_scope}"
    return 0
  fi
  printf '%s service %s, %s\n' "${installer_service_scope}" "${active:-unknown}" "${enabled:-unknown}"
}

# installer_inspect_installation reports each component's installed binary
# and service for the effective user, without changing anything.
installer_inspect_installation() {
  local component version service

  printf 'Current installation:\n' >&2
  for component in panel node; do
    installer_resolve_layout "${component}" "${EUID}" "${HOME:-}" "${XDG_CONFIG_HOME:-}" "${XDG_DATA_HOME:-}" || return
    if [[ -f "${installer_binary_path}" && -x "${installer_binary_path}" ]]; then
      version="$(installer_installed_version "${component}" "${installer_binary_path}")" || version="unknown version"
    else
      version="not installed"
    fi
    service="$(installer_service_state "${component}")"
    printf '  %-6s %-15s %s (%s)\n' "${component}" "${version}" "${installer_binary_path}" "${service}" >&2
  done
}

installer_require_commands() {
  local command_name
  local missing=()

  for command_name in curl openssl mktemp mkdir cp chmod mv rm uname wc cat; do
    if ! command -v "${command_name}" >/dev/null 2>&1; then
      missing+=("${command_name}")
    fi
  done
  if [[ ${#missing[@]} -gt 0 ]]; then
    installer_error "required commands are missing: ${missing[*]}"
    return 1
  fi
}

installer_curl() {
  curl \
    --proto '=https' \
    --proto-redir '=https' \
    --tlsv1.2 \
    --fail \
    --silent \
    --show-error \
    --location \
    --retry 3 \
    --retry-delay 1 \
    --retry-connrefused \
    "$@"
}

installer_version_from_latest_url() {
  local latest_url="$1"
  local version

  case "${latest_url}" in
  "${installer_release_origin}/releases/tag/"*)
    version="${latest_url#"${installer_release_origin}/releases/tag/"}"
    ;;
  *)
    installer_error "GitHub returned an unexpected latest-release URL: ${latest_url}"
    return 1
    ;;
  esac
  if ! installer_validate_version "${version}"; then
    installer_error "the latest published release is not a stable vMAJOR.MINOR.PATCH version: ${version}"
    return 1
  fi
  printf '%s\n' "${version}"
}

installer_resolve_latest_version() {
  local latest_url

  if ! latest_url="$(installer_curl \
    --output /dev/null \
    --write-out '%{url_effective}' \
    "${installer_release_origin}/releases/latest")"; then
    installer_error "cannot resolve the latest published GitHub Release"
    return 1
  fi
  installer_version_from_latest_url "${latest_url}"
}

installer_download_asset() {
  local version="$1"
  local asset_name="$2"
  local destination="$3"
  local asset_url="${installer_release_origin}/releases/download/${version}/${asset_name}"

  if ! installer_curl --output "${destination}" "${asset_url}"; then
    installer_error "cannot download ${asset_name} for ${version}"
    return 1
  fi
  if [[ ! -f "${destination}" || -L "${destination}" ]]; then
    installer_error "downloaded asset is not a regular file: ${asset_name}"
    return 1
  fi
}

installer_download_public_key() {
  local destination="$1"

  if ! installer_curl --output "${destination}" "${installer_public_key_url}"; then
    installer_error "cannot download the repository release-signing public key"
    return 1
  fi
  if [[ ! -f "${destination}" || -L "${destination}" ]]; then
    installer_error "downloaded release-signing public key is not a regular file"
    return 1
  fi
}

installer_prepare_public_key() {
  local source="$1"
  local destination="$2"
  local source_size
  local encoded_key

  if [[ ! -f "${source}" || -L "${source}" ]]; then
    installer_error "release-signing public key must be a regular, non-symbolic file"
    return 1
  fi
  source_size="$(wc -c <"${source}")"
  encoded_key="$(<"${source}")"
  if [[ ! "${encoded_key}" =~ ^[A-Za-z0-9+/]{43}=$ ]] || \
    { ((source_size != 44)) && ((source_size != 45)); }; then
    installer_error "release-signing public key must contain one canonical Base64 Ed25519 key"
    return 1
  fi

  printf '%s\n%s%s\n%s\n' \
    '-----BEGIN PUBLIC KEY-----' \
    "${installer_ed25519_spki_prefix}" "${encoded_key}" \
    '-----END PUBLIC KEY-----' >"${destination}"
  chmod 0600 "${destination}"
  if ! openssl pkey -pubin -in "${destination}" -noout >/dev/null 2>&1; then
    installer_error "OpenSSL cannot load the repository Ed25519 release key"
    return 1
  fi
}

installer_verify_signature() {
  local version="$1"
  local checksums_path="$2"
  local signature_path="$3"
  local public_key_path="$4"
  local work_directory="$5"
  local signature_size
  local signature_text
  local signature_raw="${work_directory}/SHA256SUMS.signature"
  local message_path="${work_directory}/SHA256SUMS.message"

  signature_size="$(wc -c <"${signature_path}")"
  signature_text="$(<"${signature_path}")"
  if ((signature_size != 89)) || [[ ! "${signature_text}" =~ ^[A-Za-z0-9+/]{86}==$ ]]; then
    installer_error "SHA256SUMS.sig is not a canonical Ed25519 signature file"
    return 1
  fi
  if ! printf '%s' "${signature_text}" | openssl base64 -d -A -out "${signature_raw}"; then
    installer_error "cannot decode SHA256SUMS.sig"
    return 1
  fi
  if (($(wc -c <"${signature_raw}") != 64)); then
    installer_error "SHA256SUMS.sig does not contain a 64-byte Ed25519 signature"
    return 1
  fi

  printf '%s\n%s\n' "${installer_signature_domain}" "${version}" >"${message_path}"
  cat -- "${checksums_path}" >>"${message_path}"
  if ! openssl pkeyutl \
    -verify \
    -pubin \
    -inkey "${public_key_path}" \
    -rawin \
    -in "${message_path}" \
    -sigfile "${signature_raw}" >/dev/null 2>&1; then
    installer_error "release signature verification failed"
    return 1
  fi
}

# installer_release_assets lists the binaries every release publishes.
installer_release_assets=(
  sing-box-panel-linux-amd64 sing-box-panel-linux-arm64
  sing-box-node-linux-amd64 sing-box-node-linux-arm64
)

installer_parse_checksums() {
  local checksums_path="$1"
  local target_asset="$2"
  local line
  local digest
  local asset_name
  local extra
  local expected
  local known
  local entry_count=0
  local -A digests=()

  while IFS= read -r line || [[ -n "${line}" ]]; do
    if [[ -z "${line}" ]]; then
      installer_error "SHA256SUMS contains an empty entry"
      return 1
    fi
    digest=""
    asset_name=""
    extra=""
    read -r digest asset_name extra <<<"${line}"
    if [[ -n "${extra}" || ! "${digest}" =~ ^[0-9a-f]{64}$ || -z "${asset_name}" ]]; then
      installer_error "SHA256SUMS contains a malformed entry"
      return 1
    fi
    asset_name="${asset_name#\*}"
    asset_name="${asset_name#./}"
    known=false
    for expected in "${installer_release_assets[@]}"; do
      [[ "${asset_name}" == "${expected}" ]] && known=true
    done
    if [[ "${known}" != true ]]; then
      installer_error "SHA256SUMS contains an unexpected asset: ${asset_name}"
      return 1
    fi
    if [[ -n "${digests[${asset_name}]:-}" ]]; then
      installer_error "SHA256SUMS contains a duplicate entry for ${asset_name}"
      return 1
    fi
    digests["${asset_name}"]="${digest}"
    entry_count=$((entry_count + 1))
  done <"${checksums_path}"

  if ((entry_count != ${#installer_release_assets[@]})); then
    installer_error "SHA256SUMS must contain exactly the panel and node release binaries for amd64 and arm64"
    return 1
  fi
  if [[ -z "${digests[${target_asset}]:-}" ]]; then
    installer_error "unsupported checksum target: ${target_asset}"
    return 1
  fi
  installer_expected_digest="${digests[${target_asset}]}"
}

installer_sha256() {
  local path="$1"
  local digest

  digest="$(openssl dgst -sha256 -r "${path}")"
  digest="${digest%% *}"
  if [[ ! "${digest}" =~ ^[0-9a-f]{64}$ ]]; then
    installer_error "OpenSSL returned an invalid SHA-256 digest"
    return 1
  fi
  printf '%s\n' "${digest}"
}

installer_verify_checksum() {
  local path="$1"
  local expected="$2"
  local actual

  actual="$(installer_sha256 "${path}")" || return
  if [[ "${actual}" != "${expected}" ]]; then
    installer_error "release binary checksum verification failed"
    return 1
  fi
}

installer_verify_binary_version() {
  local binary_path="$1"
  local expected_version="$2"
  local version

  if ! version="$(installer_installed_version "${installer_component}" "${binary_path}")"; then
    installer_error "the verified release binary cannot run on this host"
    return 1
  fi
  if [[ "${version}" != "${expected_version}" ]]; then
    installer_error "release binary metadata does not report ${expected_version}"
    return 1
  fi
}

installer_ensure_binary_directory() {
  local directory="$1"
  local previous_umask

  if [[ -L "${directory}" ]]; then
    installer_error "refusing symbolic binary directory: ${directory}"
    return 1
  fi
  if [[ -e "${directory}" ]]; then
    if [[ ! -d "${directory}" ]]; then
      installer_error "binary directory is not a directory: ${directory}"
      return 1
    fi
    return 0
  fi

  previous_umask="$(umask)"
  umask 022
  if ! mkdir -p -- "${directory}"; then
    umask "${previous_umask}"
    return 1
  fi
  umask "${previous_umask}"
}

installer_validate_binary_destination() {
  local destination="$1"
  local directory="${destination%/*}"

  installer_ensure_binary_directory "${directory}" || return
  if [[ -L "${destination}" ]]; then
    installer_error "refusing symbolic binary destination: ${destination}"
    return 1
  fi
  if [[ -e "${destination}" && ! -f "${destination}" ]]; then
    installer_error "binary destination is not a regular file: ${destination}"
    return 1
  fi
}

installer_preflight_binary_destination() {
  local destination="$1"
  local directory="${destination%/*}"
  local probe_path

  installer_validate_binary_destination "${destination}" || return
  if ! probe_path="$(mktemp "${directory}/.${destination##*/}.preflight.XXXXXX")"; then
    installer_error "binary directory is not writable: ${directory}"
    return 1
  fi
  rm -f -- "${probe_path}"
}

installer_install_binary() {
  local source="$1"
  local destination="$2"
  local directory="${destination%/*}"
  local staging_path
  local source_digest
  local destination_digest

  installer_validate_binary_destination "${destination}" || return

  source_digest="$(installer_sha256 "${source}")" || return
  if [[ -f "${destination}" ]]; then
    destination_digest="$(installer_sha256 "${destination}")" || return
    if [[ "${source_digest}" == "${destination_digest}" ]]; then
      chmod 0755 "${destination}"
      return 0
    fi
  fi

  staging_path="$(mktemp "${directory}/.${destination##*/}.install.XXXXXX")"
  if ! cp -- "${source}" "${staging_path}"; then
    rm -f -- "${staging_path}"
    return 1
  fi
  chmod 0755 "${staging_path}"
  if [[ -L "${destination}" || ( -e "${destination}" && ! -f "${destination}" ) ]]; then
    rm -f -- "${staging_path}"
    installer_error "binary destination changed during installation: ${destination}"
    return 1
  fi
  if ! mv -f -- "${staging_path}" "${destination}"; then
    rm -f -- "${staging_path}"
    return 1
  fi
}

installer_cleanup() {
  if [[ -n "${installer_temporary_directory}" && -d "${installer_temporary_directory}" ]]; then
    rm -rf -- "${installer_temporary_directory}"
  fi
  installer_temporary_directory=""
}

# installer_leftovers reports whether earlier releases left files beside the
# binary or, in system scope, their root update helper units.
installer_leftovers() {
  local directory="${installer_binary_path%/*}" name="${installer_binary_name}" path

  for path in "${directory}/${name}.previous" "${directory}/.${name}.update.lock" \
    "/etc/systemd/system/${name}-update.path"; do
    [[ "${path}" == */"${name}-update.path" && "${installer_service_scope}" != system ]] && continue
    [[ -e "${path}" || -L "${path}" ]] && return 0
  done
  return 1
}

# installer_print_next_steps summarizes the installed binary and the
# commands that start it, in the aligned layout the binaries print.
installer_print_next_steps() {
  local version="$1"
  local service_installed="$2"
  local binary="${installer_binary_path}"
  local binary_directory="${binary%/*}"
  local scope="--scope=${installer_service_scope}"

  installer_success "Installed ${installer_binary_name} ${version}"
  printf '\n  %-14s  %s\n' Binary "${binary}"
  printf '  %-14s  %s (default)\n' Settings "${installer_settings_path}"
  printf '  %-14s  %s (default)\n' 'Data directory' "${installer_default_data_dir}"
  if [[ "${installer_component}" == panel ]]; then
    printf '\nStart the panel in this terminal (creates missing settings and data):\n'
    printf '  %q server start\n' "${binary}"
  else
    printf '\nStart the node and its console in this terminal (creates missing settings and prints the console password):\n'
    printf '  %q server start\n' "${binary}"
    printf '\nEnroll it from the console, or with the command its panel shows after adding it:\n'
    printf '  %q enroll --code <code>\n' "${binary}"
  fi
  if [[ "${service_installed}" == true ]]; then
    printf '\nThe %s service still runs the previous binary. Refresh its unit and restart it:\n' "${installer_service_scope}"
  else
    printf '\nTo run it as a systemd service instead (creates missing settings and service directories):\n'
    printf '  %q systemd install %s --now\n' "${binary}" "${scope}"
    printf '\nAfter a later binary upgrade, refresh the unit and restart the service:\n'
  fi
  printf '  %q systemd install %s\n' "${binary}" "${scope}"
  printf '  %q systemd restart %s\n' "${binary}" "${scope}"
  printf '\nEnable shell completion:\n'
  printf '  %-5s %s\n' 'bash:' "source <(${installer_binary_name} completion bash)" \
    'fish:' "${installer_binary_name} completion fish | source" \
    'zsh:' "eval \"\$(${installer_binary_name} completion zsh)\""

  if installer_leftovers; then
    printf '\n' >&2
    installer_warn "Earlier releases left files behind; list and remove them with:"
    printf '  curl -fsSL %s | %sbash -s -- legacy\n' "${installer_cleanup_url}" \
      "$([[ "${installer_service_scope}" == system ]] && printf 'sudo ')" >&2
  fi
  if [[ "${installer_service_scope}" == "user" ]]; then
    printf '\n' >&2
    case ":${PATH:-}:" in
    *":${binary_directory}:"*) ;;
    *) installer_warn "${binary_directory} is not on PATH; add it if your shell does not include it." ;;
    esac
    installer_info "A user service keeps running after logout only with lingering enabled by an administrator."
  fi
}

installer_main() {
  local release_version
  local release_asset
  local checksums_path
  local signature_path
  local release_binary
  local raw_public_key_path
  local public_key_path
  local confirmation_status
  local choice_status
  local service_installed=false

  if ! installer_parse_args "$@"; then
    installer_usage >&2
    return 2
  fi
  if [[ "${installer_show_help}" == true ]]; then
    installer_usage
    return 0
  fi
  if [[ -n "${installer_requested_version}" ]] && ! installer_validate_version "${installer_requested_version}"; then
    installer_error "--version must use vMAJOR.MINOR.PATCH syntax"
    return 2
  fi

  installer_require_commands || return
  installer_resolve_architecture "$(uname -s)" "$(uname -m)" || return
  installer_inspect_installation || return
  if installer_choose_component; then
    :
  else
    choice_status=$?
    [[ "${choice_status}" == 2 ]] && return 0
    return "${choice_status}"
  fi
  installer_resolve_layout "${installer_component}" "${EUID}" "${HOME:-}" "${XDG_CONFIG_HOME:-}" "${XDG_DATA_HOME:-}" || return
  case "$(installer_service_state "${installer_component}")" in
  "systemd unavailable" | "no "*" service") ;;
  *) service_installed=true ;;
  esac
  installer_preflight_binary_destination "${installer_binary_path}" || return
  if installer_confirm_replacement; then
    :
  else
    confirmation_status=$?
    [[ "${confirmation_status}" == 2 ]] && return 0
    return "${confirmation_status}"
  fi

  if [[ -n "${installer_requested_version}" ]]; then
    release_version="${installer_requested_version}"
  else
    release_version="$(installer_resolve_latest_version)" || return
  fi
  release_asset="${installer_binary_name}-linux-${installer_architecture}"

  umask 077
  installer_temporary_directory="$(mktemp -d "${TMPDIR:-/tmp}/sing-box-panel-installer.XXXXXX")"
  trap installer_cleanup EXIT
  trap 'installer_cleanup; exit 130' HUP INT TERM
  checksums_path="${installer_temporary_directory}/SHA256SUMS"
  signature_path="${installer_temporary_directory}/SHA256SUMS.sig"
  release_binary="${installer_temporary_directory}/${release_asset}"
  raw_public_key_path="${installer_temporary_directory}/release-signing-public-key"
  public_key_path="${installer_temporary_directory}/release-public-key.pem"

  installer_info "Downloading ${installer_binary_name} ${release_version} for linux/${installer_architecture}"
  installer_download_asset "${release_version}" "SHA256SUMS" "${checksums_path}"
  installer_download_asset "${release_version}" "SHA256SUMS.sig" "${signature_path}"
  installer_download_asset "${release_version}" "${release_asset}" "${release_binary}"
  installer_download_public_key "${raw_public_key_path}"
  installer_prepare_public_key "${raw_public_key_path}" "${public_key_path}"
  installer_info "Verifying release signature and checksums"
  installer_verify_signature \
    "${release_version}" "${checksums_path}" "${signature_path}" \
    "${public_key_path}" "${installer_temporary_directory}"
  installer_parse_checksums "${checksums_path}" "${release_asset}"
  installer_verify_checksum "${release_binary}" "${installer_expected_digest}"
  chmod 0700 "${release_binary}"
  installer_verify_binary_version "${release_binary}" "${release_version}"
  installer_info "Installing binary at ${installer_binary_path}"
  installer_install_binary "${release_binary}" "${installer_binary_path}"
  installer_verify_binary_version "${installer_binary_path}" "${release_version}"
  installer_print_next_steps "${release_version}" "${service_installed}"
}

if [[ "${BASH_SOURCE[0]:-$0}" == "$0" ]]; then
  installer_main "$@"
fi
