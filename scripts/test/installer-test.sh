#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-or-later

set -euo pipefail

script_dir="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd -P)"
workspace_root="$(cd -- "${script_dir}/../.." && pwd -P)"
# shellcheck disable=SC1091
source "${script_dir}/../installer.sh"

test_root="$(mktemp -d "${TMPDIR:-/tmp}/sing-box-panel-installer-test.XXXXXX")"
trap 'rm -rf -- "${test_root}"' EXIT HUP INT TERM
test_count=0

fail() {
  printf 'FAIL: %s\n' "$*" >&2
  exit 1
}

pass() {
  test_count=$((test_count + 1))
}

assert_equal() {
  local expected="$1"
  local actual="$2"
  local label="$3"

  if [[ "${actual}" != "${expected}" ]]; then
    printf 'FAIL: %s: expected %q, got %q\n' "${label}" "${expected}" "${actual}" >&2
    exit 1
  fi
}

expect_failure() {
  if "$@" >/dev/null 2>&1; then
    fail "command unexpectedly succeeded: $*"
  fi
}

test_entrypoints() {
  local direct_output
  local stdin_output

  direct_output="$(bash "${script_dir}/../installer.sh" --help)" || fail "direct --help entrypoint failed"
  stdin_output="$(bash -s -- --help <"${script_dir}/../installer.sh")" || fail "stdin --help entrypoint failed"
  [[ "${direct_output}" == *"usage: installer.sh"* ]] || fail "direct --help omitted usage"
  [[ "${stdin_output}" == *"usage: installer.sh"* ]] || fail "stdin --help omitted usage"
  pass
}

# Values asserted below are globals assigned by the sourced installer.
# shellcheck disable=SC2154
test_arguments_and_versions() {
  installer_parse_args --version v1.2.3
  assert_equal "v1.2.3" "${installer_requested_version}" "parsed version"
  [[ "${installer_show_help}" == false ]] || fail "version request showed help"
  installer_parse_args --help
  [[ "${installer_show_help}" == true ]] || fail "help request was not recorded"
  expect_failure installer_parse_args --version
  expect_failure installer_parse_args --unknown

  installer_validate_version v0.1.0 || fail "stable version was rejected"
  installer_validate_version v10.20.30 || fail "multi-digit stable version was rejected"
  for invalid in 1.2.3 v01.2.3 v1.02.3 v1.2.03 v1.2 v1.2.3-rc.1 v1.2.3+meta dev; do
    expect_failure installer_validate_version "${invalid}"
  done
  assert_equal \
    "v1.2.3" \
    "$(installer_version_from_latest_url "${installer_release_origin}/releases/tag/v1.2.3")" \
    "latest release redirect"
  expect_failure installer_version_from_latest_url "${installer_release_origin}/releases/tag/v1.2.3-rc.1"
  expect_failure installer_version_from_latest_url "https://example.com/releases/tag/v1.2.3"
  pass
}

# shellcheck disable=SC2154
test_architectures_and_layouts() {
  installer_resolve_architecture Linux x86_64
  assert_equal "amd64" "${installer_architecture}" "x86_64 mapping"
  installer_resolve_architecture Linux aarch64
  assert_equal "arm64" "${installer_architecture}" "aarch64 mapping"
  expect_failure installer_resolve_architecture Darwin arm64
  expect_failure installer_resolve_architecture Linux riscv64

  installer_resolve_layout 0 /ignored /ignored /ignored
  assert_equal "/usr/local/bin/sing-box-panel" "${installer_binary_path}" "root binary"
  assert_equal "/etc/sing-box-panel/setting.json" "${installer_settings_path}" "root settings"
  assert_equal "/var/lib/sing-box-panel" "${installer_default_data_dir}" "root data"
  assert_equal "system" "${installer_service_scope}" "root service scope"

  installer_resolve_layout 1000 /home/panel "" ""
  assert_equal "/home/panel/.local/bin/sing-box-panel" "${installer_binary_path}" "user binary"
  assert_equal "/home/panel/.config/sing-box-panel/setting.json" "${installer_settings_path}" "user settings"
  assert_equal "/home/panel/.local/share/sing-box-panel" "${installer_default_data_dir}" "user data"
  assert_equal "user" "${installer_service_scope}" "user service scope"

  installer_resolve_layout 1000 /home/panel /srv/config /srv/data
  assert_equal "/srv/config/sing-box-panel/setting.json" "${installer_settings_path}" "XDG settings"
  assert_equal "/srv/data/sing-box-panel" "${installer_default_data_dir}" "XDG data"
  expect_failure installer_resolve_layout 1000 relative-home "" ""
  expect_failure installer_resolve_layout 1000 /home/panel relative-config /srv/data
  pass
}

