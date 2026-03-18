#!/usr/bin/env bash

set -euo pipefail

source "$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)/common.sh" "$@"

launchd_label="com.autopr.auto-prd"
systemd_unit="auto-prd.service"
os_name="$(uname -s)"

if [[ "${os_name}" == "Darwin" ]]; then
  plist_path="${HOME}/Library/LaunchAgents/${launchd_label}.plist"

  launchctl bootout "gui/$(id -u)/${launchd_label}" >/dev/null 2>&1 || true
  rm -f "${plist_path}"

  echo "removed launchd service ${plist_path}"
  exit 0
fi

if [[ "${os_name}" == "Linux" ]] && command -v systemctl >/dev/null 2>&1; then
  unit_path="${HOME}/.config/systemd/user/${systemd_unit}"

  systemctl --user disable --now "${systemd_unit}" >/dev/null 2>&1 || true
  rm -f "${unit_path}"
  systemctl --user daemon-reload >/dev/null 2>&1 || true

  echo "removed systemd user service ${unit_path}"
  exit 0
fi

echo "skipped background service removal (supported: macOS launchd, Linux systemd --user)"
