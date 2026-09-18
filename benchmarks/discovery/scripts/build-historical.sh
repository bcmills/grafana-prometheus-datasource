#!/usr/bin/env bash
set -euo pipefail

# Build historical TSDB blocks from an OpenMetrics fixture.
# Usage: scripts/build-historical.sh benchmarks/discovery/out/10k-mixed-historical

PROFILE_DIR=${1:?path to a generated profile directory that contains historical.om}

OM="${PROFILE_DIR}/historical.om"
TSDB="${PROFILE_DIR}/tsdb"

if [[ ! -f "${OM}" ]]; then
  echo "missing ${OM}" >&2
  exit 1
fi

if ! command -v promtool >/dev/null 2>&1; then
  echo "promtool is required. Install Prometheus and add it to PATH." >&2
  exit 1
fi

mkdir -p "${TSDB}"
promtool tsdb create-blocks-from openmetrics "${OM}" "${TSDB}"
echo "wrote TSDB blocks to ${TSDB}"