# shellcheck disable=SC2154
test_signature_and_checksums() {
  local fixture_dir="${test_root}/signature"
  local amd64_path="${fixture_dir}/sing-box-panel-linux-amd64"
  local arm64_path="${fixture_dir}/sing-box-panel-linux-arm64"
  local checksums_path="${fixture_dir}/SHA256SUMS"
  local signed_checksums_path="${fixture_dir}/SHA256SUMS.signed"
  local message_path="${fixture_dir}/message"
  local signature_raw="${fixture_dir}/signature.raw"
  local signature_path="${fixture_dir}/SHA256SUMS.sig"
  local private_key="${fixture_dir}/private.pem"
  local public_key="${fixture_dir}/public.pem"
  local verify_dir="${fixture_dir}/verify"
  local amd64_digest
  local arm64_digest
  local repository_public_key
  local embedded_public_key_der

  mkdir -p -- "${verify_dir}"
  printf 'amd64 release fixture\n' >"${amd64_path}"
  printf 'arm64 release fixture\n' >"${arm64_path}"
  amd64_digest="$(installer_sha256 "${amd64_path}")"
  arm64_digest="$(installer_sha256 "${arm64_path}")"
  printf '%s  %s\n%s  %s\n' \
    "${amd64_digest}" "${amd64_path##*/}" \
    "${arm64_digest}" "${arm64_path##*/}" >"${checksums_path}"
  cp -- "${checksums_path}" "${signed_checksums_path}"

  openssl genpkey -algorithm ED25519 -out "${private_key}" >/dev/null 2>&1
  openssl pkey -in "${private_key}" -pubout -out "${public_key}" >/dev/null 2>&1
  printf '%s\n%s\n' "${installer_signature_domain}" "v1.2.3" >"${message_path}"
  cat -- "${checksums_path}" >>"${message_path}"
  openssl pkeyutl -sign -inkey "${private_key}" -rawin -in "${message_path}" -out "${signature_raw}"
  openssl base64 -A -in "${signature_raw}" -out "${signature_path}"
  printf '\n' >>"${signature_path}"

  installer_verify_signature v1.2.3 "${checksums_path}" "${signature_path}" "${public_key}" "${verify_dir}"
  installer_parse_checksums "${checksums_path}" "${amd64_path##*/}"
  assert_equal "${amd64_digest}" "${installer_expected_digest}" "selected checksum"
  installer_verify_checksum "${amd64_path}" "${installer_expected_digest}"

  printf 'tampered\n' >>"${checksums_path}"
  expect_failure installer_verify_signature v1.2.3 "${checksums_path}" "${signature_path}" "${public_key}" "${verify_dir}"
  expect_failure installer_parse_checksums "${checksums_path}" "${amd64_path##*/}"
  expect_failure installer_verify_signature v1.2.4 "${signed_checksums_path}" "${signature_path}" "${public_key}" "${verify_dir}"
  printf 'tampered binary\n' >>"${amd64_path}"
  expect_failure installer_verify_checksum "${amd64_path}" "${amd64_digest}"

  installer_prepare_public_key \
    "${workspace_root}/.github/keypair/release-signing-public-key" \
    "${fixture_dir}/production-public.pem"
  openssl pkey -pubin -in "${fixture_dir}/production-public.pem" -noout >/dev/null
  repository_public_key="$(<"${workspace_root}/.github/keypair/release-signing-public-key")"
  embedded_public_key_der="$(openssl pkey \
    -pubin \
    -in "${fixture_dir}/production-public.pem" \
    -outform DER | openssl base64 -A)"
  assert_equal \
    "MCowBQYDK2VwAyEA${repository_public_key}" \
    "${embedded_public_key_der}" \
    "embedded release public key"
  printf 'not-a-public-key\n' >"${fixture_dir}/invalid-public-key"
  expect_failure installer_prepare_public_key \
    "${fixture_dir}/invalid-public-key" \
    "${fixture_dir}/invalid-public.pem"
  pass
}

test_atomic_binary_installation() {
  local fixture_dir="${test_root}/install"
  local source_path="${fixture_dir}/source"
  local destination_path="${fixture_dir}/bin/sing-box-panel"
  local symlink_path="${fixture_dir}/bin/symlink-panel"

  mkdir -p -- "${fixture_dir}"
  printf 'first binary\n' >"${source_path}"
  installer_install_binary "${source_path}" "${destination_path}"
  cmp -- "${source_path}" "${destination_path}" >/dev/null || fail "installed binary differs from source"
  [[ -x "${destination_path}" ]] || fail "installed binary is not executable"

  installer_install_binary "${source_path}" "${destination_path}"
  cmp -- "${source_path}" "${destination_path}" >/dev/null || fail "idempotent install changed content"
  printf 'second binary\n' >"${source_path}"
  installer_install_binary "${source_path}" "${destination_path}"
  cmp -- "${source_path}" "${destination_path}" >/dev/null || fail "upgrade did not replace binary"

  ln -s -- "${destination_path}" "${symlink_path}"
  expect_failure installer_install_binary "${source_path}" "${symlink_path}"
  pass
}

