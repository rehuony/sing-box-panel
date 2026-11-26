#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-or-later

set -euo pipefail

# Every path below is read under this prefix; tests point it at a scratch tree,
# and on a host it is empty.
cleanup_root="${SING_BOX_CLEANUP_ROOT:-}"

cleanup_mode=check
cleanup_components=(panel node)
cleanup_keep_data=false
cleanup_assume_yes=false

# Each finding is one row across these arrays: its class (install, leftover
# or advice), its type, its target, a note, and for a unit its scope.
cleanup_classes=()
cleanup_types=()
cleanup_targets=()
cleanup_notes=()
cleanup_scopes=()
declare -A cleanup_seen=()
# Binaries and node data directories whose processes a purge stops.
declare -A cleanup_process_targets=()

cleanup_usage() {
  cat <<'EOF'
usage: cleanup.sh [check|legacy|purge] [--component panel|node] [--keep-data] [--yes]

Find and remove what sing-box-panel and sing-box-node left on this host,
including what earlier releases left behind:

  check   report the installation and its leftovers; change nothing (default)
  legacy  remove only what earlier releases and interrupted updates left
          behind, and drop settings fields the current release rejects
  purge   stop and remove the services, binaries, settings, data and service
          accounts, and every leftover

options:
  --component NAME  act on panel or node only; both by default
  --keep-data       with purge, keep settings, data and service accounts
  -y, --yes         do not ask before removing
  -h, --help        show this help

Run as root to cover the system services and every user's installation; a
regular user covers their own. Journal logs, shell profiles and reverse proxy
configuration are left alone.
EOF
}

cleanup_label() {
  local level="$1" color="$2"
  if [[ -t 2 && -z "${NO_COLOR:-}" && "${TERM:-}" != dumb ]]; then
    printf '\033[%sm%-7s\033[0m ' "${color}" "${level}" >&2
  else
    printf '%-7s ' "${level}" >&2
  fi
}

cleanup_log() {
  cleanup_label "$1" "$2"
  shift 2
  printf '%s\n' "$*" >&2
}

cleanup_error() { cleanup_log ERROR 31 "$*"; }
cleanup_warn() { cleanup_log WARN 33 "$*"; }
cleanup_info() { cleanup_log INFO 36 "$*"; }
cleanup_success() { cleanup_log OK 32 "$*"; }

cleanup_has_tty() { { : </dev/tty; } 2>/dev/null; }

cleanup_parse_args() {
  cleanup_mode=check
  cleanup_components=(panel node)
  cleanup_keep_data=false
  cleanup_assume_yes=false
  while [[ $# -gt 0 ]]; do
    case "$1" in
    check | legacy | purge) cleanup_mode="$1" ;;
    --component)
      if [[ $# -lt 2 || ( "$2" != panel && "$2" != node ) ]]; then
        cleanup_error "--component requires panel or node"
        return 2
      fi
      cleanup_components=("$2")
      shift
      ;;
    --keep-data) cleanup_keep_data=true ;;
    -y | --yes) cleanup_assume_yes=true ;;
    -h | --help) cleanup_mode=help ;;
    *)
      cleanup_error "unknown argument: $1"
      return 2
      ;;
    esac
    shift
  done
  if [[ "${cleanup_keep_data}" == true && "${cleanup_mode}" != purge ]]; then
    cleanup_error "--keep-data applies only to purge"
    return 2
  fi
}

# cleanup_add records a finding once per class and target.
cleanup_add() {
  local class="$1" type="$2" target="$3" note="$4" scope="${5:-}"
  local key="${class}|${target}"
  [[ -n "${cleanup_seen[${key}]:-}" ]] && return 0
  cleanup_seen["${key}"]=1
  cleanup_classes+=("${class}")
  cleanup_types+=("${type}")
  cleanup_targets+=("${target}")
  cleanup_notes+=("${note}")
  cleanup_scopes+=("${scope}")
}

cleanup_exists() { [[ -e "$1" || -L "$1" ]]; }

