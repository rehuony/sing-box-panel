#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-or-later

set -euo pipefail

script_dir="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd -P)"
notes="${script_dir}/../release-notes.sh"

test_root="$(mktemp -d "${TMPDIR:-/tmp}/sing-box-panel-release-notes-test.XXXXXX")"
trap 'rm -rf -- "${test_root}"' EXIT HUP INT TERM

fail() {
  printf 'FAIL: %s\n' "$*" >&2
  exit 1
}

commit=0123456789abcdef0123456789abcdef01234567
release="${test_root}/release"
mkdir -p -- "${release}"
# A binary of 3 MiB and the others of one byte more each, so every row's size
# and digest are its own.
size=$((3 << 20))
for binary in sing-box-panel-linux-amd64 sing-box-panel-linux-arm64 sing-box-node-linux-amd64 sing-box-node-linux-arm64; do
  head -c "${size}" /dev/zero >"${release}/${binary}"
  size=$((size + 1))
  digest="$(openssl dgst -sha256 -r "${release}/${binary}" | cut -d' ' -f1)"
  printf '%s  %s\n' "${digest}" "${binary}" >>"${release}/SHA256SUMS"
done
printf 'signature\n' >"${release}/SHA256SUMS.sig"

arguments=(--version v1.2.3 --commit "${commit}" --protocol 1.3 --repository owner/project)
output="$(bash "${notes}" "${arguments[@]}" "${release}")" || fail "notes of a complete release failed"
while read -r digest binary; do
  [[ "${output}" == *"| \`${binary}\` | "*" | 3.0 MiB | \`${digest}\` |"* ]] || fail "no row with the digest of ${binary}"
done <"${release}/SHA256SUMS"
for expected in \
  '| `SHA256SUMS.sig` | Ed25519 signature over the version and `SHA256SUMS` | 10 B | — |' \
  "[\`0123456789ab\`](https://github.com/owner/project/commit/${commit})" \
  'speaks panel–node protocol 1.3' \
  'owner/project/main/scripts/installer.sh | sudo bash -s -- --component node --version v1.2.3' \
  'blob/v1.2.3/.github/keypair/release-signing-public-key' \
  '--version v1.2.3 --checksums SHA256SUMS --signature SHA256SUMS.sig'; do
  [[ "${output}" == *"${expected}"* ]] || fail "notes omit ${expected}"
done

# A missing asset, a digest SHA256SUMS lacks or a malformed version print no
# notes.
rm -- "${release}/SHA256SUMS.sig"
if bash "${notes}" "${arguments[@]}" "${release}" >/dev/null 2>&1; then
  fail "notes without the signature succeeded"
fi
printf 'signature\n' >"${release}/SHA256SUMS.sig"
grep -v sing-box-node-linux-arm64 "${release}/SHA256SUMS" >"${test_root}/sums"
mv -- "${test_root}/sums" "${release}/SHA256SUMS"
if bash "${notes}" "${arguments[@]}" "${release}" >/dev/null 2>&1; then
  fail "notes without a binary's digest succeeded"
fi
if bash "${notes}" --version 1.2.3 --commit "${commit}" --protocol 1.3 --repository owner/project "${release}" >/dev/null 2>&1; then
  fail "notes of a version without its v succeeded"
fi

printf 'release notes tests passed\n'
