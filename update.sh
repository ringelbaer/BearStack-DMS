#!/usr/bin/env bash
set -euo pipefail

repo_dir="${BEARSTACK_REPO_DIR:-$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)}"
service_name="${BEARSTACK_SERVICE:-bearstack.service}"
install_path="${BEARSTACK_INSTALL_PATH:-/usr/local/bin/bearstack}"
artifact_dir=""

cleanup() {
	if [[ -n "$artifact_dir" && -d "$artifact_dir" ]]; then
		rm -rf "$artifact_dir"
	fi
}
trap cleanup EXIT

cd "$repo_dir"
git fetch --all --tags
git pull --ff-only

go test ./...

artifact_dir="$(mktemp -d)"
artifact_path="$artifact_dir/bearstack"
go build -trimpath -ldflags="-s -w" -o "$artifact_path" ./cmd/bearstack

sudo systemctl stop "$service_name"
# systemctl stop waits, but a forced timeout can still leave the unit stopped.
# Replace the binary only after a successful exit with no remaining main PID.
stop_state="$(sudo systemctl show "$service_name" --property=ActiveState --property=Result --property=MainPID)"
active_state=""
stop_result=""
main_pid=""
while IFS='=' read -r property value; do
	case "$property" in
		ActiveState) active_state="$value" ;;
		Result) stop_result="$value" ;;
		MainPID) main_pid="$value" ;;
	esac
done <<< "$stop_state"
if [[ "$active_state" != inactive || "$stop_result" != success || "$main_pid" != 0 ]]; then
	printf 'Update abgebrochen: %s wurde nicht sauber beendet.\n%s\n' "$service_name" "$stop_state" >&2
	exit 1
fi
sudo install -o root -g root -m 0755 "$artifact_path" "$install_path"
sudo systemctl start "$service_name"

sudo systemctl --no-pager --full status "$service_name"
