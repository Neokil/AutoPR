#!/usr/bin/env bash

set -euo pipefail

source "$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)/common.sh" "$@"

mkdir -p "${CONF_DIR}" "${PROMPTS_DIR}" "${SERVER_DIR}" "${LOG_DIR}"

if [[ ! -f "${CONF_DIR}/config.yaml" ]]; then
  cp "${REPO_ROOT}/config.example.yaml" "${CONF_DIR}/config.yaml"
  echo "scaffolded ${CONF_DIR}/config.yaml"
else
  echo "kept existing ${CONF_DIR}/config.yaml"
fi

for prompt in ticket.md.tmpl investigate.md.tmpl implement.md.tmpl pr.md.tmpl; do
  if [[ ! -f "${PROMPTS_DIR}/${prompt}" ]]; then
    cp "${REPO_ROOT}/internal/providers/prompts/${prompt}" "${PROMPTS_DIR}/${prompt}"
    echo "scaffolded ${PROMPTS_DIR}/${prompt}"
  else
    echo "kept existing ${PROMPTS_DIR}/${prompt}"
  fi
done

echo "server logs: ${LOG_DIR}"
