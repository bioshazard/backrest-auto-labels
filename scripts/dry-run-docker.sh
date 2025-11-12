#!/usr/bin/env bash
set -euo pipefail

CONFIG_DIR=${CONFIG_DIR:-testdata}
CONFIG_FILE=${CONFIG_FILE:-example-sidecar.config.json}
COMPOSE_FILE=${COMPOSE_FILE:-testing/compose.dry-run.yaml}
PROJECT_NAME=${PROJECT_NAME:-backrest-sidecar}
HOST_CONFIG_DIR=$(realpath "$CONFIG_DIR")

if [ ! -d "$HOST_CONFIG_DIR" ]; then
  echo "Config directory '$HOST_CONFIG_DIR' not found. Override CONFIG_DIR=/path/to/dir" >&2
  exit 1
fi

if [ ! -f "$HOST_CONFIG_DIR/$CONFIG_FILE" ]; then
  echo "Config file '$HOST_CONFIG_DIR/$CONFIG_FILE' not found. Override CONFIG_FILE=<name>" >&2
  exit 1
fi

DEFAULT_CMD=(reconcile --dry-run --config "/etc/backrest/$CONFIG_FILE" --docker-sock /var/run/docker.sock --docker-root /var/lib/docker --default-repo sample-repo --include-project-name --log-format text --log-level debug)

if [ "$#" -eq 0 ]; then
  set -- "${DEFAULT_CMD[@]}"
fi

echo "CONFIG_DIR=$HOST_CONFIG_DIR CONFIG_FILE=$CONFIG_FILE"
echo "@ $@"
CONFIG_DIR="$HOST_CONFIG_DIR" CONFIG_FILE="$CONFIG_FILE" COMPOSE_PROJECT_NAME="$PROJECT_NAME" docker compose -p "$PROJECT_NAME" -f "$COMPOSE_FILE" run --build --rm sidecar "$@"
