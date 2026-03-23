#!/usr/bin/env bash

set -euo pipefail

source "$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)/common.sh" "$@"

if [[ ! -d "${CONF_DIR}" ]]; then
  echo "no ${CONF_DIR} found; nothing to remove"
  exit 0
fi

rm -rf "${CONF_DIR}"
echo "removed ${CONF_DIR}"
