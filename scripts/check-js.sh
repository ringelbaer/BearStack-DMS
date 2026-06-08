#!/usr/bin/env bash
set -euo pipefail

repo_dir="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)"
node_bin="${NODE:-node}"

if ! command -v "$node_bin" >/dev/null 2>&1; then
	echo "node is required for browser script syntax checks" >&2
	exit 127
fi

cd "$repo_dir"
find internal/server/static -maxdepth 1 -type f -name '*.js' -print | sort | while IFS= read -r file; do
	echo "node --check $file"
	"$node_bin" --check "$file"
done

echo "node scripts/js-dom-tests.mjs"
"$node_bin" scripts/js-dom-tests.mjs
