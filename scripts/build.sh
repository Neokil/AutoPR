#!/usr/bin/env bash

set -euo pipefail

source "$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)/common.sh" "$@"

mkdir -p "${REPO_ROOT}/.build"

if [[ -f "${REPO_ROOT}/web/package.json" ]]; then
  echo "building frontend (web/dist)"
  (
    cd "${REPO_ROOT}/web"
    npm install
    npm run build
  )
fi

go build -o "${REPO_ROOT}/.build/auto-pr" "${REPO_ROOT}/cmd/auto-pr"
go build -o "${REPO_ROOT}/.build/auto-prd" "${REPO_ROOT}/cmd/auto-prd"

echo "built ${REPO_ROOT}/.build/auto-pr and ${REPO_ROOT}/.build/auto-prd"
