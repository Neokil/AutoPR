#!/usr/bin/env bash

set -euo pipefail

source "$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)/common.sh" "$@"

stdout_log="${LOG_DIR}/stdout.log"
stderr_log="${LOG_DIR}/stderr.log"

mkdir -p "${LOG_DIR}"
touch "${stdout_log}" "${stderr_log}"

echo "showing logs from ${LOG_DIR}"
exec tail -n 100 -f "${stdout_log}" "${stderr_log}"
