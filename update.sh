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
sudo install -o root -g root -m 0755 "$artifact_path" "$install_path"
sudo systemctl start "$service_name"

sudo systemctl --no-pager --full status "$service_name"
