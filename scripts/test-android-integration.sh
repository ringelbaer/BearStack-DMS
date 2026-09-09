#!/usr/bin/env bash
# Requires one booted emulator. Uses temporary Go fixtures, not an existing installation.
set -euo pipefail
repo_dir="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$repo_dir"
release_smoke=false
if [[ "${1:-}" == --release ]]; then
  release_smoke=true
  shift
fi
fixture_dir="$(mktemp -d)"
fixture_pid=""
cleanup() {
  touch "$fixture_dir/stop"
  if [[ -n "$fixture_pid" ]]; then wait "$fixture_pid" || true; fi
  adb reverse --remove tcp:18787 >/dev/null 2>&1 || true
  rm -rf -- "$fixture_dir"
}
trap cleanup EXIT
BEARSTACK_ANDROID_FIXTURE_STOP="$fixture_dir/stop" go test -v ./internal/server -run '^TestAndroidLabelingFixture$' -count=1 >"$fixture_dir/server.log" 2>&1 &
fixture_pid=$!
for ((attempt=0;attempt<120;attempt++)); do
  if rg -q 'HTTPS fixture ready' "$fixture_dir/server.log"; then break; fi
  if ! kill -0 "$fixture_pid" 2>/dev/null; then cat "$fixture_dir/server.log"; exit 1; fi
  sleep 1
done
if ! rg -q 'HTTPS fixture ready' "$fixture_dir/server.log"; then cat "$fixture_dir/server.log"; exit 1; fi
adb reverse tcp:18787 tcp:18787
if [[ "$release_smoke" == true ]]; then
  "$repo_dir/scripts/check-android.sh" --release -Pbearstack.releaseSmoke=true "$@"
  "${PYTHON:-python3}" "$repo_dir/scripts/android-release-smoke.py" https://127.0.0.1:18787/gallery/
else
  "$repo_dir/apps/android/gradlew" -p "$repo_dir/apps/android" :app:connectedDebugAndroidTest \
    -Pandroid.testInstrumentationRunnerArguments.labelingUrl=https://127.0.0.1:18787/ "$@"
  "${PYTHON:-python3}" "$repo_dir/scripts/check-android-results.py" \
    "$repo_dir/apps/android/app/build/outputs/androidTest-results/connected/debug"
fi