# cleanup_kind names what a path is, for the report.
cleanup_kind() { if [[ -d "$1" && ! -L "$1" ]]; then printf 'directory'; else printf 'file'; fi; }

cleanup_system_scope() { [[ -n "${cleanup_root}" || "${EUID}" == 0 ]]; }

# cleanup_homes prints "user<TAB>home" for every home to scan: each user's
# when run as root, otherwise the caller's own.
cleanup_homes() {
  local entry user home
  if [[ -n "${cleanup_root}" ]]; then
    for home in "${cleanup_root}/root" "${cleanup_root}"/home/*; do
      if [[ -d "${home}" ]]; then printf '%s\t%s\n' "${home##*/}" "${home}"; fi
    done
    return 0
  fi
  if [[ "${EUID}" != 0 ]]; then
    printf '%s\t%s\n' "$(id -un)" "${HOME}"
    return 0
  fi
  command -v getent >/dev/null 2>&1 || return 0
  while IFS= read -r entry; do
    IFS=: read -r user _ _ _ _ home _ <<<"${entry}"
    [[ -n "${home}" && "${home}" != / && -d "${home}" ]] && printf '%s\t%s\n' "${user}" "${home}"
  done < <(getent passwd | awk -F: '$3 == 0 || ($3 >= 1000 && $3 < 65534)')
}

