#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-or-later

set -euo pipefail

script_dir="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd -P)"
cd -- "${script_dir}/../../web"

run_phase() {
  local phase="$1" started=${SECONDS} result=0
  shift
  printf '\n[check-web] %s\n' "${phase}"
  "$@" || result=$?
  if ((result != 0)); then
    printf '[check-web] FAIL %s (%ss, exit %s)\n' "${phase}" "$((SECONDS - started))" "${result}" >&2
    exit "${result}"
  fi
  printf '[check-web] PASS %s (%ss)\n' "${phase}" "$((SECONDS - started))"
}

run_phase 'TypeScript' corepack pnpm run typecheck
run_phase 'ESLint' corepack pnpm run lint
run_phase 'Logic tests' corepack pnpm run test:logic
run_phase 'React tests' corepack pnpm run test:react
run_phase 'Production build' corepack pnpm run build
run_phase 'Build contracts' corepack pnpm run test:contracts
run_phase 'Third-party notices' go -C .. tool third-party-notices --check
