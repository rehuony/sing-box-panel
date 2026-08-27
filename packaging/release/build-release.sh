#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-or-later

set -euo pipefail

usage() {
  cat >&2 <<'EOF'
usage:
  packaging/release/build-release.sh snapshot --output DIRECTORY
  packaging/release/build-release.sh release --version vX.Y.Z --output DIRECTORY [--date RFC3339]
  packaging/release/build-release.sh verify
EOF
}

usage_error() {
  echo "$1" >&2
  usage
  exit 2
}

script_dir="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd -P)"
workspace_root="$(cd -- "${script_dir}/../.." && pwd -P)"
buildinfo_package="github.com/rehuony/sing-box-panel/internal/buildinfo"

evidence_overlays=(
  release/evidence.json
  release/evidence/core-version-matrix.json
  release/evidence/structured-capability-matrix.json
  release/evidence/linux-runtime-resilience.json
  release/evidence/browser-contract-accessibility.json
  release/evidence/subscription-observability-e2e.json
)

source_commit=""
source_date=""
release_target_goos=""
release_target_goarch=""
release_target_cgo=0

resolve_source_identity() {
  local git_root

  if ! git_root="$(git -C "${workspace_root}" rev-parse --show-toplevel 2>/dev/null)"; then
    echo "release builds require a Git worktree" >&2
    return 1
  fi
  git_root="$(cd -- "${git_root}" && pwd -P)"
  if [[ "${git_root}" != "${workspace_root}" ]]; then
    echo "release builds must use the repository root containing the build sources" >&2
    return 1
  fi
  if ! source_commit="$(git -C "${workspace_root}" rev-parse --verify 'HEAD^{commit}' 2>/dev/null)"; then
    echo "cannot resolve the checked-out Git HEAD" >&2
    return 1
  fi
  if [[ ! "${source_commit}" =~ ^[0-9a-f]{40}$ && ! "${source_commit}" =~ ^[0-9a-f]{64}$ ]] ||
    [[ "${source_commit}" != *[1-9a-f]* ]]; then
    echo "Git HEAD is not a canonical non-zero lowercase commit identifier" >&2
    return 1
  fi
  if ! source_date="$(git -C "${workspace_root}" show -s --format=%cI "${source_commit}")" ||
    [[ -z "${source_date}" ]]; then
    echo "cannot resolve the Git HEAD commit timestamp" >&2
    return 1
  fi
}