# cleanup_settings_data_dir prints the data directory a settings file names,
# resolved beside the file when relative.
cleanup_settings_data_dir() {
  local settings="$1" value
  value="$(sed -n 's/.*"data_dir"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p' "${settings}" 2>/dev/null | head -n 1)"
  [[ -z "${value}" ]] && return 0
  [[ "${value}" == /* ]] || value="${settings%/*}/${value}"
  printf '%s\n' "${cleanup_root}${value#"${cleanup_root}"}"
}

# cleanup_owned_directory reports whether a data directory is the service's
# own: a default one by its name, a custom one by the files the service keeps
# there. A system directory or a home is never one.
cleanup_owned_directory() {
  local path="${1%/}" marker bare
  shift
  bare="${path#"${cleanup_root}"}"
  case "${bare}" in
  "" | / | /bin | /boot | /etc | /home | /lib | /media | /mnt | /opt | /root | /sbin | /srv | /tmp | /usr | /usr/local | /var | /var/lib) return 1 ;;
  esac
  [[ "${bare}" =~ ^/home/[^/]+$ || "${bare}" == "${HOME:-/root}" ]] && return 1
  [[ "${bare##*/}" == sing-box-panel || "${bare##*/}" == sing-box-node ]] && return 0
  for marker in "$@"; do
    [[ -e "${path}/${marker}" ]] && return 0
  done
  return 1
}

cleanup_scan_units() {
  local scope="$1" unit_dir="$2" name="$3"
  local unit="${unit_dir}/${name}.service" link helper
  if cleanup_exists "${unit}"; then
    cleanup_add install unit "${unit}" "${scope} service" "${scope}"
    if [[ "${scope}" == system && -f "${unit}" ]] && ! grep -q -- '--apply-staged' "${unit}"; then
      cleanup_add advice note "${unit}" "lacks the install step of one-way updates; with a current binary, rewrite it: ${name} systemd install --scope=system"
    fi
  fi
  [[ -d "${unit_dir}/${name}.service.d" ]] && cleanup_add install directory "${unit_dir}/${name}.service.d" "service drop-ins"
  for link in "${unit_dir}"/*.wants/"${name}.service"; do
    cleanup_exists "${link}" && cleanup_add install file "${link}" "service enablement link"
  done
  [[ "${scope}" == system ]] || return 0
  # Releases before one-way updates installed a root update helper beside the service.
  for helper in "${name}-update.path" "${name}-update.service"; do
    cleanup_exists "${unit_dir}/${helper}" && cleanup_add leftover unit "${unit_dir}/${helper}" "update helper of an earlier release" system
    for link in "${unit_dir}"/*.wants/"${helper}"; do
      cleanup_exists "${link}" && cleanup_add leftover file "${link}" "update helper enablement link"
    done
  done
  return 0
}

cleanup_scan_binary_dir() {
  local directory="$1" name="$2" entry
  cleanup_process_targets["${directory}/${name}"]=1
  [[ -f "${directory}/${name}" ]] && cleanup_add install file "${directory}/${name}" "binary"
  # Updates before one-way installs kept the replaced binary and a lock here;
  # interrupted updates and installs leave hidden temporary files.
  for entry in "${directory}/${name}.previous" "${directory}/.${name}.update.lock" \
    "${directory}/.${name}-update-"* "${directory}/.${name}-apply-"* "${directory}/.${name}-previous-"* \
    "${directory}/.${name}.install."* "${directory}/.${name}.preflight."*; do
    cleanup_exists "${entry}" && cleanup_add leftover "$(cleanup_kind "${entry}")" "${entry}" "left beside the binary by an earlier update or install"
  done
  return 0
}

cleanup_scan_settings() {
  local settings="$1" component="$2" fields
  [[ -f "${settings}" ]] || return 0
  cleanup_add install directory "${settings%/*}" "settings"
  [[ "${component}" == panel ]] || return 0
  if command -v python3 >/dev/null 2>&1; then
    fields="$(python3 - "${settings}" <<'PYEOF' 2>/dev/null || true
import json, sys
settings = json.load(open(sys.argv[1]))
for section, field in (("nodes", "endpoint"), ("nodes", "trust"), ("auth", "secure_cookie")):
    if isinstance(settings.get(section), dict) and field in settings[section]:
        print(f"{section}.{field}")
PYEOF
)"
  else
    fields="$(grep -oE '"(endpoint|trust|secure_cookie)"' "${settings}" | tr -d '"' | tr '\n' ' ' || true)"
  fi
  [[ -n "${fields}" ]] && cleanup_add leftover settings "${settings}" "fields the current release rejects: $(printf '%s' "${fields}" | tr '\n' ' ')"
  return 0
}

cleanup_scan_data_dir() {
  local data_dir="$1" component="$2" entry
  [[ -d "${data_dir}" ]] || return 0
  if [[ "${component}" == panel ]]; then
    cleanup_owned_directory "${data_dir}" panel.db logs.db process.lock || {
      cleanup_add advice note "${data_dir}" "does not look like a panel data directory; left alone"
      return 0
    }
    cleanup_add install directory "${data_dir}" "data"
    # The panel ran cores itself and served nodes over gRPC before.
    if [[ ! -e "${data_dir}/node.db" ]]; then
      for entry in node-tls.pem artifacts runtime logs panel-control.sock runtime-executor.lock; do
        cleanup_exists "${data_dir}/${entry}" && cleanup_add leftover "$(cleanup_kind "${data_dir}/${entry}")" "${data_dir}/${entry}" "unused since an earlier release"
      done
    fi
  else
    cleanup_owned_directory "${data_dir}" node.db enrollment.json || {
      cleanup_add advice note "${data_dir}" "does not look like a node data directory; left alone"
      return 0
    }
    cleanup_add install directory "${data_dir}" "data and installed cores"
    cleanup_process_targets["${data_dir}/"]=1
    if [[ -f "${data_dir}/enrollment.json" ]] && ! grep -q '"url"' "${data_dir}/enrollment.json"; then
      cleanup_add advice note "${data_dir}/enrollment.json" "enrolled with an earlier release; enroll the node again with a new code"
    fi
  fi
  if [[ -f "${data_dir}/updates/request" ]]; then
    cleanup_add advice note "${data_dir}/updates/request" "an update was never installed; rewrite the system unit, then restart the service"
  fi
  return 0
}

cleanup_scan_account() {
  local name="$1" entry shell
  command -v getent >/dev/null 2>&1 || return 0
  entry="$(getent passwd "${name}" 2>/dev/null)" || return 0
  shell="${entry##*:}"
  case "${shell}" in
  */nologin | */false) cleanup_add install account "${name}" "service account" ;;
  *) cleanup_add advice note "${name}" "account has a login shell, so it was not created by the installer; left alone" ;;
  esac
}

