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

EXAMPLE_PROVIDER_DIR="${PROMPTS_DIR}/example-provider"
mkdir -p "${EXAMPLE_PROVIDER_DIR}"
for prompt in ticket.md.tmpl investigate.md.tmpl implement.md.tmpl pr.md.tmpl; do
  if [[ ! -f "${EXAMPLE_PROVIDER_DIR}/${prompt}" ]]; then
    cp "${REPO_ROOT}/internal/providers/prompts/${prompt}" "${EXAMPLE_PROVIDER_DIR}/${prompt}"
    echo "scaffolded ${EXAMPLE_PROVIDER_DIR}/${prompt}"
  else
    echo "kept existing ${EXAMPLE_PROVIDER_DIR}/${prompt}"
  fi
done

echo "server logs: ${LOG_DIR}"
