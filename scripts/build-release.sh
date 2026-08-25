#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-or-later

set -euo pipefail

usage() {
  echo "usage: $0 OUTPUT_DIRECTORY" >&2
}

if [[ $# -ne 1 ]]; then
  usage
  exit 2
fi

script_dir="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd -P)"
workspace_root="$(cd -- "${script_dir}/.." && pwd -P)"
requested_output_dir="$1"

run_go() {
  env \
    GO111MODULE=on \
    GOENV=off \
    GOAMD64=v1 \
    GOARM64=v8.0 \
    GOEXPERIMENT= \
    GOFLAGS= \
    GOFIPS140=off \
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
    TMPDIR="${TMPDIR:-/tmp}" \
    CI=true \
    COREPACK_ENABLE_DOWNLOAD_PROMPT=0 \
    COREPACK_HOME="${formal_web_backup_root}/corepack" \
    NODE_OPTIONS= \
    NODE_PATH= \
    NPM_CONFIG_DRY_RUN=false \
    NPM_CONFIG_GLOBALCONFIG="${formal_web_backup_root}/global.npmrc" \
    NPM_CONFIG_NODE_OPTIONS= \
    NPM_CONFIG_SCRIPT_SHELL=/bin/sh \
    NPM_CONFIG_USERCONFIG="${formal_web_backup_root}/user.npmrc" \
    pnpm "$@"
}

formal_web_backup_root=""
formal_go_root=""
release_go_build_cache=""
release_go_module_cache=""
release_go_path=""
staging_dir=""

cleanup() {
  local generated
  local saved
  local tree

  if [[ -n "${staging_dir}" && -d "${staging_dir}" ]]; then
    rm -rf -- "${staging_dir}"
  fi
  if [[ -n "${formal_go_root}" && -d "${formal_go_root}" ]]; then
    rm -rf -- "${formal_go_root}"
  fi
  if [[ -z "${formal_web_backup_root}" ]]; then
    return
  fi
  for tree in node_modules dist; do
    generated="${workspace_root}/web/${tree}"
    saved="${formal_web_backup_root}/${tree}"
    rm -rf -- "${generated}"
    if [[ -e "${saved}" || -L "${saved}" ]]; then
      mv -- "${saved}" "${generated}"
    fi
  done
  rm -rf -- "${formal_web_backup_root}"
}
trap cleanup EXIT

prepare_development_go_workspace() {
  local configured

  configured="$(go env GOPATH GOMODCACHE GOCACHE)"
  release_go_path="$(printf '%s\n' "${configured}" | sed -n '1p')"
  release_go_module_cache="$(printf '%s\n' "${configured}" | sed -n '2p')"
  release_go_build_cache="$(printf '%s\n' "${configured}" | sed -n '3p')"
  if [[ -z "${release_go_path}" || -z "${release_go_module_cache}" || -z "${release_go_build_cache}" ]]; then
    echo "cannot resolve the active Go workspace/cache paths" >&2
    return 1
  fi
}

prepare_formal_go_workspace() {
  formal_go_root="$(mktemp -d "${TMPDIR:-/tmp}/sing-box-panel-go-inputs.XXXXXX")"
  release_go_path="${formal_go_root}/gopath"
  release_go_module_cache="${release_go_path}/pkg/mod"
  release_go_build_cache="${formal_go_root}/build-cache"
  mkdir -p -- "${release_go_module_cache}" "${release_go_build_cache}"
}

prepare_formal_web_workspace() {
  local source
  local tree

  formal_web_backup_root="$(mktemp -d "${TMPDIR:-/tmp}/sing-box-panel-web-inputs.XXXXXX")"
  mkdir -p -- "${formal_web_backup_root}/corepack" "${formal_web_backup_root}/pnpm-store"
  touch -- "${formal_web_backup_root}/global.npmrc" "${formal_web_backup_root}/user.npmrc"
  for tree in node_modules dist; do
    source="${workspace_root}/web/${tree}"
    if [[ -e "${source}" || -L "${source}" ]]; then
      mv -- "${source}" "${formal_web_backup_root}/${tree}"
    fi
  done
}

verify_fresh_formal_web_workspace() {
  if [[ ! -s "${workspace_root}/web/node_modules/.pnpm/lock.yaml" ]] ||
    ! cmp "${workspace_root}/web/pnpm-lock.yaml" "${workspace_root}/web/node_modules/.pnpm/lock.yaml"; then
    echo "formal pnpm install did not recreate web/node_modules from the frozen lock file" >&2
    return 1
  fi
  if [[ ! -s "${workspace_root}/web/dist/index.html" ]] ||
    [[ ! -d "${workspace_root}/web/dist/assets" ]] ||
    ! find "${workspace_root}/web/dist/assets" -type f -print -quit | grep -q .; then
    echo "formal web build did not recreate web/dist/index.html and assets" >&2
    return 1
  fi
  if find "${workspace_root}/web/dist" -type l -print -quit | grep -q .; then
    echo "formal web output must not contain symbolic links after recreation" >&2
    return 1
  fi
}

resolve_future_directory() {
  local candidate="$1"
  local suffix=""
  local component
  local parent
  local resolved

  if [[ -z "${candidate}" ]]; then
    echo "release output directory is empty" >&2
    return 1
  fi
  while [[ "${candidate}" != "/" && "${candidate}" == */ ]]; do
    candidate="${candidate%/}"
  done
  if [[ "${candidate}" != /* ]]; then
    candidate="$(pwd -P)/${candidate}"
  fi
  while [[ ! -e "${candidate}" ]]; do
    component="${candidate##*/}"
    parent="${candidate%/*}"
    if [[ -z "${parent}" ]]; then
      parent="/"
    fi
    if [[ -z "${component}" || "${parent}" == "${candidate}" ]]; then
      echo "cannot resolve release output directory ${requested_output_dir}" >&2
      return 1
    fi
    suffix="/${component}${suffix}"
    candidate="${parent}"
  done
  if [[ -d "${candidate}" ]]; then
    resolved="$(cd -- "${candidate}" && pwd -P)"
  else
    parent="$(cd -- "$(dirname -- "${candidate}")" && pwd -P)"
    resolved="${parent}/$(basename -- "${candidate}")"
  fi
  printf '%s%s\n' "${resolved}" "${suffix}"
}

verify_release_output_location() {
  local candidate="$1"
  local embedded_root
  for embedded_root in \
    "${workspace_root}/web/dist" \
    "${workspace_root}/web/fallback" \
    "${workspace_root}/release/evidence" \
    "${workspace_root}/internal/store/migrations"; do
    if [[ "${candidate}" == "${embedded_root}" || "${candidate}" == "${embedded_root}/"* ]]; then
      echo "release output directory must not be inside embedded source root ${embedded_root}" >&2
      return 1
    fi
  done
}

release_evidence_overlays=(
  release/evidence.json
  release/evidence/core-version-matrix.json
  release/evidence/structured-capability-matrix.json
  release/evidence/linux-runtime-resilience.json
  release/evidence/browser-contract-accessibility.json
  release/evidence/subscription-observability-e2e.json
)

release_evidence_exclusions=()
for overlay in "${release_evidence_overlays[@]}"; do
  release_evidence_exclusions+=(":(top,exclude)${overlay}")
done

is_allowed_formal_untracked() {
  local candidate="$1"
  local overlay
  for overlay in "${release_evidence_overlays[@]}"; do
    if [[ "${candidate}" == "${overlay}" ]]; then
      return 0
    fi
  done
  case "${candidate}" in
  web/node_modules/* | web/dist/*)
    # Formal builds recreate both trees from the committed pnpm lock file and
    # source before they consume either one.
    return 0
    ;;
  esac
  return 1
}

verify_formal_source_checkout() {
  local git_root
  local actual_commit

  if ! git_root="$(git -C "${workspace_root}" rev-parse --show-toplevel 2>/dev/null)"; then
    echo "formal release requires a Git worktree" >&2
    return 1
  fi
  git_root="$(cd -- "${git_root}" && pwd)"
  if [[ "${git_root}" != "${workspace_root}" ]]; then
    echo "formal release must run from the repository root containing the build sources" >&2
    return 1
  fi
  if ! actual_commit="$(git -C "${workspace_root}" rev-parse --verify 'HEAD^{commit}' 2>/dev/null)"; then
    echo "formal release requires a resolvable Git HEAD commit" >&2
    return 1
  fi
  if [[ "${release_commit}" != "${actual_commit}" ]]; then
    echo "RELEASE_COMMIT does not match the checked-out Git HEAD (${actual_commit})" >&2
    return 1
  fi

  if ! git -C "${workspace_root}" diff --quiet --ignore-submodules=none -- . "${release_evidence_exclusions[@]}"; then
    echo "formal release refuses modified tracked sources outside the release evidence overlay" >&2
    return 1
  fi
  if ! git -C "${workspace_root}" diff --cached --quiet --ignore-submodules=none -- . "${release_evidence_exclusions[@]}"; then
    echo "formal release refuses staged sources outside the release evidence overlay" >&2
    return 1
  fi
  if ! git -C "${workspace_root}" ls-files -v -z -- . |
    while IFS= read -r -d '' record; do
      marker="${record:0:1}"
      candidate="${record:2}"
      case "${marker}" in
      S | [a-z])
        echo "formal release refuses hidden tracked-source index flags on ${candidate}" >&2
        exit 1
        ;;
      esac
    done; then
    return 1
  fi
  # Deliberately omit --exclude-standard: ignored files can still affect Go
  # package selection, extensionless TypeScript resolution, or go:embed.
  if ! git -C "${workspace_root}" ls-files --others -z -- . |
    while IFS= read -r -d '' candidate; do
      if ! is_allowed_formal_untracked "${candidate}"; then
        echo "formal release refuses untracked source outside the formal build allowlist: ${candidate}" >&2
        exit 1
      fi
    done; then
    return 1
  fi
}

verify_formal_readiness() {
  (
    cd -- "${workspace_root}"
    run_go mod download all
    run_go mod verify
    run_go run ./cmd/release-readiness \
      --release-version "${release_version}" \
      --source-commit "${release_commit}"
  )
}

release_version="${RELEASE_VERSION-}"
release_commit="${RELEASE_COMMIT-}"
release_date="${RELEASE_DATE:-unknown}"

for value in "${release_version}" "${release_commit}" "${release_date}"; do
  if [[ "${value}" =~ [[:space:]] ]]; then
    echo "release metadata must not contain whitespace" >&2
    exit 2
  fi
done

resolved_output_candidate="$(resolve_future_directory "${requested_output_dir}")"
verify_release_output_location "${resolved_output_candidate}"

case "${release_version}" in
dev|ci)
  prepare_development_go_workspace
  if [[ -z "${release_commit}" ]]; then
    release_commit=unknown
  fi
  if ! readiness="$( (cd -- "${workspace_root}" && run_go run ./cmd/release-readiness) 2>&1)"; then
    echo "development release readiness warning: ${readiness}" >&2
  fi
  ;;
*)
  if [[ ${#release_commit} -ne 40 && ${#release_commit} -ne 64 ]] ||
    [[ "${release_commit}" == *[!0-9a-f]* ]] ||
    [[ "${release_commit}" != *[1-9a-f]* ]]; then
    echo "formal release requires RELEASE_COMMIT as a non-zero lowercase 40- or 64-character hexadecimal identifier" >&2
    exit 2
  fi
  verify_formal_source_checkout
  prepare_formal_go_workspace
  verify_formal_readiness
  prepare_formal_web_workspace
  (
    cd -- "${workspace_root}/web"
    run_pnpm install \
      --frozen-lockfile \
      --ignore-scripts \
      --verify-store-integrity \
      --store-dir "${formal_web_backup_root}/pnpm-store"
    run_pnpm run build
  )
  verify_fresh_formal_web_workspace
  verify_formal_source_checkout
  # The evidence overlay is itself embedded by the Go build. Revalidate it
  # after the web build so a changed overlay can never bypass the ledger gate
  # between the first readiness decision and compilation.
  verify_formal_readiness
  ;;
esac

if [[ ! -f "${workspace_root}/web/dist/index.html" ]]; then
  echo "web/dist is missing; run 'cd web && pnpm run build' first" >&2
  exit 1
fi

output_dir="${requested_output_dir}"
mkdir -p -- "${output_dir}"
output_dir="$(cd -- "${output_dir}" && pwd -P)"
verify_release_output_location "${output_dir}"

outputs=(
  sing-box-panel-linux-amd64
  sing-box-panel-linux-arm64
  SHA256SUMS
)
for output in "${outputs[@]}"; do
  if [[ -e "${output_dir}/${output}" ]]; then
    echo "refusing to overwrite ${output_dir}/${output}" >&2
    exit 1
  fi
done

staging_dir="$(mktemp -d "${output_dir}/.release-staging.XXXXXX")"

ldflags=(
  -s
  -w
  "-X=main.version=${release_version}"
  "-X=main.commit=${release_commit}"
  "-X=main.date=${release_date}"
)

for arch in amd64 arm64; do
  target="${staging_dir}/sing-box-panel-linux-${arch}"
  (
    cd -- "${workspace_root}"
    CGO_ENABLED=0 GOOS=linux GOARCH="${arch}" \
      run_go build \
        -tags webdist \
        -trimpath \
        -ldflags="${ldflags[*]}" \
        -o "${target}" \
        ./cmd/sing-box-panel
  )
  chmod 0755 "${target}"
done

(
  cd -- "${staging_dir}"
  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum sing-box-panel-linux-amd64 sing-box-panel-linux-arm64 >SHA256SUMS
  else
    shasum -a 256 sing-box-panel-linux-amd64 sing-box-panel-linux-arm64 >SHA256SUMS
  fi
)

for output in "${outputs[@]}"; do
  mv -- "${staging_dir}/${output}" "${output_dir}/${output}"
done