cleanup_scan_component() {
  local component="$1" name="sing-box-$1"
  local user home config_home data_home settings custom

  if cleanup_system_scope; then
    cleanup_scan_units system "${cleanup_root}/etc/systemd/system" "${name}"
    for settings in "${cleanup_root}/etc/sysusers.d/${name}.conf" "${cleanup_root}/etc/tmpfiles.d/${name}.conf"; do
      cleanup_exists "${settings}" && cleanup_add install file "${settings}" "service account and directory declaration"
    done
    for settings in "${cleanup_root}"/usr/lib/systemd/system/"${name}"{,-update}.{service,path}; do
      cleanup_exists "${settings}" && cleanup_add advice note "${settings}" "installed by a distribution package; remove the package instead"
    done
    cleanup_scan_binary_dir "${cleanup_root}/usr/local/bin" "${name}"
    settings="${cleanup_root}/etc/${name}/setting.json"
    cleanup_scan_settings "${settings}" "${component}"
    cleanup_scan_data_dir "${cleanup_root}/var/lib/${name}" "${component}"
    if [[ -f "${settings}" ]]; then
      custom="$(cleanup_settings_data_dir "${settings}")"
      [[ -n "${custom}" ]] && cleanup_scan_data_dir "${custom}" "${component}"
    fi
    cleanup_scan_account "${name}"
  fi

  while IFS=$'\t' read -r user home; do
    [[ -n "${home}" ]] || continue
    config_home="${home}/.config"
    data_home="${home}/.local/share"
    if [[ -z "${cleanup_root}" && "${home}" == "${HOME:-}" ]]; then
      config_home="${XDG_CONFIG_HOME:-${config_home}}"
      data_home="${XDG_DATA_HOME:-${data_home}}"
    fi
    cleanup_scan_units "user:${user}" "${config_home}/systemd/user" "${name}"
    cleanup_scan_binary_dir "${home}/.local/bin" "${name}"
    settings="${config_home}/${name}/setting.json"
    cleanup_scan_settings "${settings}" "${component}"
    cleanup_scan_data_dir "${data_home}/${name}" "${component}"
    if [[ -f "${settings}" ]]; then
      custom="$(cleanup_settings_data_dir "${settings}")"
      [[ -n "${custom}" ]] && cleanup_scan_data_dir "${custom}" "${component}"
    fi
  done < <(cleanup_homes)
  return 0
}

# cleanup_scan_processes finds running binaries of the scanned layouts and
# cores under node data directories, and services that still run a binary
# an update replaced.
cleanup_scan_processes() {
  local proc exe target pid
  [[ -z "${cleanup_root}" && -d /proc ]] || return 0
  for proc in /proc/[0-9]*; do
    exe="$(readlink "${proc}/exe" 2>/dev/null)" || continue
    pid="${proc##*/}"
    for target in "${!cleanup_process_targets[@]}"; do
      if [[ "${exe%" (deleted)"}" == "${target}" || ( "${target}" == */ && "${exe}" == "${target}"* ) ]]; then
        cleanup_add install process "${pid}" "${exe}"
        [[ "${exe}" == *" (deleted)" ]] && cleanup_add advice note "${pid}" "runs a binary an update replaced; restart its service"
      fi
    done
  done
  return 0
}

cleanup_scan() {
  local component
  for component in "${cleanup_components[@]}"; do
    cleanup_scan_component "${component}"
  done
  cleanup_scan_processes
}

# cleanup_selected reports whether finding index takes part in the mode.
cleanup_selected() {
  local class="${cleanup_classes[$1]}" type="${cleanup_types[$1]}" note="${cleanup_notes[$1]}"
  case "${cleanup_mode}" in
  legacy) [[ "${class}" == leftover ]] ;;
  purge)
    [[ "${class}" == advice ]] && return 1
    # Without --keep-data the settings go whole, so there are no fields to drop.
    [[ "${cleanup_keep_data}" == true ]] || [[ "${type}" != settings ]] || return 1
    [[ "${cleanup_keep_data}" == true ]] || return 0
    [[ "${type}" == account || "${note}" == settings || "${note}" == data* ]] && return 1
    return 0
    ;;
  *) return 1 ;;
  esac
}

