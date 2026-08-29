#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-or-later

set -euo pipefail

readonly installer_repository="rehuony/sing-box-panel"
readonly installer_release_origin="https://github.com/${installer_repository}"
readonly installer_public_key_url="https://raw.githubusercontent.com/${installer_repository}/main/.github/keypair/release-signing-public-key"
readonly installer_signature_domain="sing-box-panel release checksums v1"
readonly installer_ed25519_spki_prefix="MCowBQYDK2VwAyEA"

installer_requested_version=""
installer_show_help=false
installer_architecture=""
installer_binary_path=""
installer_settings_path=""
installer_default_data_dir=""
installer_service_scope=""
installer_expected_digest=""
installer_temporary_directory=""

installer_usage() {
  cat <<'EOF'
usage: installer.sh [--version vMAJOR.MINOR.PATCH]

Install a signed sing-box-panel Linux release for the current effective user.

options:
  --version VERSION  install one exact published release (for example v0.1.0)
  -h, --help         show this help

Without --version, the installer selects the latest stable GitHub Release.
An explicit version may identify a stable release or a GitHub pre-release.

The installer verifies the signed checksum manifest, installs the binary, and
initializes settings only when they do not already exist. It does not install,
start, stop, or restart a systemd service.
EOF
}

installer_error() {
  printf 'installer: %s\n' "$*" >&2
}

