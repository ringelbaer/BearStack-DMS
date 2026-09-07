#!/usr/bin/env bash
set -euo pipefail
repo_dir="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)"
exec "$repo_dir/apps/android/gradlew" -p "$repo_dir/apps/android" :app:testDebugUnitTest :app:lintDebug :app:assembleDebug "$@"
