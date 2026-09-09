#!/usr/bin/env bash
set -euo pipefail
repo_dir="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)"
variant=Debug
if [[ "${1:-}" == --release ]]; then variant=Release; shift; fi
exec "$repo_dir/apps/android/gradlew" -p "$repo_dir/apps/android" ":app:test${variant}UnitTest" ":app:lint${variant}" ":app:assemble${variant}" "$@"
