#!/usr/bin/env bash
set -euo pipefail

CONFIG=${CONFIG:-testdata/example-sidecar.config.json}
RUN_FLAGS=${RUN_FLAGS:---docker-sock /var/run/docker.sock --docker-root /var/lib/docker --default-repo sample-repo --include-project-name}

if [ ! -f "$CONFIG" ]; then
  echo "Config file '$CONFIG' not found. Override CONFIG=/path/to/config.json" >&2
  exit 1
fi

exec make dry-run CONFIG="$CONFIG" RUN_FLAGS="$RUN_FLAGS" "$@"