# cleanup_report lists the findings by class, then what each mode would do.
cleanup_report() {
  local index class heading count
  local -A counts=([install]=0 [leftover]=0 [advice]=0)
  for class in install leftover advice; do
    case "${class}" in
    install) heading="Installed" ;;
    leftover) heading="Left behind by earlier releases or interrupted updates" ;;
    advice) heading="Needs attention" ;;
    esac
    for index in "${!cleanup_targets[@]}"; do
      [[ "${cleanup_classes[${index}]}" == "${class}" ]] || continue
      count=$((counts[${class}] + 1))
      counts["${class}"]="${count}"
      ((count == 1)) && printf '\n%s:\n' "${heading}"
      printf '  %-9s %s\n  %-9s %s\n' "${cleanup_types[${index}]}" \
        "${cleanup_targets[${index}]#"${cleanup_root}"}" "" "${cleanup_notes[${index}]}"
    done
  done
  if ((counts[install] + counts[leftover] + counts[advice] == 0)); then
    printf 'Nothing from sing-box-panel or sing-box-node was found.\n'
    return 0
  fi
  printf '\n%d installed, %d left behind, %d needing attention.\n' \
    "${counts[install]}" "${counts[leftover]}" "${counts[advice]}"
  [[ "${cleanup_mode}" == check ]] || return 0
  ((counts[leftover] > 0)) && printf 'Remove what was left behind: cleanup.sh legacy\n'
  ((counts[install] + counts[leftover] > 0)) && printf 'Remove everything: cleanup.sh purge\n'
  return 0
}

cleanup_confirm() {
  local count="$1" answer
  [[ "${cleanup_assume_yes}" == true ]] && return 0
  if ! cleanup_has_tty; then
    cleanup_error "rerun with --yes to remove without a terminal"
    return 1
  fi
  cleanup_label CONFIRM 33
  printf 'Remove the %d items listed above? [y/N] ' "${count}" >&2
  IFS= read -r answer </dev/tty || { printf '\n' >&2; return 1; }
  [[ "${answer}" == y || "${answer}" == Y || "${answer}" == yes ]]
}

# cleanup_systemctl runs systemctl in a unit's scope; another user's units go
# through that user's manager.
cleanup_systemctl() {
  local scope="$1"
  shift
  command -v systemctl >/dev/null 2>&1 || return 0
  case "${scope}" in
  system) systemctl "$@" ;;
  "user:$(id -un)") systemctl --user "$@" ;;
  user:*) systemctl --user --machine="${scope#user:}@" "$@" ;;
  esac
}

cleanup_stop_unit() {
  local scope="$1" unit="${2##*/}"
  cleanup_systemctl "${scope}" disable --now "${unit}" >/dev/null 2>&1 ||
    cleanup_warn "could not stop ${unit} through systemd; its processes are stopped directly"
}

cleanup_stop_process() {
  local pid="$1"
  kill -TERM "${pid}" 2>/dev/null || return 0
  for _ in 1 2 3 4 5 6 7 8 9 10; do
    kill -0 "${pid}" 2>/dev/null || return 0
    sleep 0.5
  done
  kill -KILL "${pid}" 2>/dev/null || true
}

cleanup_strip_settings_fields() {
  local settings="$1"
  local fields="nodes.endpoint, nodes.trust and auth.secure_cookie"
  if ! command -v python3 >/dev/null 2>&1; then
    cleanup_warn "python3 is missing; delete ${fields} from ${settings#"${cleanup_root}"} by hand"
    return 1
  fi
  # Rewrite in place, so the file keeps its owner and mode.
  python3 - "${settings}" <<'PYEOF' 2>/dev/null && return 0
import json, sys
path = sys.argv[1]
with open(path) as file:
    settings = json.load(file)
for section, field in (("nodes", "endpoint"), ("nodes", "trust"), ("auth", "secure_cookie")):
    if isinstance(settings.get(section), dict):
        settings[section].pop(field, None)
with open(path, "r+") as file:
    file.seek(0)
    json.dump(settings, file, indent=2)
    file.write("\n")
    file.truncate()
PYEOF
  cleanup_warn "could not read ${settings#"${cleanup_root}"}; delete ${fields} from it by hand"
  return 1
}