# Exercise the real install flow with signed local assets. Only network and the
# destination layout are replaced; settings and data are deliberately unusable.
test_binary_only_installation() (
  local fixture_dir="${test_root}/binary-only"
  local assets="${fixture_dir}/assets"
  local private_key="${test_root}/signature/private.pem"
  local output digest
  mkdir -p -- "${assets}" "${fixture_dir}/config" "${fixture_dir}/data"
  printf '{ invalid JSON\n' >"${fixture_dir}/config/setting.json"
  printf 'not a SQLite database\n' >"${fixture_dir}/data/panel.db"
  # shellcheck disable=SC2016
  printf '%s\n' '#!/usr/bin/env bash' \
    '[[ "$*" == "--output json version" ]] || exit 91' \
    'printf '\''{"version":"v1.2.3"}\n'\''' >"${assets}/sing-box-panel-linux-amd64"
  cp -- "${assets}/sing-box-panel-linux-amd64" "${assets}/sing-box-panel-linux-arm64"
  digest="$(installer_sha256 "${assets}/sing-box-panel-linux-amd64")"
  printf '%s  sing-box-panel-linux-amd64\n%s  sing-box-panel-linux-arm64\n' "${digest}" "${digest}" >"${assets}/SHA256SUMS"
  printf '%s\nv1.2.3\n' "${installer_signature_domain}" >"${assets}/message"
  cat -- "${assets}/SHA256SUMS" >>"${assets}/message"
  openssl pkeyutl -sign -inkey "${private_key}" -rawin -in "${assets}/message" -out "${assets}/signature"
  openssl base64 -A -in "${assets}/signature" -out "${assets}/SHA256SUMS.sig"
  printf '\n' >>"${assets}/SHA256SUMS.sig"
  openssl pkey -in "${private_key}" -pubout -outform DER 2>/dev/null | tail -c 32 | openssl base64 -A >"${assets}/key"
  printf '\n' >>"${assets}/key"

  installer_resolve_architecture() { installer_architecture=amd64; }
  installer_resolve_layout() {
    installer_binary_path="${fixture_dir}/bin/sing-box-panel"
    installer_settings_path="${fixture_dir}/config/setting.json"
    installer_default_data_dir="${fixture_dir}/data"
    installer_service_scope=user
  }
  installer_download_asset() { cp -- "${assets}/$2" "$3"; }
  installer_download_public_key() { cp -- "${assets}/key" "$1"; }
  output="$(installer_main --version v1.2.3 2>&1)" || fail "binary-only install failed"
  [[ -x "${fixture_dir}/bin/sing-box-panel" ]] || fail "binary not installed"
  [[ "$(<"${fixture_dir}/config/setting.json")" == '{ invalid JSON' ]] || fail "settings changed"
  [[ "$(<"${fixture_dir}/data/panel.db")" == 'not a SQLite database' ]] || fail "data changed"
  for expected in "${fixture_dir}/bin/sing-box-panel" "${fixture_dir}/config/setting.json" "${fixture_dir}/data" 'eval "$(sing-box-panel completion zsh)"' 'INFO ' 'OK '; do
    [[ "${output}" == *"${expected}"* ]] || fail "summary omitted ${expected}"
  done
  [[ "${output}" != *$'\033'* ]] || fail "redirected output contains ANSI codes"
  installer_main --yes --version v1.2.3 >/dev/null 2>&1 || fail "explicit replacement failed"

  # A missing configuration directory is also left uninitialized.
  fixture_dir="${test_root}/fresh-binary-only"
  installer_main --version v1.2.3 >/dev/null 2>&1 || fail "fresh install failed"
  [[ ! -e "${fixture_dir}/config" && ! -e "${fixture_dir}/data" ]] || fail "fresh install initialized data"
)

test_replacement_confirmation() (
  installer_binary_path="${test_root}/install/bin/sing-box-panel"
  installer_parse_args --yes
  installer_confirm_replacement || fail "--yes did not permit replacement"
  installer_assume_yes=false
  # Replace only the prompt decision to verify cancellation stops before download.
  installer_resolve_architecture() { installer_architecture=amd64; }
  installer_resolve_layout() { :; }
  installer_confirm_replacement() { return 2; }
  installer_download_asset() { fail "canceled install downloaded assets"; }
  installer_resolve_latest_version() { fail "canceled install contacted GitHub"; }
  installer_main || fail "cancel should exit successfully"
)

test_entrypoints
test_arguments_and_versions
test_architectures_and_layouts
test_signature_and_checksums
test_atomic_binary_installation
test_binary_only_installation
pass
test_replacement_confirmation
pass

printf 'installer tests passed (%d groups)\n' "${test_count}"