is_rfc3339() {
  local value="$1"
  local year
  local month
  local day
  local maximum_day

  if [[ ! "${value}" =~ ^[0-9]{4}-(0[1-9]|1[0-2])-(0[1-9]|[12][0-9]|3[01])T([01][0-9]|2[0-3]):[0-5][0-9]:[0-5][0-9](\.[0-9]+)?(Z|[+-]([01][0-9]|2[0-3]):[0-5][0-9])$ ]]; then
    return 1
  fi
  year=$((10#${value:0:4}))
  month=$((10#${value:5:2}))
  day=$((10#${value:8:2}))
  case "${month}" in
  2)
    maximum_day=28
    if ((year % 400 == 0 || (year % 4 == 0 && year % 100 != 0))); then
      maximum_day=29
    fi
    ;;
  4 | 6 | 9 | 11) maximum_day=30 ;;
  *) maximum_day=31 ;;
  esac
  ((day <= maximum_day))
}

run_go() {
  env \
    GO111MODULE=on \
    GOENV=off \
    GOAMD64=v1 \
    GOARM64=v8.0 \
    GOEXPERIMENT= \
    "GOFLAGS=-mod=readonly -modcacherw" \
    GOFIPS140=off \
    GOOS="${release_target_goos}" \
    GOARCH="${release_target_goarch}" \
    CGO_ENABLED="${release_target_cgo}" \
    GOCACHE="${release_go_build_cache}" \
    GOMODCACHE="${release_go_module_cache}" \
    GOPATH="${release_go_path}" \
    GOTOOLCHAIN=local \
    GOWORK=off \
    go "$@"
}

run_pnpm() {
  env -i \
    PATH="${PATH}" \
    HOME="${release_web_state}/home" \
    TMPDIR="${release_web_state}/tmp" \
    XDG_CACHE_HOME="${release_web_state}/xdg-cache" \
    XDG_CONFIG_HOME="${release_web_state}/xdg-config" \
    XDG_DATA_HOME="${release_web_state}/xdg-data" \
    CI=true \
    COREPACK_ENABLE_DOWNLOAD_PROMPT=0 \
    COREPACK_HOME="${release_web_state}/corepack" \
    NODE_OPTIONS= \
    NODE_PATH= \
    NPM_CONFIG_CACHE="${release_web_state}/npm-cache" \
    NPM_CONFIG_DRY_RUN=false \
    NPM_CONFIG_GLOBALCONFIG="${release_web_state}/global.npmrc" \
    NPM_CONFIG_NODE_OPTIONS= \
    NPM_CONFIG_SCRIPT_SHELL=/bin/sh \
    NPM_CONFIG_USERCONFIG="${release_web_state}/user.npmrc" \
    corepack pnpm "$@"
}

prepare_source_snapshot() {
  local source_root="$1"
  local web_parent="$2"
  local overlay
  local source
  local target

  if [[ ! -d "${workspace_root}/release" || -L "${workspace_root}/release" ]] ||
    [[ ! -d "${workspace_root}/release/evidence" || -L "${workspace_root}/release/evidence" ]]; then
    echo "the working tree must contain regular release evidence directories" >&2
    return 1
  fi

  mkdir -p -- "${source_root}" "${web_parent}"
  git -C "${workspace_root}" archive --format=tar "${source_commit}" |
    tar -xf - -C "${source_root}"
  git -C "${workspace_root}" archive --format=tar "${source_commit}" web |
    tar -xf - -C "${web_parent}"

  if [[ ! -d "${source_root}/release" || -L "${source_root}/release" ]] ||
    [[ ! -d "${source_root}/release/evidence" || -L "${source_root}/release/evidence" ]]; then
    echo "HEAD does not contain regular release evidence directories" >&2
    return 1
  fi
  if [[ ! -d "${source_root}/web" || -L "${source_root}/web" ]] ||
    [[ ! -d "${web_parent}/web" || -L "${web_parent}/web" ]]; then
    echo "HEAD does not contain a regular Web source directory" >&2
    return 1
  fi

  for overlay in "${evidence_overlays[@]}"; do
    source="${workspace_root}/${overlay}"
    target="${source_root}/${overlay}"
    rm -rf -- "${target}"
    if [[ -L "${source}" ]]; then
      echo "release evidence overlay must not be a symbolic link: ${overlay}" >&2
      return 1
    fi
    if [[ -e "${source}" ]]; then
      if [[ ! -f "${source}" ]]; then
        echo "release evidence overlay must be a regular file: ${overlay}" >&2
        return 1
      fi
      cp -- "${source}" "${target}"
    fi
  done
}

prepare_isolated_state() {
  local state_root="$1"

  release_go_path="${state_root}/go/gopath"
  release_go_module_cache="${release_go_path}/pkg/mod"
  release_go_build_cache="${state_root}/go/build-cache"
  release_web_state="${state_root}/web"
  mkdir -p -- \
    "${release_go_module_cache}" \
    "${release_go_build_cache}" \
    "${release_web_state}/home" \
    "${release_web_state}/tmp" \
    "${release_web_state}/xdg-cache" \
    "${release_web_state}/xdg-config" \
    "${release_web_state}/xdg-data" \
    "${release_web_state}/corepack" \
    "${release_web_state}/npm-cache" \
    "${release_web_state}/pnpm-store"
  : >"${release_web_state}/global.npmrc"
  : >"${release_web_state}/user.npmrc"
}

build_web_distribution() {
  local web_root="$1"
  local source_root="$2"
  local expected_pnpm
  local actual_pnpm

  if ! command -v corepack >/dev/null 2>&1; then
    echo "corepack is required to build the pinned Web dependencies" >&2
    return 1
  fi
  expected_pnpm="$(sed -n 's/^[[:space:]]*"packageManager":[[:space:]]*"pnpm@\([^"]*\)".*/\1/p' "${web_root}/package.json")"
  if [[ -z "${expected_pnpm}" ]]; then
    echo "web/package.json does not pin pnpm" >&2
    return 1
  fi
  actual_pnpm="$(cd -- "${web_root}" && run_pnpm --version)"
  if [[ "${actual_pnpm}" != "${expected_pnpm}" ]]; then
    echo "Corepack selected pnpm ${actual_pnpm}, expected ${expected_pnpm}" >&2
    return 1
  fi

  (
    cd -- "${web_root}"
    run_pnpm install \
      --frozen-lockfile \
      --ignore-scripts \
      --verify-store-integrity \
      --store-dir "${release_web_state}/pnpm-store"
    run_pnpm run build
  )

  if [[ ! -s "${web_root}/node_modules/.pnpm/lock.yaml" ]] ||
    ! cmp "${web_root}/pnpm-lock.yaml" "${web_root}/node_modules/.pnpm/lock.yaml"; then
    echo "pnpm did not recreate node_modules from the frozen lock file" >&2
    return 1
  fi
  if [[ ! -s "${web_root}/dist/index.html" ]] ||
    [[ ! -d "${web_root}/dist/assets" ]] ||
    ! find "${web_root}/dist/assets" -type f -print -quit | grep -q .; then
    echo "the Web build did not create dist/index.html and assets" >&2
    return 1
  fi
  if find "${web_root}/dist" -type l -print -quit | grep -q .; then
    echo "the Web distribution must not contain symbolic links" >&2
    return 1
  fi

  rm -rf -- "${source_root}/web/dist"
  cp -R -- "${web_root}/dist" "${source_root}/web/dist"
}

build_binary() {
  local source_root="$1"
  local operating_system="$2"
  local architecture="$3"
  local target="$4"
  local ldflags="$5"

  (
    cd -- "${source_root}"
    release_target_cgo=0 \
      release_target_goos="${operating_system}" \
      release_target_goarch="${architecture}" \
      run_go build \
        -mod=readonly \
        -buildvcs=false \
        -tags webdist \
        -trimpath \
        -ldflags="${ldflags}" \
        -o "${target}" \
        ./cmd/sing-box-panel
  )
  chmod 0755 "${target}"
}

write_checksums() {
  local staging_dir="$1"

  (
    cd -- "${staging_dir}"
    if command -v sha256sum >/dev/null 2>&1; then
      sha256sum sing-box-panel-linux-amd64 sing-box-panel-linux-arm64 >SHA256SUMS
    elif command -v shasum >/dev/null 2>&1; then
      shasum -a 256 sing-box-panel-linux-amd64 sing-box-panel-linux-arm64 >SHA256SUMS
    else
      echo "sha256sum or shasum is required" >&2
      return 1
    fi
  )
}

check_checksums() {
  local directory="$1"

  (
    cd -- "${directory}"
    if command -v sha256sum >/dev/null 2>&1; then
      sha256sum --check SHA256SUMS >/dev/null
    elif command -v shasum >/dev/null 2>&1; then
      shasum -a 256 --check SHA256SUMS >/dev/null
    else
      echo "sha256sum or shasum is required" >&2
      return 1
    fi
  )
}

check_binary_build() {
  local binary="$1"
  local architecture="$2"
  local architecture_baseline
  local metadata

  metadata="$(go version -m "${binary}")"
  for expected in \
    $'\tpath\tgithub.com/rehuony/sing-box-panel/cmd/sing-box-panel' \
    $'\tbuild\t-tags=webdist' \
    $'\tbuild\t-trimpath=true' \
    $'\tbuild\tCGO_ENABLED=0' \
    $'\tbuild\tGOOS=linux' \
    $'\tbuild\tGOARCH='"${architecture}"; do
    if ! grep -Fqx "${expected}" <<<"${metadata}"; then
      echo "release binary metadata is missing ${expected}" >&2
      return 1
    fi
  done
  if grep -Fq $'\tbuild\tvcs' <<<"${metadata}"; then
    echo "release binary unexpectedly contains automatic VCS metadata" >&2
    return 1
  fi
  case "${architecture}" in
  amd64) architecture_baseline=$'\tbuild\tGOAMD64=v1' ;;
  arm64) architecture_baseline=$'\tbuild\tGOARM64=v8.0' ;;
  *)
    echo "unsupported release architecture ${architecture}" >&2
    return 1
    ;;
  esac
  if ! grep -Fqx "${architecture_baseline}" <<<"${metadata}"; then
    echo "release binary metadata has the wrong ${architecture} baseline" >&2
    return 1
  fi
}

validate_staging() {
  local staging_dir="$1"
  local entry_count

  for binary in \
    "${staging_dir}/sing-box-panel-linux-amd64" \
    "${staging_dir}/sing-box-panel-linux-arm64"; do
    if [[ ! -f "${binary}" || -L "${binary}" || ! -x "${binary}" ]]; then
      echo "release binary is missing, non-regular, symbolic, or non-executable: ${binary}" >&2
      return 1
    fi
  done
  if [[ ! -f "${staging_dir}/SHA256SUMS" || -L "${staging_dir}/SHA256SUMS" ]]; then
    echo "release checksum file is missing, non-regular, or symbolic" >&2
    return 1
  fi
  entry_count="$(find "${staging_dir}" -mindepth 1 -maxdepth 1 -print | wc -l | tr -d '[:space:]')"
  if [[ "${entry_count}" != 3 ]]; then
    echo "release staging contains unexpected outputs" >&2
    return 1
  fi

  check_checksums "${staging_dir}"
  check_binary_build "${staging_dir}/sing-box-panel-linux-amd64" amd64
  check_binary_build "${staging_dir}/sing-box-panel-linux-arm64" arm64
}

verify_runtime_metadata() {
  local source_root="$1"
  local staging_dir="$2"
  local build_root="$3"
  local version="$4"
  local commit="$5"
  local date="$6"
  local ldflags="$7"
  local host_os
  local host_arch
  local probe
  local actual
  local expected

  host_os="$(go env GOHOSTOS)"
  host_arch="$(go env GOHOSTARCH)"
  if [[ "${host_os}" == linux && ("${host_arch}" == amd64 || "${host_arch}" == arm64) ]]; then
    probe="${staging_dir}/sing-box-panel-linux-${host_arch}"
  else
    probe="${build_root}/sing-box-panel-metadata-probe"
    build_binary "${source_root}" "${host_os}" "${host_arch}" "${probe}" "${ldflags}"
  fi

  actual="$("${probe}" --output json version)"
  expected="$(printf '{"version":"%s","commit":"%s","date":"%s"}' "${version}" "${commit}" "${date}")"
  if [[ "${actual}" != "${expected}" ]]; then
    echo "release metadata mismatch: got ${actual}, expected ${expected}" >&2
    return 1
  fi
}

build_distribution() (
  set -euo pipefail

  local mode="$1"
  local requested_output="$2"
  local version="$3"
  local date="$4"
  local verify_metadata="$5"
  local output_parent_input
  local output_parent
  local output_name
  local final_output
  local build_root=""
  local staging_dir=""
  local source_root
  local web_parent
  local web_root
  local state_root
  local gate_status
  local ldflags

  cleanup_build() {
    if [[ -n "${staging_dir}" && -d "${staging_dir}" ]]; then
      rm -rf -- "${staging_dir}"
    fi
    if [[ -n "${build_root}" && -d "${build_root}" ]]; then
      rm -rf -- "${build_root}"
    fi
  }
  trap cleanup_build EXIT

  if [[ -z "${requested_output}" ]]; then
    echo "release output directory is empty" >&2
    return 2
  fi
  output_parent_input="$(dirname -- "${requested_output}")"
  output_name="$(basename -- "${requested_output}")"
  if [[ "${output_name}" == . || "${output_name}" == .. || "${output_name}" == / ]]; then
    echo "invalid release output directory ${requested_output}" >&2
    return 2
  fi
  if [[ ! -d "${output_parent_input}" ]]; then
    echo "release output parent does not exist: ${output_parent_input}" >&2
    return 2
  fi
  output_parent="$(cd -- "${output_parent_input}" && pwd -P)"
  final_output="${output_parent}/${output_name}"
  if [[ -e "${final_output}" || -L "${final_output}" ]]; then
    echo "release output already exists: ${final_output}" >&2
    return 1
  fi

  build_root="$(mktemp -d "${TMPDIR:-/tmp}/sing-box-panel-release.XXXXXX")"
  source_root="${build_root}/source"
  web_parent="${build_root}/web-input"
  web_root="${web_parent}/web"
  state_root="${build_root}/state"
  prepare_source_snapshot "${source_root}" "${web_parent}"
  prepare_isolated_state "${state_root}"

  (
    cd -- "${source_root}"
    run_go mod download all
    run_go mod verify
  )
  if [[ "${mode}" == release ]]; then
    set +e
    (
      cd -- "${source_root}"
      run_go tool release-readiness \
        --release-version "${version}" \
        --source-commit "${source_commit}"
    )
    gate_status=$?
    set -e
    if [[ ${gate_status} -ne 0 ]]; then
      return "${gate_status}"
    fi
  fi

  build_web_distribution "${web_root}" "${source_root}"

  if [[ -e "${final_output}" || -L "${final_output}" ]]; then
    echo "release output appeared while the build was running: ${final_output}" >&2
    return 1
  fi
  staging_dir="$(mktemp -d "${output_parent}/.${output_name}.staging.XXXXXX")"
  ldflags="-s -w -X=${buildinfo_package}.version=${version} -X=${buildinfo_package}.commit=${source_commit} -X=${buildinfo_package}.date=${date}"

  build_binary \
    "${source_root}" linux amd64 \
    "${staging_dir}/sing-box-panel-linux-amd64" "${ldflags}"
  build_binary \
    "${source_root}" linux arm64 \
    "${staging_dir}/sing-box-panel-linux-arm64" "${ldflags}"
  write_checksums "${staging_dir}"
  validate_staging "${staging_dir}"
  if [[ "${verify_metadata}" == true ]]; then
    verify_runtime_metadata \
      "${source_root}" "${staging_dir}" "${build_root}" \
      "${version}" "${source_commit}" "${date}" "${ldflags}"
  fi

  if [[ -e "${final_output}" || -L "${final_output}" ]]; then
    echo "refusing to replace release output: ${final_output}" >&2
    return 1
  fi
  mv -- "${staging_dir}" "${final_output}"
  staging_dir=""
  echo "built ${mode} artifacts in ${final_output}"
)

run_verify() (
  set -euo pipefail

  local verify_root=""
  local snapshot_output
  local formal_output
  local formal_status

  cleanup_verify() {
    if [[ -n "${verify_root}" && -d "${verify_root}" ]]; then
      rm -rf -- "${verify_root}"
    fi
  }
  trap cleanup_verify EXIT

  verify_root="$(mktemp -d "${TMPDIR:-/tmp}/sing-box-panel-release-verify.XXXXXX")"
  snapshot_output="${verify_root}/snapshot"
  formal_output="${verify_root}/formal"

  build_distribution snapshot "${snapshot_output}" dev "${source_date}" true

  set +e
  build_distribution release "${formal_output}" v0.0.0 "${source_date}" true
  formal_status=$?
  set -e
  case "${formal_status}" in
  0)
    echo "release readiness passed and the formal release path was verified"
    ;;
  3)
    if [[ -e "${formal_output}" || -L "${formal_output}" ]]; then
      echo "a blocked formal release left output behind" >&2
      return 1
    fi
    echo "release readiness is blocked and the formal release failed without output as expected"
    ;;
  *)
    echo "formal release verification failed with status ${formal_status}" >&2
    return 1
    ;;
  esac
)

if [[ $# -lt 1 ]]; then
  usage_error "a release command is required"
fi

command_name="$1"
shift

case "${command_name}" in
snapshot)
  [[ $# -eq 2 && "$1" == --output ]] || usage_error "snapshot requires --output DIRECTORY"
  output="$2"
  resolve_source_identity
  build_distribution snapshot "${output}" dev "${source_date}" true
  ;;
release)
  if [[ $# -ne 4 && $# -ne 6 ]] || [[ "$1" != --version || "$3" != --output ]] ||
    [[ $# -eq 6 && "$5" != --date ]]; then
    usage_error "release requires --version VERSION --output DIRECTORY [--date RFC3339]"
  fi
  version="$2"
  output="$4"
  resolve_source_identity
  if [[ $# -eq 6 ]]; then
    date="$6"
  else
    date="${source_date}"
  fi
  is_rfc3339 "${date}" || usage_error "--date must be an RFC3339 timestamp"
  build_distribution release "${output}" "${version}" "${date}" true
  ;;
verify)
  [[ $# -eq 0 ]] || usage_error "verify accepts no arguments"
  resolve_source_identity
  run_verify
  ;;
*) usage_error "unknown release command: ${command_name}" ;;
esac
