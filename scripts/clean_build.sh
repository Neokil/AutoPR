#!/usr/bin/env bash

set -euo pipefail

source "$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)/common.sh" "$@"

rm -rf "${REPO_ROOT}/.build"
echo "removed ${REPO_ROOT}/.build"
