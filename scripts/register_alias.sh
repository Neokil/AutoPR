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

if [[ -f "${ZSHRC}" ]]; then
  awk -v start="${start_marker}" -v end="${end_marker}" '
    BEGIN { skip = 0 }
    $0 == start { skip = 1; next }
    $0 == end { skip = 0; next }
    !skip { print }
  ' "${ZSHRC}" > "${tmp_file}"
else
  : > "${tmp_file}"
fi

{
  cat "${tmp_file}"
  echo
  echo "${start_marker}"
  echo "export PATH=\"${REPO_ROOT}/.build:\$PATH\""
  echo "${end_marker}"
} > "${ZSHRC}"

echo "updated PATH in ${ZSHRC}"
