#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-or-later

set -euo pipefail

script_dir="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd -P)"
# shellcheck disable=SC1091
source "${script_dir}/../cleanup.sh"

test_root="$(mktemp -d "${TMPDIR:-/tmp}/sing-box-panel-cleanup-test.XXXXXX")"
trap 'rm -rf -- "${test_root}"' EXIT HUP INT TERM
test_count=0

fail() {
  printf 'FAIL: %s\n' "$*" >&2
  exit 1
}

pass() { test_count=$((test_count + 1)); }

# The host commands the script calls record their arguments instead.
systemctl() { printf 'systemctl %s\n' "$*" >>"${test_root}/commands"; }
userdel() { printf 'userdel %s\n' "$*" >>"${test_root}/commands"; }
groupdel() { printf 'groupdel %s\n' "$*" >>"${test_root}/commands"; }
id() { printf 'alice\n'; }
getent() {
  case "$1 ${2:-}" in
  "passwd sing-box-panel") printf 'sing-box-panel:x:990:990::/var/lib/sing-box-panel:/usr/sbin/nologin\n' ;;
  "passwd sing-box-node") printf 'sing-box-node:x:991:991::/var/lib/sing-box-node:/bin/bash\n' ;;
  "group sing-box-panel") printf 'sing-box-panel:x:990:\n' ;;
  *) return 2 ;;
  esac
}
cleanup_has_tty() { return 1; }

# make_host writes a host that went through earlier releases: an old system
# unit with its update helper, files beside the binary, retired settings
# fields and data, a node enrolled over gRPC and a user-scope panel.
make_host() {
  local host="$1"
  rm -rf -- "${host}"
  mkdir -p "${host}"/etc/systemd/system/{multi-user.target.wants,paths.target.wants,sing-box-panel.service.d} \
    "${host}"/etc/{sysusers.d,tmpfiles.d,sing-box-panel,sing-box-node} "${host}"/usr/local/bin \
    "${host}"/var/lib/sing-box-panel/artifacts "${host}"/var/lib/sing-box-node "${host}"/srv/panel-data \
    "${host}"/home/alice/.local/bin "${host}"/home/alice/.config/{systemd/user,sing-box-panel} \
    "${host}"/home/alice/.local/share/sing-box-panel
  printf '[Service]\nExecStart=/usr/local/bin/sing-box-panel server start\n' >"${host}/etc/systemd/system/sing-box-panel.service"
  printf '[Service]\nExecStartPre=-+/usr/local/bin/sing-box-node update --apply-staged\n' >"${host}/etc/systemd/system/sing-box-node.service"
  printf '[Path]\n' >"${host}/etc/systemd/system/sing-box-panel-update.path"
  printf '[Service]\n' >"${host}/etc/systemd/system/sing-box-panel-update.service"
  ln -s ../sing-box-panel.service "${host}/etc/systemd/system/multi-user.target.wants/sing-box-panel.service"
  ln -s ../sing-box-panel-update.path "${host}/etc/systemd/system/paths.target.wants/sing-box-panel-update.path"
  printf '[Service]\n' >"${host}/etc/systemd/system/sing-box-panel.service.d/override.conf"
  touch "${host}"/etc/sysusers.d/sing-box-panel.conf "${host}"/etc/tmpfiles.d/sing-box-panel.conf \
    "${host}"/usr/local/bin/{sing-box-panel,sing-box-panel.previous,.sing-box-panel.update.lock,.sing-box-panel-update-1,sing-box-node} \
    "${host}"/var/lib/sing-box-panel/{panel.db,node-tls.pem} "${host}"/var/lib/sing-box-node/node.db \
    "${host}"/srv/panel-data/panel.db "${host}"/home/alice/.local/bin/sing-box-panel \
    "${host}"/home/alice/.config/systemd/user/sing-box-panel.service "${host}"/home/alice/.local/share/sing-box-panel/panel.db
  printf '{\n  "data_dir": "/srv/panel-data",\n  "nodes": {"endpoint": "127.0.0.1:9443", "trust": "pin", "runtime_history_retention_days": 90}\n}\n' \
    >"${host}/etc/sing-box-panel/setting.json"
  printf '{"data_dir": "/var/lib/sing-box-node"}\n' >"${host}/etc/sing-box-node/setting.json"
  printf '{"endpoint": "panel.example.com:443", "key": "k"}\n' >"${host}/var/lib/sing-box-node/enrollment.json"
  # A custom data directory that is a home is never removed.
  printf '{"data_dir": "/home/alice"}\n' >"${host}/home/alice/.config/sing-box-panel/setting.json"
}

