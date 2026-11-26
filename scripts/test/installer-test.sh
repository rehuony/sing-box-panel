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
  installer_parse_args --component node
  assert_equal "node" "${installer_requested_component}" "parsed component"
  expect_failure installer_parse_args --version
  expect_failure installer_parse_args --unknown
  expect_failure installer_parse_args --component
  expect_failure installer_parse_args --component server

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

  installer_resolve_layout panel 0 /ignored /ignored /ignored
  assert_equal "/usr/local/bin/sing-box-panel" "${installer_binary_path}" "root binary"
  assert_equal "/etc/sing-box-panel/setting.json" "${installer_settings_path}" "root settings"
  assert_equal "/var/lib/sing-box-panel" "${installer_default_data_dir}" "root data"
  assert_equal "system" "${installer_service_scope}" "root service scope"

  installer_resolve_layout panel 1000 /home/panel "" ""
  assert_equal "/home/panel/.local/bin/sing-box-panel" "${installer_binary_path}" "user binary"
  assert_equal "/home/panel/.config/sing-box-panel/setting.json" "${installer_settings_path}" "user settings"
  assert_equal "/home/panel/.local/share/sing-box-panel" "${installer_default_data_dir}" "user data"
  assert_equal "user" "${installer_service_scope}" "user service scope"

  installer_resolve_layout panel 1000 /home/panel /srv/config /srv/data
  assert_equal "/srv/config/sing-box-panel/setting.json" "${installer_settings_path}" "XDG settings"
  assert_equal "/srv/data/sing-box-panel" "${installer_default_data_dir}" "XDG data"
  expect_failure installer_resolve_layout panel 1000 relative-home "" ""
  expect_failure installer_resolve_layout panel 1000 /home/panel relative-config /srv/data

  # The node has a settings file of its own, laid out like the panel's.
  installer_resolve_layout node 0 /ignored /ignored /ignored
  assert_equal "/usr/local/bin/sing-box-node" "${installer_binary_path}" "root node binary"
  assert_equal "/etc/sing-box-node/setting.json" "${installer_settings_path}" "root node settings"
  assert_equal "/var/lib/sing-box-node" "${installer_default_data_dir}" "root node data"
  installer_resolve_layout node 1000 /home/node "" /srv/data
  assert_equal "/home/node/.local/bin/sing-box-node" "${installer_binary_path}" "user node binary"
  assert_equal "/home/node/.config/sing-box-node/setting.json" "${installer_settings_path}" "user node settings"
  assert_equal "/srv/data/sing-box-node" "${installer_default_data_dir}" "user node data"
  pass
}

# shellcheck disable=SC2154
test_component_choice() (
  installer_requested_component=""
  installer_has_tty() { return 1; }
  expect_failure installer_choose_component

  # Without --component the installer asks until it gets a component.
  answers=(server 2)
  installer_has_tty() { return 0; }
  installer_read_tty() {
    printf -v "$1" '%s' "${answers[0]}"
    answers=("${answers[@]:1}")
  }
  installer_choose_component 2>/dev/null || fail "an answered choice failed"
  assert_equal "node" "${installer_component}" "chosen component"
  answers=("")
  installer_choose_component 2>/dev/null && fail "an empty answer chose a component"
  installer_requested_component=panel
  installer_has_tty() { return 1; }
  installer_choose_component || fail "--component needed a terminal"
  assert_equal "panel" "${installer_component}" "requested component"
)

