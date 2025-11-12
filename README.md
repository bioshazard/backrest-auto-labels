# backrest-sidecar

Backrest-sidecar discovers Docker containers labeled with `backrest.*` metadata, renders Backrest plans, and keeps `config.json` up-to-date so the Backrest UI and restic jobs always see the latest workloads. Pair it with the main Backrest container and a shared config volume to automate plan management for Compose stacks.

## Features

- Watches the Docker socket and `/var/lib/docker` to map labeled containers and named volumes to host paths.
- Writes `config.json` atomically (0644, existing ownership) and can restart Backrest when plans change (`--apply`).
- Derives hooks, repo IDs, schedules, retention policies, and plan IDs from labels or sane defaults.
- Supports a `backrest.hooks.template=simple-stop-start` label to auto-stop/start containers around backups.
- Ships helper scripts and sample Compose files for dry runs and production deployments.

## Getting Started

### Build locally
```bash
make build   # outputs ./bin/backrest-sidecar
```

### Run a dry-run against sample labels
```bash
CONFIG_DIR=testdata CONFIG_FILE=example-sidecar.config.json \
  scripts/dry-run-docker.sh --log-format text --log-level debug
```
This uses `testing/compose.dry-run.yaml` to spin up demo containers and prints the plans the sidecar would write.

### Canonical Compose deployment
The repo root includes `compose.yaml` with a Backrest + sidecar stack that shares the same `backrest-config` volume:
```yaml
services:
  backrest:
    image: garethgeorge/backrest:latest
    volumes:
      - backrest-config:/config
  sidecar:
    image: ghcr.io/bioshazard/backrest-auto-labels:latest
    volumes:
      - backrest-config:/etc/backrest
      - /var/run/docker.sock:/var/run/docker.sock:ro
      - /var/lib/docker:/var/lib/docker:ro
    command: ["daemon","--config","/etc/backrest/config.json","--with-events","--apply"]
volumes:
  backrest-config:
```
Customize the tags, env vars, and mounts to match your environment, but always share the config volume between Backrest and the sidecar.

### Labels you care about
| Label | Purpose |
| --- | --- |
| `backrest.enable=true` | opt-in a container |
| `backrest.repo` | override repo id (defaults to first repo in config) |
| `backrest.schedule` | cron schedule (default `0 2 * * *`) |
| `backrest.paths.include` | comma-separated container paths |
| `backrest.keep` | retention spec (default `daily=7,weekly=4`) |
| `backrest.hooks.template` | `simple-stop-start` autogenerates stop/start hooks |

See `docs/design-init.md` for the full matrix.

## CI / Publishing
`.github/workflows/docker-image.yml` builds with Buildx, tags via `docker/metadata-action`, and pushes to GHCR (or just builds on PRs). Set `GHCR` permissions or swap auth to your registry of choice.

## License
MIT (see LICENSE).