run_cleanup() {
  local host="$1"
  shift
  : >"${test_root}/commands"
  # cleanup_root is the sourced script's global, read by cleanup_main.
  # shellcheck disable=SC2034
  (cleanup_root="${host}" && cleanup_main "$@") >"${test_root}/stdout" 2>"${test_root}/stderr"
}

tree_listing() { (cd "$1" && find . | LC_ALL=C sort); }

assert_present() { cleanup_exists "$1" || fail "$2 was removed: $1"; }
assert_absent() { ! cleanup_exists "$1" || fail "$2 was kept: $1"; }

test_check_reports_without_changes() {
  local host="${test_root}/check" before output
  make_host "${host}"
  before="$(tree_listing "${host}")"
  run_cleanup "${host}" check || fail "check failed"
  [[ "$(tree_listing "${host}")" == "${before}" ]] || fail "check changed the host"
  output="$(<"${test_root}/stdout")"
  for expected in /etc/systemd/system/sing-box-panel-update.path /usr/local/bin/sing-box-panel.previous \
    /var/lib/sing-box-panel/node-tls.pem "fields the current release rejects: nodes.endpoint nodes.trust" \
    "rewrite it: sing-box-panel systemd install --scope=system" "enroll the node again" \
    "does not look like a panel data directory" "Remove what was left behind: cleanup.sh legacy"; do
    [[ "${output}" == *"${expected}"* ]] || fail "check did not report: ${expected}"
  done
  [[ "${output}" != *"sing-box-node systemd install"* ]] || fail "a current unit was reported as outdated"
  [[ ! -s "${test_root}/commands" ]] || fail "check ran host commands"
  pass
}

test_legacy_keeps_the_installation() {
  local host="${test_root}/legacy"
  make_host "${host}"
  run_cleanup "${host}" legacy --yes || fail "legacy failed: $(<"${test_root}/stderr")"
  for path in etc/systemd/system/sing-box-panel-update.path etc/systemd/system/sing-box-panel-update.service \
    etc/systemd/system/paths.target.wants/sing-box-panel-update.path usr/local/bin/sing-box-panel.previous \
    usr/local/bin/.sing-box-panel.update.lock usr/local/bin/.sing-box-panel-update-1 \
    var/lib/sing-box-panel/node-tls.pem var/lib/sing-box-panel/artifacts; do
    assert_absent "${host}/${path}" "a leftover"
  done
  for path in etc/systemd/system/sing-box-panel.service usr/local/bin/sing-box-panel var/lib/sing-box-panel/panel.db \
    var/lib/sing-box-node/enrollment.json home/alice/.local/bin/sing-box-panel; do
    assert_present "${host}/${path}" "the installation"
  done
  grep -q 'systemctl disable --now sing-box-panel-update.path' "${test_root}/commands" || fail "the helper unit was not stopped"
  ! grep -q 'disable --now sing-box-panel.service' "${test_root}/commands" || fail "legacy stopped the service"
  if command -v python3 >/dev/null 2>&1; then
    ! grep -q '"endpoint"\|"trust"' "${host}/etc/sing-box-panel/setting.json" || fail "retired settings fields were kept"
    grep -q '"runtime_history_retention_days": 90' "${host}/etc/sing-box-panel/setting.json" || fail "a current field was dropped"
  fi
  pass
}

test_purge_one_component() {
  local host="${test_root}/node-only"
  make_host "${host}"
  run_cleanup "${host}" purge --component node --yes || fail "node purge failed: $(<"${test_root}/stderr")"
  for path in etc/systemd/system/sing-box-node.service usr/local/bin/sing-box-node etc/sing-box-node var/lib/sing-box-node; do
    assert_absent "${host}/${path}" "the node"
  done
  for path in etc/systemd/system/sing-box-panel.service usr/local/bin/sing-box-panel var/lib/sing-box-panel etc/sing-box-panel; do
    assert_present "${host}/${path}" "the panel"
  done
  # The node's account has a login shell, so it was not the installer's.
  ! grep -q userdel "${test_root}/commands" || fail "an account with a login shell was deleted"
  pass
}

