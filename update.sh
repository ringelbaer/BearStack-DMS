#!/usr/bin/env bash
set -euo pipefail

repo_dir="${BEARSTACK_REPO_DIR:-$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)}"
service_name="${BEARSTACK_SERVICE:-bearstack.service}"
install_path="${BEARSTACK_INSTALL_PATH:-/usr/local/bin/bearstack}"
remote_name="${BEARSTACK_GIT_REMOTE:-origin}"
update_branch="${BEARSTACK_UPDATE_BRANCH:-main}"
artifact_dir=""

usage() {
	cat <<'USAGE'
Usage: ./update.sh [--alpha|--branch BRANCH]

Options:
  --alpha          Update from branch Alpha.
  --branch BRANCH Update from the selected branch instead of main.
  -h, --help      Show this help.
USAGE
}

while [[ $# -gt 0 ]]; do
	case "$1" in
		--alpha)
			update_branch="Alpha"
			shift
			;;
		--branch)
			if [[ $# -lt 2 || -z "$2" ]]; then
				echo "Missing branch name for --branch" >&2
				usage >&2
				exit 2
			fi
			update_branch="$2"
			shift 2
			;;
		--branch=*)
			update_branch="${1#--branch=}"
			if [[ -z "$update_branch" ]]; then
				echo "Missing branch name for --branch" >&2
				usage >&2
				exit 2
			fi
			shift
			;;
		-h|--help)
			usage
			exit 0
			;;
		*)
			echo "Unknown option: $1" >&2
			usage >&2
			exit 2
			;;
	esac
done

cleanup() {
	if [[ -n "$artifact_dir" && -d "$artifact_dir" ]]; then
		rm -rf "$artifact_dir"
	fi
}
trap cleanup EXIT

cd "$repo_dir"
if ! git check-ref-format --branch "$update_branch" >/dev/null; then
	echo "Invalid branch name: $update_branch" >&2
	exit 2
fi

git fetch --all --tags
if git show-ref --verify --quiet "refs/heads/$update_branch"; then
	git switch "$update_branch"
elif git show-ref --verify --quiet "refs/remotes/$remote_name/$update_branch"; then
	git switch --track -c "$update_branch" "$remote_name/$update_branch"
else
	echo "Branch '$update_branch' not found locally or on remote '$remote_name'." >&2
	exit 1
fi
git pull --ff-only "$remote_name" "$update_branch"

go test ./...

artifact_dir="$(mktemp -d)"
artifact_path="$artifact_dir/bearstack"
go build -trimpath -ldflags="-s -w" -o "$artifact_path" ./cmd/bearstack

sudo systemctl stop "$service_name"
sudo install -o root -g root -m 0755 "$artifact_path" "$install_path"
sudo systemctl start "$service_name"

sudo systemctl --no-pager --full status "$service_name"
