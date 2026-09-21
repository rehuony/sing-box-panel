#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-or-later
# assert_json receives literal jq programs whose $variables are bound by jq.
# shellcheck disable=SC2016

set -Eeuo pipefail

smoke_phase="validate inputs"
trap 'printf "release smoke failed: phase=%s line=%s status=%s\n" "${smoke_phase}" "${LINENO}" "$?" >&2' ERR

phase() {
  smoke_phase="$1"
  printf '[release smoke] %s\n' "${smoke_phase}"
}

assert_json() {
  local description="$1"
  local payload
  shift
  payload="$(cat)"
  if ! jq -e "$@" <<<"${payload}" >/dev/null; then
    printf 'assertion failed: %s\n' "${description}" >&2
    # Report only metadata; configuration text and settings can contain secrets.
    jq -c '{version, commit, panel_version, status, sequence, schema_version, revision, syntax_valid, canonical_revision_id, updated, previous_version, code}' \
      <<<"${payload}" >&2 || true
    return 1
  fi
}

usage() {
  cat >&2 <<'EOF'
usage: scripts/test/smoke-release.sh \
  --release-dir DIRECTORY \
  --version vX.Y.Z \
  --source-commit COMMIT \
  --architecture amd64|arm64 \
  --public-key FILE
EOF
}

usage_error() {
  printf '%s\n' "$1" >&2
  usage
  exit 2
}

release_dir=""
release_version=""
source_commit=""
architecture=""
public_key_path=""

