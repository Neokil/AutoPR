#!/usr/bin/env bash

set -euo pipefail

source "$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)/common.sh" "$@"

launchd_label="com.autopr.auto-prd"
systemd_unit="auto-prd.service"
os_name="$(uname -s)"

mkdir -p "${SERVER_DIR}" "${LOG_DIR}"

if [[ "${os_name}" == "Darwin" ]]; then
  launch_dir="${HOME}/Library/LaunchAgents"
  plist_path="${launch_dir}/${launchd_label}.plist"

  mkdir -p "${launch_dir}"

  printf '%s\n' \
    '<?xml version="1.0" encoding="UTF-8"?>' \
    '<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">' \
    '<plist version="1.0">' \
    '<dict>' \
    '  <key>Label</key>' \
    "  <string>${launchd_label}</string>" \
    '  <key>ProgramArguments</key>' \
    '  <array>' \
    "    <string>${REPO_ROOT}/.build/auto-prd</string>" \
    '  </array>' \
    '  <key>RunAtLoad</key>' \
    '  <true/>' \
    '  <key>KeepAlive</key>' \
    '  <true/>' \
    '  <key>WorkingDirectory</key>' \
    "  <string>${REPO_ROOT}</string>" \
    '  <key>StandardOutPath</key>' \
    "  <string>${LOG_DIR}/stdout.log</string>" \
    '  <key>StandardErrorPath</key>' \
    "  <string>${LOG_DIR}/stderr.log</string>" \
    '</dict>' \
    '</plist>' > "${plist_path}"

  launchctl bootout "gui/$(id -u)/${launchd_label}" >/dev/null 2>&1 || true
  launchctl bootstrap "gui/$(id -u)" "${plist_path}"
  launchctl enable "gui/$(id -u)/${launchd_label}" >/dev/null 2>&1 || true

  echo "installed and started launchd service ${plist_path}"
  exit 0
fi

if [[ "${os_name}" == "Linux" ]] && command -v systemctl >/dev/null 2>&1; then
  systemd_dir="${HOME}/.config/systemd/user"
  unit_path="${systemd_dir}/${systemd_unit}"

  mkdir -p "${systemd_dir}"

  printf '%s\n' \
    '[Unit]' \
    'Description=AutoPR daemon' \
    'After=default.target' \
    '' \
    '[Service]' \
    'Type=simple' \
    "WorkingDirectory=${REPO_ROOT}" \
    "ExecStart=${REPO_ROOT}/.build/auto-prd" \
    'Restart=always' \
    'RestartSec=5' \
    "StandardOutput=append:${LOG_DIR}/stdout.log" \
    "StandardError=append:${LOG_DIR}/stderr.log" \
    '' \
    '[Install]' \
    'WantedBy=default.target' > "${unit_path}"

  if systemctl --user daemon-reload >/dev/null 2>&1; then
    systemctl --user enable --now "${systemd_unit}" >/dev/null 2>&1 || systemctl --user restart "${systemd_unit}" >/dev/null 2>&1
    echo "installed and started systemd user service ${unit_path}"
  else
    echo "installed ${unit_path}, but could not reach the user systemd instance"
    echo "run: systemctl --user daemon-reload && systemctl --user enable --now ${systemd_unit}"
  fi
  exit 0
fi

echo "skipped background service install (supported: macOS launchd, Linux systemd --user)"
