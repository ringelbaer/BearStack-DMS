#!/usr/bin/env bash
# Disposable mount only; never remount the user's photo directory.
set -euo pipefail
repo_dir="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$repo_dir"
go_bin="${GO:-go}"
run_test() {
  BEARSTACK_READONLY_TEST_ROOT="$1" "$go_bin" test ./internal/photos -run '^TestPhotoIdentityReadonlyMount$' -count=1 -v
}
if [[ -n "${BEARSTACK_READONLY_TEST_ROOT:-}" ]]; then
  run_test "$BEARSTACK_READONLY_TEST_ROOT"
  exit
fi
fixture_dir="$(mktemp -d "${TMPDIR:-/tmp}/bearstack-readonly.XXXXXXXX")"
fixture_dir="$(cd "$fixture_dir" && pwd -P)"
mounted=false
cleanup() {
  if [[ "$mounted" == true ]]; then
    # Never remove the mount point's contents if detach fails.
    if ! hdiutil detach "$fixture_dir/root" -quiet; then
      echo "Could not detach disposable mount: $fixture_dir/root" >&2
      return 1
    fi
  fi
  rm -rf -- "$fixture_dir"
}
trap cleanup EXIT
mkdir "$fixture_dir/source" "$fixture_dir/root"
cp _site-src/docs/assets/images/bearstack-app-screenshot.webp "$fixture_dir/source/photo.webp"
printf '# Read-only fixture\n' > "$fixture_dir/source/blog.md"
case "$(uname -s)" in
  Darwin)
    hdiutil create -quiet -srcfolder "$fixture_dir/source" -fs APFS -format UDRO "$fixture_dir/photos.dmg"
    hdiutil attach -quiet -readonly -nobrowse -mountpoint "$fixture_dir/root" "$fixture_dir/photos.dmg"
    mounted=true
    run_test "$fixture_dir/root"
    ;;
  Linux)
    # The bind mount exists only in this child mount namespace. A missing
    # user-namespace capability is an actionable failure, not a passed test.
    unshare --user --map-root-user --mount bash -c '
      set -euo pipefail
      mount --bind "$1" "$2"
      mount -o remount,bind,ro "$2"
      BEARSTACK_READONLY_TEST_ROOT="$2" "$3" test ./internal/photos -run "^TestPhotoIdentityReadonlyMount$" -count=1 -v
    ' _ "$fixture_dir/source" "$fixture_dir/root" "$go_bin"
    ;;
  *)
    echo "Provide BEARSTACK_READONLY_TEST_ROOT with a disposable read-only mount on this platform." >&2
    exit 1
    ;;
esac