while [[ $# -gt 0 ]]; do
  case "$1" in
  --release-dir)
    [[ $# -ge 2 ]] || usage_error "--release-dir requires a value"
    release_dir="$2"
    shift 2
    ;;
  --version)
    [[ $# -ge 2 ]] || usage_error "--version requires a value"
    release_version="$2"
    shift 2
    ;;
  --source-commit)
    [[ $# -ge 2 ]] || usage_error "--source-commit requires a value"
    source_commit="$2"
    shift 2
    ;;
  --architecture)
    [[ $# -ge 2 ]] || usage_error "--architecture requires a value"
    architecture="$2"
    shift 2
    ;;
  --public-key)
    [[ $# -ge 2 ]] || usage_error "--public-key requires a value"
    public_key_path="$2"
    shift 2
    ;;
  -h | --help)
    usage
    exit 0
    ;;
  *) usage_error "unknown argument: $1" ;;
  esac
done

[[ -n "${release_dir}" ]] || usage_error "--release-dir is required"
[[ -n "${release_version}" ]] || usage_error "--version is required"
[[ -n "${source_commit}" ]] || usage_error "--source-commit is required"
[[ -n "${architecture}" ]] || usage_error "--architecture is required"
[[ -n "${public_key_path}" ]] || usage_error "--public-key is required"

for required_command in curl go jq python3 sha256sum; do
  command -v "${required_command}" >/dev/null 2>&1 || {
    printf '%s is required for release smoke tests\n' "${required_command}" >&2
    exit 1
  }
done

if [[ "$(uname -s)" != Linux || "${EUID}" -eq 0 ]]; then
  printf 'release smoke tests require native Linux and a non-root user (isolated XDG data)\n' >&2
  exit 1
fi

script_dir="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd -P)"
workspace_root="$(cd -- "${script_dir}/../.." && pwd -P)"
git_root="$(git -C "${workspace_root}" rev-parse --show-toplevel)"
git_root="$(cd -- "${git_root}" && pwd -P)"
if [[ "${git_root}" != "${workspace_root}" ]]; then
  printf 'smoke tests must run from the repository root\n' >&2
  exit 1
fi

if [[ ! "${source_commit}" =~ ^[0-9a-f]{40}$ && ! "${source_commit}" =~ ^[0-9a-f]{64}$ ]] ||
  [[ "${source_commit}" != *[1-9a-f]* ]]; then
  printf 'source commit must be a canonical non-zero lowercase Git object ID\n' >&2
  exit 1
fi
if [[ "$(git -C "${workspace_root}" rev-parse --verify 'HEAD^{commit}')" != "${source_commit}" ]]; then
  printf 'the checked-out source does not match %s\n' "${source_commit}" >&2
  exit 1
fi

case "${architecture}" in
amd64)
  expected_machine="x86_64"
  ;;
arm64)
  expected_machine="aarch64"
  ;;
*) usage_error "--architecture must be amd64 or arm64" ;;
esac
if [[ "$(uname -m)" != "${expected_machine}" ]]; then
  printf 'smoke runner architecture mismatch: expected %s, got %s\n' "${expected_machine}" "$(uname -m)" >&2
  exit 1
fi

if [[ ! -d "${release_dir}" || -L "${release_dir}" ]]; then
  printf 'release directory must be a physical directory\n' >&2
  exit 1
fi
release_dir="$(cd -- "${release_dir}" && pwd -P)"
if [[ ! -f "${public_key_path}" || -L "${public_key_path}" ]]; then
  printf 'public key must be a regular, non-symbolic file\n' >&2
  exit 1
fi
public_key_path="$(cd -- "$(dirname -- "${public_key_path}")" && pwd -P)/$(basename -- "${public_key_path}")"
public_key_size="$(wc -c <"${public_key_path}" | tr -d '[:space:]')"
public_key="$(<"${public_key_path}")"
if [[ ! "${public_key}" =~ ^[A-Za-z0-9+/]{43}=$ ]] ||
  [[ "${public_key_size}" != 44 && "${public_key_size}" != 45 ]]; then
  printf 'public key must contain one standard-Base64 Ed25519 public key\n' >&2
  exit 1
fi

phase 'verify signed artifacts and build identity'
go tool sign-release validate-version --version "${release_version}"

expected_assets=(
  SHA256SUMS
  SHA256SUMS.sig
  sing-box-panel-linux-amd64
  sing-box-panel-linux-arm64
)
mapfile -t actual_assets < <(
  find "${release_dir}" -mindepth 1 -maxdepth 1 -printf '%f\n' | LC_ALL=C sort
)
if [[ ${#actual_assets[@]} -ne ${#expected_assets[@]} ]]; then
  printf 'release directory must contain exactly four assets\n' >&2
  exit 1
fi
for index in "${!expected_assets[@]}"; do
  if [[ "${actual_assets[index]}" != "${expected_assets[index]}" ]]; then
    printf 'unexpected release asset set\n' >&2
    exit 1
  fi
done
for asset_name in "${expected_assets[@]}"; do
  asset_path="${release_dir}/${asset_name}"
  if [[ ! -f "${asset_path}" || -L "${asset_path}" ]]; then
    printf 'release asset must be a regular, non-symbolic file: %s\n' "${asset_name}" >&2
    exit 1
  fi
done

checksum_entries="$(awk 'NF == 2 { name = $2; sub(/^\*/, "", name); sub(/^\.\//, "", name); print name }' "${release_dir}/SHA256SUMS" | LC_ALL=C sort)"
if [[ "${checksum_entries}" != $'sing-box-panel-linux-amd64\nsing-box-panel-linux-arm64' ]]; then
  printf 'SHA256SUMS must contain exactly the two release binaries\n' >&2
  exit 1
fi
go tool sign-release verify \
  --public-key "${public_key_path}" \
  --version "${release_version}" \
  --checksums "${release_dir}/SHA256SUMS" \
  --signature "${release_dir}/SHA256SUMS.sig"
(
  cd -- "${release_dir}"
  sha256sum --check --strict SHA256SUMS
)

release_binary="${release_dir}/sing-box-panel-linux-${architecture}"
chmod 0755 "${release_binary}"
release_metadata="$(${release_binary} --output json version)"
source_date="$(git -C "${workspace_root}" show -s --format=%cI "${source_commit}")"
assert_json 'release version, commit and date match the frozen source' \
  --arg version "${release_version}" \
  --arg commit "${source_commit}" \
  --arg date "${source_date}" \
  '.version == $version and .commit == $commit and .date == $date' \
  <<<"${release_metadata}"
binary_metadata="$(go version -m "${release_binary}")"
for expected_line in \
  $'\tpath\tgithub.com/rehuony/sing-box-panel/cmd/sing-box-panel' \
  $'\tbuild\t-trimpath=true' \
  $'\tbuild\tCGO_ENABLED=0' \
  $'\tbuild\tGOOS=linux' \
  $'\tbuild\tGOARCH='"${architecture}"; do
  grep -Fqx "${expected_line}" <<<"${binary_metadata}" || {
    printf 'release binary metadata is missing %s\n' "${expected_line}" >&2
    exit 1
  }
done
LC_ALL=C grep -aFq -- "${public_key}" "${release_binary}" || {
  printf 'release binary does not embed the repository update public key\n' >&2
  exit 1
}

smoke_root="$(mktemp -d "${RUNNER_TEMP:-/tmp}/sing-box-panel-release-smoke.XXXXXX")"
panel_pid=""
mock_pid=""

terminate_process() {
  local process_id="$1"
  local count

  if [[ -z "${process_id}" ]] || ! kill -0 "${process_id}" 2>/dev/null; then
    return 0
  fi
  kill -TERM "${process_id}" 2>/dev/null || true
  for ((count = 0; count < 40; count++)); do
    if ! kill -0 "${process_id}" 2>/dev/null; then
      wait "${process_id}" 2>/dev/null || true
      return 0
    fi
    sleep 0.25
  done
  kill -KILL "${process_id}" 2>/dev/null || true
  wait "${process_id}" 2>/dev/null || true
}

cleanup() {
  local smoke_status=$?
  local log_path

  set +e
  terminate_process "${panel_pid}"
  terminate_process "${mock_pid}"
  if [[ ${smoke_status} -ne 0 ]]; then
    for log_path in "${smoke_root}"/*.log; do
      if [[ -f "${log_path}" ]]; then
        printf '\n===== %s (last 65536 bytes) =====\n' "$(basename -- "${log_path}")" >&2
        tail -c 65536 -- "${log_path}" >&2 || true
      fi
    done
  fi
  rm -rf -- "${smoke_root}"
  return "${smoke_status}"
}
trap cleanup EXIT
trap 'exit 130' INT
trap 'exit 143' HUP TERM

allocate_port() {
  python3 -c 'import socket
s = socket.socket()
s.bind(("127.0.0.1", 0))
print(s.getsockname()[1])
s.close()'
}

wait_for_url() {
  local url="$1"
  local count

  for ((count = 0; count < 80; count++)); do
    if curl --fail-with-body --silent --show-error --max-time 2 "${url}" >/dev/null 2>&1; then
      return 0
    fi
    sleep 0.25
  done
  printf 'timed out waiting for %s\n' "${url}" >&2
  return 1
}

mock_root="${smoke_root}/release-server"
phase 'start the isolated release endpoint'
mkdir -p -- "${mock_root}/assets"
for asset_name in "${expected_assets[@]}"; do
  cp -- "${release_dir}/${asset_name}" "${mock_root}/assets/${asset_name}"
  chmod 0644 "${mock_root}/assets/${asset_name}"
done
mock_port="$(allocate_port)"
mock_origin="http://127.0.0.1:${mock_port}"
asset_metadata='[]'
for asset_name in "${expected_assets[@]}"; do
  asset_size="$(wc -c <"${mock_root}/assets/${asset_name}" | tr -d '[:space:]')"
  asset_metadata="$(
    jq -c \
      --arg name "${asset_name}" \
      --arg url "${mock_origin}/assets/${asset_name}" \
      --argjson size "${asset_size}" \
      '. + [{name: $name, browser_download_url: $url, size: $size}]' \
      <<<"${asset_metadata}"
  )"
done
jq -n \
  --arg tag "${release_version}" \
  --argjson assets "${asset_metadata}" \
  '{tag_name: $tag, draft: false, prerelease: false, assets: $assets}' \
  >"${mock_root}/latest"
python3 -m http.server "${mock_port}" \
  --bind 127.0.0.1 \
  --directory "${mock_root}" \
  >"${smoke_root}/release-server.log" 2>&1 &
mock_pid=$!
wait_for_url "${mock_origin}/latest"

probe_version="v0.0.0-smoke"
phase 'build and initialize the lower-version probe'
install_dir="${smoke_root}/install"
settings_path="${smoke_root}/config/setting.json"
smoke_config_home="${smoke_root}/xdg-config"
smoke_data_home="${smoke_root}/xdg-data"
installed_binary="${install_dir}/sing-box-panel"
mkdir -p -- "${install_dir}" "$(dirname -- "${settings_path}")" "${smoke_config_home}" "${smoke_data_home}"
if [[ ! -s "${workspace_root}/web/dist/index.html" ]]; then
  printf 'web/dist must be built before the native smoke-test probe\n' >&2
  exit 1
fi
(
  cd -- "${workspace_root}"
  env \
    CGO_ENABLED=0 \
    GOARCH="${architecture}" \
    GOOS=linux \
    GOAMD64=v1 GOARM64=v8.0 GOENV=off GOEXPERIMENT= GOFIPS140=off GOWORK=off \
    GOFLAGS='-mod=readonly' \
    GOTOOLCHAIN=local \
    go build \
      -buildvcs=false \
      -trimpath \
      -ldflags="-s -w -X=github.com/rehuony/sing-box-panel/internal/buildinfo.version=${probe_version} -X=github.com/rehuony/sing-box-panel/internal/buildinfo.commit=${source_commit} -X=github.com/rehuony/sing-box-panel/internal/buildinfo.date=${source_date} -X=github.com/rehuony/sing-box-panel/internal/selfupdate.embeddedPublicKey=${public_key} -X=github.com/rehuony/sing-box-panel/internal/selfupdate.defaultLatestReleaseURL=${mock_origin}/latest" \
      -o "${installed_binary}" \
      ./cmd/sing-box-panel
)
chmod 0755 "${installed_binary}"
probe_metadata="$(${installed_binary} --output json version)"
assert_json 'probe version' --arg version "${probe_version}" '.version == $version' <<<"${probe_metadata}"

run_installed() {
  env \
    XDG_CONFIG_HOME="${smoke_config_home}" \
    XDG_DATA_HOME="${smoke_data_home}" \
    "${installed_binary}" --config "${settings_path}" "$@"
}

run_installed init >/dev/null
panel_port="$(allocate_port)"
settings_temporary="${settings_path}.tmp"
jq \
  --argjson port "${panel_port}" \
  '.server.host = "127.0.0.1" | .server.port = $port | .server.external_origin = "" | .auth.secure_cookie = false' \
  "${settings_path}" >"${settings_temporary}"
chmod 0600 "${settings_temporary}"
mv -- "${settings_temporary}" "${settings_path}"
run_installed config check >/dev/null

management_token="$(jq -er '.auth.token | select(type == "string" and length > 0)' "${settings_path}")"
panel_origin="http://127.0.0.1:${panel_port}"
start_panel() {
  local expected_version="$1"

  env \
    XDG_CONFIG_HOME="${smoke_config_home}" \
    XDG_DATA_HOME="${smoke_data_home}" \
    "${installed_binary}" --config "${settings_path}" server start \
    >"${smoke_root}/panel-${expected_version}.log" 2>&1 &
  panel_pid=$!
  wait_for_url "${panel_origin}/api/v1/health"
  health_payload="$(curl --fail-with-body --silent --show-error --max-time 2 "${panel_origin}/api/v1/health")"
  assert_json 'healthy panel reports the expected running version' --arg version "${expected_version}" \
    '.status == "ok" and .version == $version' <<<"${health_payload}"
}

stop_panel() {
  local process_id="${panel_pid}"
  local count
  local process_status

  [[ -n "${process_id}" ]] || return 0
  kill -TERM "${process_id}"
  for ((count = 0; count < 80; count++)); do
    if ! kill -0 "${process_id}" 2>/dev/null; then
      if wait "${process_id}"; then
        process_status=0
      else
        process_status=$?
      fi
      panel_pid=""
      if [[ ${process_status} -ne 0 && ${process_status} -ne 143 ]]; then
        printf 'panel exited with status %d after SIGTERM\n' "${process_status}" >&2
        return 1
      fi
      return 0
    fi
    sleep 0.25
  done
  printf 'panel did not stop after SIGTERM\n' >&2
  return 1
}

authenticated_get() {
  local path="$1"
  curl \
    --fail-with-body \
    --silent \
    --show-error \
    --max-time 5 \
    --header "Authorization: Bearer ${management_token}" \
    "${panel_origin}${path}"
}

authenticated_put() {
  local path="$1"
  local expected_status="${2:-200}"
  local response_status
  response_status="$(curl --silent --show-error --max-time 10 \
    --request PUT \
    --header "Authorization: Bearer ${management_token}" \
    --header 'Content-Type: application/json' \
    --data-binary @- \
    --output "${smoke_root}/response.json" \
    --write-out '%{http_code}' \
    "${panel_origin}${path}")"
  if [[ "${response_status}" != "${expected_status}" ]]; then
    printf 'PUT %s returned %s, expected %s\n' "${path}" "${response_status}" "${expected_status}" >&2
    return 1
  fi
  cat "${smoke_root}/response.json"
}

phase 'exercise the current editable configuration and settings APIs'
start_panel "${probe_version}"
status_payload="$(authenticated_get '/api/v1/system/status')"
assert_json 'fresh instance has no configuration history or running core' --arg version "${probe_version}" \
  '.panel_version == $version and .canonical_revision == 0 and .running == false' <<<"${status_payload}"
curl --fail-with-body --silent --show-error --max-time 5 "${panel_origin}/" >"${smoke_root}/index.html"
grep -qi '<html' "${smoke_root}/index.html"

configuration_fixture="${workspace_root}/scripts/testdata/release-configuration.json"
initial_file="$(authenticated_get '/api/v1/config/file')"
assert_json 'fresh editable file' \
  '.revision == 0 and .content == "{}" and .syntax_valid == true' <<<"${initial_file}"
file_write="$(jq -n --rawfile content "${configuration_fixture}" '{revision: 0, content: $content}')"
saved_file="$(authenticated_put '/api/v1/config/file' <<<"${file_write}")"
assert_json 'valid file preserves exact text and links immutable history' \
  --rawfile content "${configuration_fixture}" \
  '.revision == 1 and .content == $content and .syntax_valid == true and (.canonical_revision_id | length > 0)' \
  <<<"${saved_file}"
stale_save="$(authenticated_put '/api/v1/config/file' 412 <<<"${file_write}")"
assert_json 'stale editable-file writes are rejected' '.code == "configuration_file_conflict"' <<<"${stale_save}"

panel_settings="$(authenticated_get '/api/v1/panel/settings')"
settings_write="$(jq '{revision, preferences: (.preferences | .language = "en" | .appearance.theme = "dark")}' <<<"${panel_settings}")"
saved_settings="$(authenticated_put '/api/v1/panel/settings' <<<"${settings_write}")"
assert_json 'panel settings are saved through the current settings API' \
  '.preferences.language == "en" and .preferences.appearance.theme == "dark"' <<<"${saved_settings}"
saved_settings="$(jq '{revision, preferences, github_token_configured, identity_key_configured}' <<<"${saved_settings}")"
settings_digest="$(sha256sum "${settings_path}" | cut -d ' ' -f 1)"

# Unfinished text must survive an update without falling back to the valid head.
draft_write="$(jq -n --arg content $'{\n  "log": ' '{revision: 1, content: $content}')"
saved_draft="$(authenticated_put '/api/v1/config/file' <<<"${draft_write}")"
assert_json 'unfinished JSON remains editable without a usable canonical revision' \
  --argjson input "${draft_write}" \
  '.revision == 2 and .content == $input.content and .syntax_valid == false and (.canonical_revision_id // "") == ""' \
  <<<"${saved_draft}"

phase 'authenticate and install the update while the probe keeps running'
update_result="$(run_installed --output json update)"
assert_json 'self-update installs the expected release at the existing executable path' \
  --arg previous "${probe_version}" \
  --arg version "${release_version}" \
  --arg path "${installed_binary}" \
  '.updated == true and .previous_version == $previous and .version == $version and .executable_path == $path' \
  <<<"${update_result}"
cmp "${installed_binary}" "${release_binary}"

health_after_update="$(curl --fail-with-body --silent --show-error --max-time 2 "${panel_origin}/api/v1/health")"
assert_json 'running process stays on the probe until restart' --arg version "${probe_version}" \
  '.status == "ok" and .version == $version' <<<"${health_after_update}"

phase 'restart the release and verify persistent state'
stop_panel
updated_metadata="$(${installed_binary} --output json version)"
assert_json 'installed binary reports the release version and frozen commit' \
  --arg version "${release_version}" \
  --arg commit "${source_commit}" \
  '.version == $version and .commit == $commit' <<<"${updated_metadata}"
run_installed config check >/dev/null

start_panel "${release_version}"
updated_status="$(authenticated_get '/api/v1/system/status')"
assert_json 'release retains history and does not start a core implicitly' --arg version "${release_version}" \
  '.panel_version == $version and .canonical_revision == 1 and .running == false' <<<"${updated_status}"
persisted_file="$(authenticated_get '/api/v1/config/file')"
assert_json 'unfinished configuration text and revision survive restart unchanged' \
  --argjson saved "${saved_draft}" '. == $saved' <<<"${persisted_file}"
persisted_settings="$(authenticated_get '/api/v1/panel/settings')"
assert_json 'panel preferences and credential-presence flags survive restart unchanged' \
  --argjson saved "${saved_settings}" \
  '{revision, preferences, github_token_configured, identity_key_configured} == $saved' <<<"${persisted_settings}"
[[ "$(sha256sum "${settings_path}" | cut -d ' ' -f 1)" == "${settings_digest}" ]]

phase 'correct the draft through the new binary and verify another restart'
file_write="$(jq -n --rawfile content "${configuration_fixture}" '{revision: 2, content: $content}')"
corrected_file="$(authenticated_put '/api/v1/config/file' <<<"${file_write}")"
assert_json 'corrected text reconnects to the existing valid history' \
  --argjson saved "${saved_file}" \
  '.revision == 3 and .syntax_valid == true and .content == $saved.content and .canonical_revision_id == $saved.canonical_revision_id' \
  <<<"${corrected_file}"
stale_save="$(authenticated_put '/api/v1/config/file' 412 <<<"${draft_write}")"
assert_json 'CAS remains enforced after self-update' '.code == "configuration_file_conflict"' <<<"${stale_save}"
stop_panel
start_panel "${release_version}"
persisted_file="$(authenticated_get '/api/v1/config/file')"
assert_json 'corrected configuration persists across the second restart' \
  --argjson saved "${corrected_file}" '. == $saved' <<<"${persisted_file}"
stop_panel

printf 'release smoke test passed for linux/%s at %s\n' "${architecture}" "${source_commit}"