cleanup_remove_account() {
  local name="$1"
  if command -v userdel >/dev/null 2>&1; then
    userdel -- "${name}" 2>/dev/null || cleanup_warn "could not delete the account ${name}"
  fi
  if command -v getent >/dev/null 2>&1 && getent group "${name}" >/dev/null 2>&1 && command -v groupdel >/dev/null 2>&1; then
    groupdel -- "${name}" 2>/dev/null || cleanup_warn "could not delete the group ${name}"
  fi
}

cleanup_apply() {
  local index type target scope reload_system=false
  local -a selected=()
  local -A reload_users=()

  for index in "${!cleanup_targets[@]}"; do
    cleanup_selected "${index}" && selected+=("${index}")
  done
  if [[ ${#selected[@]} -eq 0 ]]; then
    cleanup_success "Nothing to remove."
    return 0
  fi
  cleanup_confirm "${#selected[@]}" || { cleanup_info "Nothing was changed."; return 0; }

  # Services stop first, then what still runs, so nothing holds the files.
  for index in "${selected[@]}"; do
    [[ "${cleanup_types[${index}]}" == unit ]] || continue
    scope="${cleanup_scopes[${index}]}"
    cleanup_stop_unit "${scope}" "${cleanup_targets[${index}]}"
    if [[ "${scope}" == system ]]; then reload_system=true; else reload_users["${scope}"]=1; fi
  done
  for index in "${selected[@]}"; do
    [[ "${cleanup_types[${index}]}" == process ]] && cleanup_stop_process "${cleanup_targets[${index}]}"
  done

  for index in "${selected[@]}"; do
    type="${cleanup_types[${index}]}"
    target="${cleanup_targets[${index}]}"
    case "${type}" in
    unit | file | directory)
      rm -rf -- "${target}"
      cleanup_info "Removed ${target#"${cleanup_root}"}"
      ;;
    settings)
      if cleanup_strip_settings_fields "${target}"; then
        cleanup_info "Dropped retired fields from ${target#"${cleanup_root}"}"
      fi
      ;;
    esac
  done

  for index in "${selected[@]}"; do
    [[ "${cleanup_types[${index}]}" == account ]] || continue
    cleanup_remove_account "${cleanup_targets[${index}]}"
    cleanup_info "Removed account ${cleanup_targets[${index}]}"
  done

  if [[ "${reload_system}" == true ]]; then
    cleanup_systemctl system daemon-reload >/dev/null 2>&1 || true
    cleanup_systemctl system reset-failed >/dev/null 2>&1 || true
  fi
  for scope in "${!reload_users[@]}"; do
    cleanup_systemctl "${scope}" daemon-reload >/dev/null 2>&1 || true
  done
  cleanup_success "Removed ${#selected[@]} items."
}

cleanup_main() {
  if ! cleanup_parse_args "$@"; then
    cleanup_usage >&2
    return 2
  fi
  if [[ "${cleanup_mode}" == help ]]; then
    cleanup_usage
    return 0
  fi
  if [[ "$(uname -s)" != Linux && -z "${cleanup_root}" ]]; then
    cleanup_error "only Linux hosts are supported"
    return 1
  fi
  if ! cleanup_system_scope; then
    cleanup_warn "not running as root: only $(id -un)'s installation is covered"
  fi
  cleanup_scan
  cleanup_report
  [[ "${cleanup_mode}" == check ]] && return 0
  cleanup_apply
}

if [[ "${BASH_SOURCE[0]:-$0}" == "$0" ]]; then
  cleanup_main "$@"
fi
