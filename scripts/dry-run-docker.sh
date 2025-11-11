#!/usr/bin/env bash
set -euo pipefail

CONFIG=${CONFIG:-testdata/example-sidecar.config.json}
COMPOSE_FILE=${COMPOSE_FILE:-compose.dry-run.yaml}
PROJECT_NAME=${PROJECT_NAME:-backrest-sidecar}
HOST_CONFIG=$(realpath "$CONFIG")
DEFAULT_CMD=(reconcile --dry-run --config /etc/backrest/config.json --docker-sock /var/run/docker.sock --docker-root /var/lib/docker --default-repo sample-repo --include-project-name)

if [ ! -f "$HOST_CONFIG" ]; then
  echo "Config file '$HOST_CONFIG' not found. Override CONFIG=/path/to/config.json" >&2
  exit 1
fi

if [ "$#" -eq 0 ]; then
  set -- "${DEFAULT_CMD[@]}"
fi

CONFIG_PATH="$HOST_CONFIG" PROJECT_NAME="$PROJECT_NAME" docker compose -f "$COMPOSE_FILE" run --build --rm sidecar "$@"
