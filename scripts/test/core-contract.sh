#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-or-later

set -euo pipefail

repository_root="$(git rev-parse --show-toplevel)"
catalog="${repository_root}/internal/singbox/catalog.json"
architecture="${CORE_CONTRACT_ARCHITECTURE:-$(go env GOARCH)}"

if [[ "$(go env GOOS)" != "linux" ]]; then
  printf 'core contract requires a native Linux host\n' >&2
  exit 2
fi
if [[ "${architecture}" != "amd64" && "${architecture}" != "arm64" ]]; then
  printf 'unsupported core contract architecture: %s\n' "${architecture}" >&2
  exit 2
fi
for command in curl jq sha256sum stat tar; do
  if ! command -v "${command}" >/dev/null 2>&1; then
    printf 'core contract requires %s\n' "${command}" >&2
    exit 2
  fi
done

work_directory="$(mktemp -d "${TMPDIR:-/tmp}/sing-box-core-contract.XXXXXX")"
cleanup() {
  rm -rf -- "${work_directory}"
}
trap cleanup EXIT

contract_test="${work_directory}/adapter-contract.test"
(
  cd -- "${repository_root}"
  CGO_ENABLED=0 go test -c -o "${contract_test}" ./internal/singbox
)

while IFS=$'\t' read -r version asset_name url expected_sha256 expected_size; do
  case_directory="${work_directory}/${version}"
  archive="${case_directory}/${asset_name}"
  member="sing-box-${version}-linux-${architecture}/sing-box"
  mkdir -- "${case_directory}"

  curl --fail --location --retry 3 --silent --show-error \
    --output "${archive}" \
    "${url}"
  actual_size="$(stat -c '%s' "${archive}")"
  if [[ "${actual_size}" != "${expected_size}" ]]; then
    printf 'archive size mismatch for sing-box %s linux/%s: got %s, want %s\n' \
      "${version}" "${architecture}" "${actual_size}" "${expected_size}" >&2
    exit 1
  fi
  printf '%s  %s\n' "${expected_sha256}" "${archive}" | sha256sum --check --strict -
  tar --extract --gzip --file "${archive}" --directory "${case_directory}" "${member}"
  binary="${case_directory}/${member}"
  chmod 0755 "${binary}"

  SING_BOX_CONTRACT_REQUIRED=1 \
  SING_BOX_CONTRACT_BINARY="${binary}" \
  SING_BOX_CONTRACT_VERSION="${version}" \
  SING_BOX_CONTRACT_ARCHITECTURE="${architecture}" \
    "${contract_test}" \
      -test.run '^TestCompiledAdaptersAcceptExactOfficialBinary$' \
      -test.count=1
done < <(
  jq -r --arg architecture "${architecture}" '
    .versions[]
    | . as $version
    | .profiles[$architecture]
    | [$version.version, .asset_name, .url, .sha256, (.size | tostring)]
    | @tsv
  ' "${catalog}"
)
