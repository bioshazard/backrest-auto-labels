#!/usr/bin/env bash
set -euo pipefail

CONFIG=${CONFIG:-testdata/example-sidecar.config.json}
BINARY=${BINARY:-./bin/backrest-sidecar}
DEFAULT_FLAGS=(--docker-sock /var/run/docker.sock --docker-root /var/lib/docker --default-repo sample-repo --include-project-name)

if [ ! -f "$CONFIG" ]; then
  echo "Config file '$CONFIG' not found. Override CONFIG=/path/to/config.json" >&2
  exit 1
fi

if [ ! -x "$BINARY" ]; then
  echo "Binary '$BINARY' missing; running make build..." >&2
  make build
fi

if [ "$#" -eq 0 ]; then
  set -- "${DEFAULT_FLAGS[@]}"
fi

BACKREST_CONFIG="$CONFIG" "$BINARY" reconcile --dry-run "$@"