test_purge_everything() {
  local host="${test_root}/purge"
  make_host "${host}"
  run_cleanup "${host}" purge --yes || fail "purge failed: $(<"${test_root}/stderr")"
  for path in etc/systemd/system/sing-box-panel.service etc/systemd/system/sing-box-panel.service.d \
    etc/systemd/system/multi-user.target.wants/sing-box-panel.service etc/systemd/system/sing-box-panel-update.path \
    etc/sysusers.d/sing-box-panel.conf etc/tmpfiles.d/sing-box-panel.conf usr/local/bin/sing-box-panel \
    usr/local/bin/sing-box-panel.previous etc/sing-box-panel var/lib/sing-box-panel srv/panel-data \
    etc/sing-box-node var/lib/sing-box-node home/alice/.local/bin/sing-box-panel \
    home/alice/.config/systemd/user/sing-box-panel.service home/alice/.config/sing-box-panel \
    home/alice/.local/share/sing-box-panel; do
    assert_absent "${host}/${path}" "an installed item"
  done
  assert_present "${host}/home/alice" "a home named as a data directory"
  grep -q 'systemctl disable --now sing-box-panel.service' "${test_root}/commands" || fail "the system service was not stopped"
  grep -q 'systemctl --user disable --now sing-box-panel.service' "${test_root}/commands" || fail "the user service was not stopped"
  grep -q 'systemctl daemon-reload' "${test_root}/commands" || fail "systemd was not reloaded"
  grep -q 'userdel -- sing-box-panel' "${test_root}/commands" || fail "the service account was kept"
  grep -q 'groupdel -- sing-box-panel' "${test_root}/commands" || fail "the service group was kept"
  pass
}

test_purge_keeping_data() {
  local host="${test_root}/keep"
  make_host "${host}"
  run_cleanup "${host}" purge --keep-data --yes || fail "purge --keep-data failed: $(<"${test_root}/stderr")"
  assert_absent "${host}/usr/local/bin/sing-box-panel" "the binary"
  assert_absent "${host}/etc/systemd/system/sing-box-panel.service" "the unit"
  for path in etc/sing-box-panel/setting.json var/lib/sing-box-panel/panel.db srv/panel-data/panel.db var/lib/sing-box-node/node.db; do
    assert_present "${host}/${path}" "kept data"
  done
  assert_absent "${host}/var/lib/sing-box-panel/node-tls.pem" "a leftover in kept data"
  ! grep -q userdel "${test_root}/commands" || fail "the account of kept data was deleted"
  pass
}

test_clean_host() (
  # The script finds no service account either.
  # shellcheck disable=SC2329
  getent() { return 2; }
  mkdir -p "${test_root}/empty"
  run_cleanup "${test_root}/empty" check || fail "check of a clean host failed"
  grep -q 'Nothing from sing-box-panel or sing-box-node was found' "${test_root}/stdout" || fail "a clean host was not reported"
  run_cleanup "${test_root}/empty" purge --yes || fail "purge of a clean host failed"
)

test_refusals() {
  local host="${test_root}/refuse" before
  make_host "${host}"
  before="$(tree_listing "${host}")"
  # Without a terminal, removing needs --yes.
  run_cleanup "${host}" purge || fail "a declined purge should exit successfully"
  [[ "$(tree_listing "${host}")" == "${before}" ]] || fail "a purge without --yes changed the host"
  ! run_cleanup "${host}" check --keep-data || fail "--keep-data was accepted outside purge"
  ! run_cleanup "${host}" --component all || fail "an unknown component was accepted"
  run_cleanup "${host}" --help || fail "help failed"
  grep -q 'usage: cleanup.sh' "${test_root}/stdout" || fail "help was not shown"
  pass
}

test_check_reports_without_changes
test_legacy_keeps_the_installation
test_purge_one_component
test_purge_everything
test_purge_keeping_data
test_clean_host
pass
test_refusals

printf 'cleanup tests passed (%d groups)\n' "${test_count}"