# shellcheck disable=SC2154
test_signature_and_checksums() {
  local fixture_dir="${test_root}/signature"
  local amd64_path="${fixture_dir}/sing-box-panel-linux-amd64"
  local arm64_path="${fixture_dir}/sing-box-panel-linux-arm64"
  local node_path="${fixture_dir}/sing-box-node-linux-arm64"
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
  local node_digest
  local repository_public_key
  local embedded_public_key_der

  mkdir -p -- "${verify_dir}"
  printf 'amd64 release fixture\n' >"${amd64_path}"
  printf 'arm64 release fixture\n' >"${arm64_path}"
  printf 'node release fixture\n' >"${node_path}"
  amd64_digest="$(installer_sha256 "${amd64_path}")"
  arm64_digest="$(installer_sha256 "${arm64_path}")"
  node_digest="$(installer_sha256 "${node_path}")"
  printf '%s  %s\n%s  %s\n' \
    "${amd64_digest}" "${amd64_path##*/}" \
    "${arm64_digest}" "${arm64_path##*/}" >"${fixture_dir}/SHA256SUMS.panel-only"
  cp -- "${fixture_dir}/SHA256SUMS.panel-only" "${checksums_path}"
  printf '%s  %s\n%s  %s\n' \
    "${arm64_digest}" sing-box-node-linux-amd64 \
    "${node_digest}" "${node_path##*/}" >>"${checksums_path}"
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
  installer_parse_checksums "${checksums_path}" "${node_path##*/}"
  assert_equal "${node_digest}" "${installer_expected_digest}" "selected node checksum"
  # A release must publish both components.
  expect_failure installer_parse_checksums "${fixture_dir}/SHA256SUMS.panel-only" "${amd64_path##*/}"

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

# Exercise the real install flow with signed local assets. Only network, the
# destination layout and the service manager are replaced; settings and data
# are deliberately unusable.
test_binary_only_installation() (
  local fixture_dir="${test_root}/binary-only"
  local assets="${fixture_dir}/assets"
  local private_key="${test_root}/signature/private.pem"
  local output digest component architecture
  mkdir -p -- "${assets}" "${fixture_dir}/config" "${fixture_dir}/data"
  printf '{ invalid JSON\n' >"${fixture_dir}/config/setting.json"
  printf 'not a SQLite database\n' >"${fixture_dir}/data/panel.db"
  # shellcheck disable=SC2016
  printf '%s\n' '#!/usr/bin/env bash' \
    '[[ "$*" == "--output json version" ]] || exit 91' \
    'printf '\''{"version":"v1.2.3"}\n'\''' >"${assets}/sing-box-panel-linux-amd64"
  # shellcheck disable=SC2016
  printf '%s\n' '#!/usr/bin/env bash' \
    '[[ "$*" == "--output json version" ]] || exit 91' \
    'printf '\''{"version":"v1.2.3","protocol":"1.0"}\n'\''' >"${assets}/sing-box-node-linux-amd64"
  : >"${assets}/SHA256SUMS"
  for component in panel node; do
    cp -- "${assets}/sing-box-${component}-linux-amd64" "${assets}/sing-box-${component}-linux-arm64"
    for architecture in amd64 arm64; do
      digest="$(installer_sha256 "${assets}/sing-box-${component}-linux-${architecture}")"
      printf '%s  sing-box-%s-linux-%s\n' "${digest}" "${component}" "${architecture}" >>"${assets}/SHA256SUMS"
    done
  done
  printf '%s\nv1.2.3\n' "${installer_signature_domain}" >"${assets}/message"
  cat -- "${assets}/SHA256SUMS" >>"${assets}/message"
  openssl pkeyutl -sign -inkey "${private_key}" -rawin -in "${assets}/message" -out "${assets}/signature"
  openssl base64 -A -in "${assets}/signature" -out "${assets}/SHA256SUMS.sig"
  printf '\n' >>"${assets}/SHA256SUMS.sig"
  openssl pkey -in "${private_key}" -pubout -outform DER 2>/dev/null | tail -c 32 | openssl base64 -A >"${assets}/key"
  printf '\n' >>"${assets}/key"

  installer_resolve_architecture() { installer_architecture=amd64; }
  installer_resolve_layout() {
    installer_binary_name="sing-box-$1"
    installer_binary_path="${fixture_dir}/bin/sing-box-$1"
    installer_settings_path="${fixture_dir}/config/setting.json"
    installer_default_data_dir="${fixture_dir}/data"
    installer_service_scope=user
  }
  installer_service_state() { printf 'no user service\n'; }
  installer_download_asset() { cp -- "${assets}/$2" "$3"; }
  installer_download_public_key() { cp -- "${assets}/key" "$1"; }
  output="$(installer_main --component panel --version v1.2.3 2>&1)" || fail "binary-only install failed"
  [[ -x "${fixture_dir}/bin/sing-box-panel" ]] || fail "binary not installed"
  [[ ! -e "${fixture_dir}/bin/sing-box-node" ]] || fail "the node was installed with the panel"
  [[ "$(<"${fixture_dir}/config/setting.json")" == '{ invalid JSON' ]] || fail "settings changed"
  [[ "$(<"${fixture_dir}/data/panel.db")" == 'not a SQLite database' ]] || fail "data changed"
  for expected in "${fixture_dir}/bin/sing-box-panel" "${fixture_dir}/config/setting.json" "${fixture_dir}/data" 'eval "$(sing-box-panel completion zsh)"' 'INFO ' 'OK ' 'node   not installed'; do
    [[ "${output}" == *"${expected}"* ]] || fail "summary omitted ${expected}"
  done
  [[ "${output}" != *$'\033'* ]] || fail "redirected output contains ANSI codes"
  installer_main --component panel --yes --version v1.2.3 >/dev/null 2>&1 || fail "explicit replacement failed"

  # The installed panel is reported before the node is installed beside it.
  output="$(installer_main --component node --version v1.2.3 2>&1)" || fail "node install failed"
  [[ -x "${fixture_dir}/bin/sing-box-node" ]] || fail "node binary not installed"
  for expected in 'panel  v1.2.3' 'node   not installed' 'server start' 'enroll --code <code>' 'systemd install --scope=user --now' "${fixture_dir}/config/setting.json"; do
    [[ "${output}" == *"${expected}"* ]] || fail "node summary omitted ${expected}"
  done

  # Without a terminal the installer needs --component and changes nothing.
  installer_has_tty() { return 1; }
  fixture_dir="${test_root}/fresh-binary-only"
  expect_failure installer_main --version v1.2.3
  [[ ! -e "${fixture_dir}/bin" ]] || fail "an unchosen component was installed"
  # A missing configuration directory is also left uninitialized.
  installer_main --component panel --version v1.2.3 >/dev/null 2>&1 || fail "fresh install failed"
  [[ ! -e "${fixture_dir}/config" && ! -e "${fixture_dir}/data" ]] || fail "fresh install initialized data"
)

# Nodes up to v0.5.1 report their version with version --json.
test_installed_version_flags() (
  local fixture_dir="${test_root}/installed-version"
  mkdir -p -- "${fixture_dir}"
  # shellcheck disable=SC2016
  printf '%s\n' '#!/usr/bin/env bash' \
    '[[ "$*" == "version --json" ]] || exit 91' \
    'printf '\''{"version":"v0.5.1","protocol":"1.1"}\n'\''' >"${fixture_dir}/earlier"
  chmod 0755 "${fixture_dir}/earlier"
  assert_equal "v0.5.1" "$(installer_installed_version node "${fixture_dir}/earlier")" "earlier node version"
  expect_failure installer_installed_version panel "${fixture_dir}/earlier"
)

test_replacement_confirmation() (
  installer_binary_path="${test_root}/install/bin/sing-box-panel"
  installer_parse_args --yes
  installer_confirm_replacement || fail "--yes did not permit replacement"
  installer_assume_yes=false
  # Replace only the prompt decision to verify cancellation stops before download.
  installer_resolve_architecture() { installer_architecture=amd64; }
  installer_resolve_layout() { installer_binary_name="sing-box-$1"; }
  installer_inspect_installation() { :; }
  installer_service_state() { printf 'no user service\n'; }
  installer_confirm_replacement() { return 2; }
  installer_download_asset() { fail "canceled install downloaded assets"; }
  installer_resolve_latest_version() { fail "canceled install contacted GitHub"; }
  installer_main --component panel || fail "cancel should exit successfully"
  # Declining to choose a component cancels as well.
  installer_choose_component() { return 2; }
  installer_main || fail "a declined choice should exit successfully"
)

# Files an earlier release left beside the binary are reported after an install.
# The globals set here are the sourced installer's, read by installer_leftovers.
# shellcheck disable=SC2034
test_leftover_detection() {
  local directory="${test_root}/leftovers"
  mkdir -p "${directory}"
  installer_binary_name=sing-box-node
  installer_binary_path="${directory}/sing-box-node"
  installer_service_scope=user
  expect_failure installer_leftovers
  touch "${directory}/sing-box-node.previous"
  installer_leftovers || fail "a kept previous binary was not reported"
  pass
}

test_entrypoints
test_leftover_detection
test_arguments_and_versions
test_architectures_and_layouts
test_component_choice
pass
test_signature_and_checksums
test_atomic_binary_installation
test_binary_only_installation
pass
test_replacement_confirmation
test_installed_version_flags
pass

printf 'installer tests passed (%d groups)\n' "${test_count}"
