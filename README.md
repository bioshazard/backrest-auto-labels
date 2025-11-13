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
This uses `testing/compose.dry-run.yaml` (demo echo workloads + bundled sidecar) and runs the sidecar container with your config bind-mounted, so you see exactly what would be written.

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

Need the sidecar to only back up the config directory? Add the minimal labels below to the `backrest` service so it opts in and constrains paths explicitly:

```yaml
services:
  backrest:
    labels:
      backrest.enable: "true"
      backrest.paths.include: "/config"
```

When `backrest.paths.include` matches a container mount, the sidecar rewrites it to the host-side path (for example `/var/lib/docker/volumes/backrest-config/_data` by default), so you capture just the config while ignoring other mounts.

Need cron jitter inside a window? Set the minute field to `T` (e.g. `backrest.schedule=T 3 * * *`). The sidecar hashes the rendered plan ID to a deterministic minute between `0-59`, so each workload keeps a consistent-but-spread start time without overlapping exactly on the hour.

### Override the default repo fallback

If your Backrest config defines repos with IDs other than `sample-repo`/`default`, set `--default-repo` (or pass it through `RUN_FLAGS`) so unlabeled containers land on a real repo. You can also export `BACKREST_DEFAULT_REPO=my-repo` to make that the default for every command. When neither flag nor env var is provided, the sidecar now falls back to the repo referenced by the first plan in `/etc/backrest/config.json` (or, if there are no plans yet, the first repo entry) so the warning below only appears when *nothing* in the config references a repo ID.

```yaml
services:
  sidecar:
    image: ghcr.io/bioshazard/backrest-auto-labels:latest
    command: [
      "daemon",
      "--config","/etc/backrest/config.json",
      "--default-repo","prod-repo",
      "--with-events",
      "--apply"
    ]
```

For ad-hoc runs (or `scripts/dry-run-*.sh`), append `--default-repo prod-repo` (or rely on `BACKREST_DEFAULT_REPO`) to the CLI invocation:

```bash
CONFIG=/etc/backrest/config.json ./bin/backrest-sidecar \
  reconcile --dry-run --config "$CONFIG" --docker-root /var/lib/docker \
  --default-repo prod-repo --include-project-name
```

Keep the repo ID in sync with whatever `repos[].id` entry Backrest already knows about; `backrest.repo=<id>` labels still win on containers that need a different target repo.

> Note: the sidecar never rewrites `repos[]`; it simply carries forward whatever repo JSON Backrest already owns (including fields like `guid`/`auto_initialize`). Make sure Backrest itself initializes the repos before letting the sidecar render plans that point at them.

### Manage the compose stack

Use the provided Make target to run Docker Compose with a stable project name (`backrest-dev`):

```bash
make compose COMPOSE_CMD="up -d"     # start stack
make compose COMPOSE_CMD="down --volumes"  # tear down
```

Override `COMPOSE_FILE`/`COMPOSE_PROJECT` if you maintain alternate stacks.

### Labels you care about
| Label | Purpose |
| --- | --- |
| `backrest.enable=true` | opt-in a container |
| `backrest.repo` | override repo id (defaults to first repo in config) |
| `backrest.schedule` | cron schedule (default `0 2 * * *`; set minute to `T` to hash-stabilize a random minute per plan) |
| `backrest.paths.include` | comma-separated container paths |
| `backrest.paths.exclude` | comma-separated paths to skip |
| `backrest.keep` | retention spec (default `daily=7,weekly=4`) |
| `backrest.snapshot-start` / `backrest.snapshot-end` | CSV commands → snapshot start/end hooks |
| `backrest.hooks.template` | `simple-stop-start` autogenerates `docker stop/start <container>` hooks |
| `backrest.quiesce` | mark containers the sidecar should stop/start around `backup-once` |

See `docs/design-init.md` for the full matrix.

## CI / Publishing
`.github/workflows/docker-image.yml` builds with Buildx, tags via `docker/metadata-action`, and pushes to GHCR (or just builds on PRs). Set `GHCR` permissions or swap auth to your registry of choice.

## License
MIT (see LICENSE).
