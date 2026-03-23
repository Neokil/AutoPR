#!/usr/bin/env bash

set -euo pipefail

SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="${1:-$(cd -- "$SCRIPT_DIR/.." && pwd)}"

CONF_DIR="${HOME}/.auto-pr"
PROMPTS_DIR="${CONF_DIR}/prompts"
SERVER_DIR="${CONF_DIR}/server"
LOG_DIR="${SERVER_DIR}/logs"
ZSHRC="${HOME}/.zshrc"

readonly SCRIPT_DIR
readonly REPO_ROOT
readonly CONF_DIR
readonly PROMPTS_DIR
readonly SERVER_DIR
readonly LOG_DIR
readonly ZSHRC
