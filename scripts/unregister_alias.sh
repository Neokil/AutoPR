#!/usr/bin/env bash

set -euo pipefail

source "$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)/common.sh" "$@"

start_marker="# >>> auto-pr build-path >>>"
end_marker="# <<< auto-pr build-path <<<"
tmp_file="$(mktemp)"

cleanup() {
  rm -f "${tmp_file}"
}

trap cleanup EXIT

if [[ ! -f "${ZSHRC}" ]]; then
  echo "no ${ZSHRC} found; nothing to remove"
  exit 0
fi

awk -v start="${start_marker}" -v end="${end_marker}" '
  BEGIN { skip = 0 }
  $0 == start { skip = 1; next }
  $0 == end { skip = 0; next }
  !skip { print }
' "${ZSHRC}" > "${tmp_file}"

mv "${tmp_file}" "${ZSHRC}"
trap - EXIT

echo "removed auto-pr PATH block from ${ZSHRC}"
