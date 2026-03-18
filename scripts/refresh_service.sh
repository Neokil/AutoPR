#!/usr/bin/env bash

set -euo pipefail

source "$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)/common.sh" "$@"

launchd_label="com.autopr.auto-prd"
systemd_unit="auto-prd.service"
os_name="$(uname -s)"

if [[ "${os_name}" == "Darwin" ]]; then
  exec launchctl kickstart -k "gui/$(id -u)/${launchd_label}"
fi

if [[ "${os_name}" == "Linux" ]] && command -v systemctl >/dev/null 2>&1; then
  exec systemctl --user restart "${systemd_unit}"
fi

echo "service refresh is only supported on macOS launchd or Linux systemd --user"
exit 1
