#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-or-later

# Exercise the production smoke scenario against the current working tree.
# These disposable binaries trust only a newly generated test key.
set -Eeuo pipefail
trap 'printf "release contract failed at line %s (status %s)\n" "${LINENO}" "$?" >&2' ERR

if [[ "$(uname -s)" != Linux || "${EUID}" -eq 0 ]]; then
  printf 'release contracts require native Linux and a non-root user\n' >&2
  exit 1
fi
case "$(uname -m)" in
x86_64) architecture=amd64 ;;
aarch64) architecture=arm64 ;;
*) printf 'release contracts require amd64 or arm64\n' >&2; exit 1 ;;
esac
if [[ "${RELEASE_ARCHITECTURE:-${architecture}}" != "${architecture}" ]]; then
  printf 'release contract runner does not match RELEASE_ARCHITECTURE\n' >&2
  exit 1
fi
for required_command in curl git go jq openssl python3 sha256sum; do
  command -v "${required_command}" >/dev/null 2>&1 || {
    printf '%s is required for release contracts\n' "${required_command}" >&2
    exit 1
  }
done

script_dir="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd -P)"
workspace_root="$(cd -- "${script_dir}/../.." && pwd -P)"
cd -- "${workspace_root}"
if [[ ! -s web/dist/index.html ]]; then
  printf 'build web/dist before running release contracts (make release-smoke)\n' >&2
  exit 1
fi
source_commit="$(git rev-parse --verify 'HEAD^{commit}')"
source_date="$(git show -s --format=%cI "${source_commit}")"
test_root="$(mktemp -d "${RUNNER_TEMP:-/tmp}/sbp-release-contract.XXXXXX")"
trap 'rm -rf -- "${test_root}"' EXIT
trap 'exit 130' INT
trap 'exit 143' HUP TERM
release_dir="${test_root}/release"
mkdir -p -- "${release_dir}"
umask 077
openssl genpkey -algorithm ED25519 -out "${test_root}/private.pem"
go tool sign-release public-key --private-key "${test_root}/private.pem" >"${test_root}/public-key"
public_key="$(<"${test_root}/public-key")"
version=v0.0.1-smoke
ldflags="-s -w -X=github.com/rehuony/sing-box-panel/internal/buildinfo.version=${version} -X=github.com/rehuony/sing-box-panel/internal/buildinfo.commit=${source_commit} -X=github.com/rehuony/sing-box-panel/internal/buildinfo.date=${source_date} -X=github.com/rehuony/sing-box-panel/internal/selfupdate.embeddedPublicKey=${public_key}"
for target_architecture in amd64 arm64; do
  printf '[release contract] build linux/%s test candidate\n' "${target_architecture}"
  env CGO_ENABLED=0 GOOS=linux GOARCH="${target_architecture}" \
    GOAMD64=v1 GOARM64=v8.0 GOENV=off GOEXPERIMENT= GOFIPS140=off \
    GOFLAGS=-mod=readonly GOTOOLCHAIN=local GOWORK=off \
    go build -buildvcs=false -trimpath -ldflags="${ldflags}" \
      -o "${release_dir}/sing-box-panel-linux-${target_architecture}" ./cmd/sing-box-panel
done
(
  cd -- "${release_dir}"
  sha256sum sing-box-panel-linux-amd64 sing-box-panel-linux-arm64 >SHA256SUMS
)
go tool sign-release sign \
  --private-key "${test_root}/private.pem" --public-key "${test_root}/public-key" \
  --version "${version}" --checksums "${release_dir}/SHA256SUMS" \
  --signature "${release_dir}/SHA256SUMS.sig"
bash "${script_dir}/smoke-release.sh" \
  --release-dir "${release_dir}" --version "${version}" \
  --source-commit "${source_commit}" --architecture "${architecture}" \
  --public-key "${test_root}/public-key"
