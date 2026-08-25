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

script_dir="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
workspace_root="$(cd -- "${script_dir}/.." && pwd)"
output_dir="$1"

if ! source_commit="$(git -C "${workspace_root}" rev-parse --verify 'HEAD^{commit}')"; then
  echo "cannot resolve the checked-out Git HEAD for formal release verification" >&2
  exit 1
fi

readiness_output="$(mktemp)"
readiness_error="$(mktemp)"
readiness_value_output="$(mktemp)"
blocked_output="$(mktemp -d)"
cleanup() {
  rm -f -- "${readiness_output}" "${readiness_error}" "${readiness_value_output}"
  rm -rf -- "${blocked_output}"
}
trap cleanup EXIT

set +e
(
  cd -- "${workspace_root}"
  go run ./cmd/release-readiness --ready-output "${readiness_value_output}"
) >"${readiness_output}" 2>"${readiness_error}"
readiness_status=$?
set -e
cat "${readiness_output}"
cat "${readiness_error}" >&2

readiness_value="$(<"${readiness_value_output}")"
if [[ "${readiness_value}" != true && "${readiness_value}" != false ]]; then
  echo "release-readiness did not emit its machine-readable status" >&2
  exit 1
fi

if [[ ${readiness_status} -eq 0 ]]; then
  if [[ "${readiness_value}" != true ]]; then
    echo "release-readiness exited successfully without reporting ready=true" >&2
    exit 1
  fi
  echo "release readiness passed; verifying the formal release path"
  RELEASE_VERSION=v0.0.0 RELEASE_COMMIT="${source_commit}" \
    "${script_dir}/build-release.sh" "${output_dir}"
  exit 0
fi

if [[ "${readiness_value}" != false ]]; then
  echo "release-readiness failed without reporting ready=false" >&2
  exit 1
fi

echo "release readiness is not yet satisfied; verifying that a formal release is blocked"
if RELEASE_VERSION=v0.0.0 RELEASE_COMMIT="${source_commit}" \
  "${script_dir}/build-release.sh" "${blocked_output}"; then
  echo "formal release unexpectedly bypassed the release-readiness gate" >&2
  exit 1
fi
if find "${blocked_output}" -mindepth 1 -print -quit | grep -q .; then
  echo "blocked formal release left partial output behind" >&2
  find "${blocked_output}" -mindepth 1 -maxdepth 1 -print >&2
  exit 1
fi

echo "formal release was blocked as expected; verifying the development release path"
RELEASE_VERSION=ci "${script_dir}/build-release.sh" "${output_dir}"