installer_parse_args() {
  installer_requested_version=""
  installer_show_help=false

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

installer_resolve_layout() {
  local effective_uid="$1"
  local home_directory="${2:-}"
  local config_home="${3:-}"
  local data_home="${4:-}"

  if [[ "${effective_uid}" == "0" ]]; then
    installer_binary_path="/usr/local/bin/sing-box-panel"
    installer_settings_path="/etc/sing-box-panel/setting.json"
    installer_default_data_dir="/var/lib/sing-box-panel"
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

  installer_binary_path="${home_directory}/.local/bin/sing-box-panel"
  installer_settings_path="${config_home}/sing-box-panel/setting.json"
  installer_default_data_dir="${data_home}/sing-box-panel"
  installer_service_scope="user"
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

installer_parse_checksums() {
  local checksums_path="$1"
  local target_asset="$2"
  local line
  local digest
  local asset_name
  local extra
  local amd64_digest=""
  local arm64_digest=""
  local entry_count=0

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
    case "${asset_name}" in
    sing-box-panel-linux-amd64)
      if [[ -n "${amd64_digest}" ]]; then
        installer_error "SHA256SUMS contains a duplicate amd64 entry"
        return 1
      fi
      amd64_digest="${digest}"
      ;;
    sing-box-panel-linux-arm64)
      if [[ -n "${arm64_digest}" ]]; then
        installer_error "SHA256SUMS contains a duplicate arm64 entry"
        return 1
      fi
      arm64_digest="${digest}"
      ;;
    *)
      installer_error "SHA256SUMS contains an unexpected asset: ${asset_name}"
      return 1
      ;;
    esac
    entry_count=$((entry_count + 1))
  done <"${checksums_path}"

  if ((entry_count != 2)) || [[ -z "${amd64_digest}" || -z "${arm64_digest}" ]]; then
    installer_error "SHA256SUMS must contain exactly the amd64 and arm64 release binaries"
    return 1
  fi
  case "${target_asset}" in
  sing-box-panel-linux-amd64) installer_expected_digest="${amd64_digest}" ;;
  sing-box-panel-linux-arm64) installer_expected_digest="${arm64_digest}" ;;
  *)
    installer_error "unsupported checksum target: ${target_asset}"
    return 1
    ;;
  esac
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
  local metadata

  if ! metadata="$("${binary_path}" --output json version)"; then
    installer_error "the verified release binary cannot run on this host"
    return 1
  fi
  if [[ "${metadata}" != *\"version\":\"${expected_version}\"* ]]; then
    installer_error "release binary metadata does not report ${expected_version}"
    return 1
  fi
}

installer_prepare_configuration() {
  local binary_path="$1"
  local settings_path="$2"

  if [[ -L "${settings_path}" ]]; then
    installer_error "refusing symbolic settings path: ${settings_path}"
    return 1
  fi
  if [[ -e "${settings_path}" ]]; then
    if [[ ! -f "${settings_path}" ]]; then
      installer_error "settings path is not a regular file: ${settings_path}"
      return 1
    fi
    printf 'Verifying existing settings at %s\n' "${settings_path}"
    "${binary_path}" verify --config "${settings_path}"
    return 0
  fi

  printf 'Initializing settings at %s\n' "${settings_path}"
  "${binary_path}" init --config "${settings_path}"
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
  if ! probe_path="$(mktemp "${directory}/.sing-box-panel.preflight.XXXXXX")"; then
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

  staging_path="$(mktemp "${directory}/.sing-box-panel.install.XXXXXX")"
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

installer_print_next_steps() {
  local binary_directory="${installer_binary_path%/*}"

  printf '\nInstalled sing-box-panel successfully.\n'
  printf '  Binary: %s\n' "${installer_binary_path}"
  printf '  Settings: %s\n' "${installer_settings_path}"
  printf '  Default data directory: %s\n' "${installer_default_data_dir}"
  printf '\nThe installer did not configure or start systemd. To install and start the service:\n  '
  printf '%q system install --scope=%s --now\n' "${installer_binary_path}" "${installer_service_scope}"
  printf 'After a later binary upgrade, restart an existing service explicitly:\n  '
  printf '%q system restart --scope=%s\n' "${installer_binary_path}" "${installer_service_scope}"

  if [[ "${installer_service_scope}" == "user" ]]; then
    case ":${PATH:-}:" in
    *":${binary_directory}:"*) ;;
    *) printf '\nAdd %s to PATH if your shell does not already include it.\n' "${binary_directory}" ;;
    esac
    printf 'A user service needs administrator-enabled lingering to continue after logout.\n'
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
  installer_resolve_layout "${EUID}" "${HOME:-}" "${XDG_CONFIG_HOME:-}" "${XDG_DATA_HOME:-}" || return

  if [[ -n "${installer_requested_version}" ]]; then
    release_version="${installer_requested_version}"
  else
    release_version="$(installer_resolve_latest_version)" || return
  fi
  release_asset="sing-box-panel-linux-${installer_architecture}"

  umask 077
  installer_temporary_directory="$(mktemp -d "${TMPDIR:-/tmp}/sing-box-panel-installer.XXXXXX")"
  trap installer_cleanup EXIT
  trap 'installer_cleanup; exit 130' HUP INT TERM
  checksums_path="${installer_temporary_directory}/SHA256SUMS"
  signature_path="${installer_temporary_directory}/SHA256SUMS.sig"
  release_binary="${installer_temporary_directory}/${release_asset}"
  raw_public_key_path="${installer_temporary_directory}/release-signing-public-key"
  public_key_path="${installer_temporary_directory}/release-public-key.pem"

  printf 'Downloading sing-box-panel %s for linux/%s\n' "${release_version}" "${installer_architecture}"
  installer_download_asset "${release_version}" "SHA256SUMS" "${checksums_path}"
  installer_download_asset "${release_version}" "SHA256SUMS.sig" "${signature_path}"
  installer_download_asset "${release_version}" "${release_asset}" "${release_binary}"
  installer_download_public_key "${raw_public_key_path}"
  installer_prepare_public_key "${raw_public_key_path}" "${public_key_path}"
  installer_verify_signature \
    "${release_version}" "${checksums_path}" "${signature_path}" \
    "${public_key_path}" "${installer_temporary_directory}"
  installer_parse_checksums "${checksums_path}" "${release_asset}"
  installer_verify_checksum "${release_binary}" "${installer_expected_digest}"
  chmod 0700 "${release_binary}"
  installer_verify_binary_version "${release_binary}" "${release_version}"
  installer_preflight_binary_destination "${installer_binary_path}"
  installer_prepare_configuration "${release_binary}" "${installer_settings_path}"
  installer_install_binary "${release_binary}" "${installer_binary_path}"
  installer_verify_binary_version "${installer_binary_path}" "${release_version}"
  installer_print_next_steps
}

if [[ "${BASH_SOURCE[0]:-$0}" == "$0" ]]; then
  installer_main "$@"
fi
