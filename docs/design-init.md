# BLUF

A **single Go sidecar** watches Docker for services labeled for backup, **upserts** Backrest plans into a **file-based JSON config**, writes atomically, and (optionally) **restarts Backrest** to apply. It also (optionally) runs **pre/post quiesce** and **per-workload restic retention** after each backup. One binary; no DB; safe to run on hosts with many Compose projects.

# High-level design

* **Discovery:** Enumerate containers across the local Docker engine via socket. Opt-in by labels.
* **Synthesis:** Map container labels + mounts → Backrest **Plan** objects.
* **Config Upsert:** Merge plans into an existing Backrest config (Repos are managed out-of-band). Atomic write.
* **Apply:** Restart Backrest (config-only change) or no-op if unchanged.
* **Run Modes:**

  * **reconcile** (default): detect changes, upsert plans, optionally apply.
  * **backup-once:** run rcb backup (one-shot), then per-workload `restic forget` per labels.
  * **daemon:** reconcile on interval and subscribe to Docker events for near-real-time updates.

# Motivations

* **Label truth:** Source of truth lives next to the workload (Compose labels).
* **Zero manual config drift:** Sidecar regenerates plans as stacks evolve.
* **Host-wide scope:** One binary covers dozens of Compose projects.
* **Safe rollouts:** Atomic config updates; idempotent merges; optional dry-run.

---

# Deliverable specifics (Go)

## Labels (authoritative schema)

Attach to *services you want backed up*:

**Enable & routing**

* `backrest.enable=true`
* `backrest.repo=<repo-id>` (defaults to `default`)
* `backrest.schedule=<cron>` (defaults `0 2 * * *`)

**Paths**

* `backrest.paths.include=/data,/config` (CSV; overrides auto)
* `backrest.paths.exclude=/cache,/tmp` (CSV)

**Quiesce hooks (Backrest executes)**

* `backrest.pre=sh -c 'docker stop $SELF'` (CSV allowed; multiple commands)
* `backrest.post=sh -c 'docker start $SELF'`

**Retention (for post-backup restic forget loop)**

* `backrest.keep=last=7,daily=7,weekly=4,monthly=12,within=90d`

**Quiesce (sidecar-controlled stop/start around rcb if desired)**

* `restic-compose-backup.quiesce=true`

**rcb selection (what to snapshot)**

* `restic-compose-backup.volumes=true`
* (optional) `restic-compose-backup.volumes.include=media,files`
* (optional) `restic-compose-backup.volumes.exclude=cache`

> Notes:
>
> * `$SELF` resolves to the container name the plan refers to.
> * If `backrest.paths.include` is absent, the sidecar derives host paths from mounts:
>
>   * bind mounts → `Mount.Source`
>   * named volumes → `${DOCKER_ROOT}/volumes/<name>/_data` (default `/var/lib/docker`)

## Backrest config (file model assumed)

Single JSON file with:

```json
{
  "repos": [
    { "id": "default", "type": "s3", "url": "s3:...", "env": { } }
  ],
  "plans": [
    {
      "id": "proj_service",           // unique, deterministic
      "repo": "default",
      "schedule": "0 2 * * *",
      "sources": ["/host/path1", "/host/path2"],
      "exclude": ["/host/path1/cache"],
      "hooks": { "pre": ["..."], "post": ["..."] },
      "retention": { "spec": "daily=7,weekly=4" }   // pass-through for sidecar forget loop
    }
  ]
}
```

* **Repos** are **pre-provisioned** (UI or initial config). Sidecar never edits repos.
* **Plans** are upserted by **id**.

## Binary responsibilities

1. **Discover containers** (filters):

   * `label=backrest.enable=true`
2. **Build plan**:

   * `id`: `${project}_${service}` if labels exist, else container name, all sanitized and prefixed (default `backrest_sidecar_`). Override with `--plan-id-prefix` / `BACKREST_PLAN_ID_PREFIX`.
   * `repo`: from `backrest.repo` or default; if the configured default is empty/unknown, the sidecar falls back to the first repo declared in the current config.
   * `schedule`: from label or default.
   * `sources`: from `backrest.paths.include` or derived from mounts; label paths that match a container mount/volume automatically rewrite to the host path (default `/var/lib/docker/volumes/...` unless `--volume-prefix` overrides).
   * `exclude`: from label.
   * `hooks.pre/post`: from label(s) (CSV → array).
   * `retention.spec`: raw string from `backrest.keep` or the configured default (defaults to `daily=7,weekly=4`).
3. **Merge** into existing config:

   * Ensure repo exists (warn if missing; do not create).
   * Upsert plan by `id` (replace entire plan object).
   * Stable key ordering for minimal diffs.
4. **Atomic write**:

   * Write `config.json.new` → `fsync` → `rename` to `config.json`.
   * Preserve the existing UID/GID and emit `config.json` with `0644` so the Backrest container and operators can read it without extra chmods.
5. **Apply**:

   * If changed and `--apply`, `docker restart <backrest-container-name>`.
6. **Backup/forget mode** (optional):

   * Execute **rcb** one-shot container (Option B) with provided env.
   * Loop containers with `backrest.keep` and run `restic forget` per path prefix.
   * Always restart quiesced containers even on failure.

## CLI

```
backrest-sidecar
  reconcile        # default
    --config /path/to/config.json
    --apply                  # restart Backrest if changed
    --backrest-container backrest
    --docker-sock /var/run/docker.sock
    --docker-root /var/lib/docker
    --volume-prefix /var/lib/docker/volumes   # override (e.g. /docker_volumes) if you bind-mount elsewhere
    --default-repo default
    --default-schedule "0 2 * * *"
    --default-retention "daily=7,weekly=4"
    --plan-id-prefix "backrest_sidecar_"
    --exclude-bind-mounts    # ignore bind mounts, volumes only
    --include-project-name   # include compose project in plan id
    --dry-run
  backup-once
    --rcb-image zettaio/restic-compose-backup:0.7.1
    --rcb-env-file /etc/rcb.env    # RESTIC_* etc.
    --quiesce-label restic-compose-backup.quiesce=true
  daemon
    --interval 60s
    --with-events             # listen to Docker events for faster reconcile
    (all reconcile flags)
```

## Dry-run workflow (existing labels)

Use `--dry-run` (or `make dry-run`) to inspect the rendered plans before writing the Backrest config. Because the host already mounts `/var/run/docker.sock`, the sidecar can read every Compose project’s labels without extra setup.

1. `make build` – produces `./bin/backrest-sidecar`.
2. `make dry-run CONFIG=/etc/backrest/config.json RUN_FLAGS="--docker-root /var/lib/docker --default-repo default --include-project-name"` – reuses production flags but never writes the config.
3. Inspect the structured logs (one per candidate plan) to confirm the existing labels resolve to the expected sources/hooks.
4. Use `testdata/example-sidecar.config.json` (the exact JSON shape the sidecar reads/writes) for local runs; `testdata/example-backrest.config.json` remains as an untouched export for reference.
5. Helper scripts under `scripts/` wrap the common flows:
   * `scripts/dry-run-make.sh` – runs the Makefile target against the sample config (override `CONFIG`/`RUN_FLAGS` as env vars).
   * `scripts/dry-run-binary.sh` – builds if needed and runs `./bin/backrest-sidecar reconcile --dry-run ...` (pass extra flags as args).
  * `scripts/dry-run-docker.sh` – mounts the entire config directory (`CONFIG_DIR`, default `testdata`) at `/etc/backrest`, sets `CONFIG_FILE` (default `example-sidecar.config.json`), and runs `docker compose run --build sidecar ...` with the usual dry-run flags. Override `CONFIG_DIR`, `CONFIG_FILE`, `COMPOSE_FILE`, or CLI args to suit your test.
  * `compose.dry-run.yaml` builds the Dockerfile, spins up a sample `demo-echo` container labeled with `backrest.*` keys **and** a `demo-echo-lite` service that only sets `backrest.enable=true`, and wires the right volumes/env. Run `CONFIG_DIR=/abs/path/to/configs CONFIG_FILE=my-config.json docker compose -f compose.dry-run.yaml up demo-echo demo-echo-lite sidecar` for an end-to-end playground, or rely on the helper script above.

Example output when the `db` service from the section below is already labeled:

```console
$ make dry-run CONFIG=/etc/backrest/config.json RUN_FLAGS="--docker-root /var/lib/docker"
./bin/backrest-sidecar reconcile --dry-run --config /etc/backrest/config.json --docker-root /var/lib/docker
{"level":"info","action":"plan.rendered","plan_id":"stack_db","sources":["/var/lib/docker/volumes/stack_pgdata/_data"],"hooks":{"pre":["sh -c 'docker stop $SELF'"],"post":["sh -c 'docker start $SELF'"]},"retention":{"spec":"daily=7,weekly=4,monthly=6"}}
{"level":"info","action":"dry-run.complete","plans_seen":1,"plans_changed":1,"config":"/etc/backrest/config.json"}
```

The command exits with code `2` (no write performed) but prints exactly how the labels would shape the config, making it safe to run directly from a host shell or via the container.

## Environment variables (fallbacks)

* `DOCKER_HOST` (socket override), `DOCKER_API_VERSION`
* `BACKREST_CONFIG` (path to config.json; overrides `--config`)
* `BACKREST_VOLUME_PREFIX` (defaults to `/var/lib/docker/volumes`, override when you bind that tree somewhere else such as `/docker_volumes`; labeled paths that refer to container mountpoints rewrite through this prefix)
* `BACKREST_DEFAULT_RETENTION` (optional) to override the fallback `daily=7,weekly=4`
* `BACKREST_PLAN_ID_PREFIX` (defaults to `backrest_sidecar_`)
* `RESTIC_*` in `--rcb-env-file` for backup-once mode

## rcb one-shot (Option B)

Run as:

```
docker run --rm \
  -v /var/run/docker.sock:/tmp/docker.sock:ro \
  --env-file /etc/rcb.env \
  zettaio/restic-compose-backup:0.7.1 rcb backup
```

Sidecar does this in `backup-once`. `--exclude-bind-mounts` maps to `EXCLUDE_BIND_MOUNTS=1`, `--include-project-name` to `INCLUDE_PROJECT_NAME=1`, etc.

## Algorithms

**Reconcile**

1. Load current config (if missing, create with empty `plans`).
2. List containers with `backrest.enable=true`.
3. For each:

   * Read compose labels: `com.docker.compose.project`, `com.docker.compose.service`.
   * Build plan (derive sources if none specified).
4. Merge: map `[plan.id] = plan`.
5. If diff:

   * Write atomically.
   * `--apply` → restart Backrest.
6. Metrics/log summary: totals, changes, skipped (no repo), errors.

**Derive sources**

* For each `Mount`:

  * If `Type=="bind"` and not `--exclude-bind-mounts`: add `Source`.
  * If `Type=="volume"`: add `${DOCKER_ROOT}/volumes/<Name>/_data`.
* De-dup + sort.

**Per-workload forget (post-backup)**

* For each container with `backrest.keep`:

  * Build restic flags from spec:

    * `last=N` → `--keep-last N`
    * `hourly/daily/weekly/monthly/yearly=N` → corresponding flags
    * `within=90d|1y5m7d` → `--keep-within` (and `within-d/w/m/y` variants)
  * Compute path prefix:

    * `/volumes[/<project>]/<service>` (match your rcb `INCLUDE_PROJECT_NAME` setting)
  * Run: `restic forget --group-by paths --path "<prefix>" [flags...] --prune`

**Quiesce (sidecar-controlled)**

* Gather running containers with `restic-compose-backup.quiesce=true`.
* `docker stop --time <timeout> ...`
* `backup-once`
* Always `docker start ...` in `defer`/`finally`.

## Edge cases & rules

* **Repo missing:** log warn, skip plan (do not create repo entries).
* **No mounts & no include paths:** skip plan with error.
* **Config invalid JSON:** fail fast (no overwrite).
* **Concurrent writers:** atomic rename minimizes tear; optional advisory lockfile.
* **Docker root non-standard:** allow `--docker-root` override.
* **Hot reload:** if Backrest later gains reload, support `--reload-cmd` instead of restart.

## Security

* Least privilege: mount Docker socket **read-only**.
* Store Backrest config on a bind volume; protect permissions.
* Never log secrets from repos or `RESTIC_PASSWORD*`.
* Validate label length/path traversal; normalize paths.

## Testing

* Unit: plan synthesis (mount derivation, label parsing, id generation), merge/upsert, diff detection.
* Integration: kind/minikube not required—use `docker` local with a test compose.
* Golden files for config output.
* Fault injection: invalid JSON, missing repos, rename failure.

## Logging/metrics

* Structured logs (`level`, `action`, `plan_id`, `changed=true/false`).
* Exit codes: 0 ok; 2 no changes; 3 partial failures.
* Optional Prometheus: counters for plans, changes, errors.

## Code layout

```
cmd/backrest-sidecar/main.go
internal/docker/discover.go        // client, filters, mount extraction
internal/model/labels.go           // label keys, parsing, defaults
internal/model/plan.go             // Plan struct, merge, diff
internal/config/file.go            // read/validate/write atomic
internal/app/reconcile.go          // orchestrates reconcile flow
internal/app/backup.go             // rcb one-shot, quiesce, forget
internal/util/exec.go              // run cmds, capture logs
internal/util/fs.go                // atomic write, lockfile
```

## Types (condensed)

```go
type Plan struct {
  ID        string   `json:"id"`
  Repo      string   `json:"repo"`
  Schedule  string   `json:"schedule"`
  Sources   []string `json:"sources"`
  Exclude   []string `json:"exclude,omitempty"`
  Hooks     Hooks    `json:"hooks,omitempty"`
  Retention RetSpec  `json:"retention,omitempty"`
}
type Hooks struct {
  Pre  []string `json:"pre,omitempty"`
  Post []string `json:"post,omitempty"`
}
type RetSpec struct {
  Spec string `json:"spec,omitempty"` // pass-through label
}
type Config struct {
  Repos []Repo `json:"repos"`
  Plans []Plan `json:"plans"`
}
type Repo struct {
  ID   string            `json:"id"`
  Type string            `json:"type"`
  URL  string            `json:"url"`
  Env  map[string]string `json:"env,omitempty"`
}
```

## Pseudocode (reconcile)

```go
cfg := loadConfig(path)
cset := discoverContainers(ctx, filters{"backrest.enable=true"})
plans := []Plan{}
for c in cset {
  p := buildPlan(c, defaults)        // labels+mounts -> Plan
  if repoMissing(cfg.Repos, p.Repo) { log.Warn(...); continue }
  plans = append(plans, p)
}
changed, _ := cfg.UpsertPlans(plans)    // map by Plan.ID
if changed {
  writeAtomic(path, cfg)
  if apply { dockerRestart(backrestContainer) }
}
```

## Build & packaging assets

### Makefile helpers

The root `Makefile` standardizes common workflows:

* `make build` – cross-compiles `./cmd/backrest-sidecar` into `./bin/backrest-sidecar` with version metadata.
* `make run ARGS="reconcile --config ..."` – builds (if needed) then runs the binary locally.
* `make dry-run CONFIG=/etc/backrest/config.json RUN_FLAGS="--docker-root ..."` – runs reconcile with `--dry-run`, ideal for validating existing labels before writes.
* `make docker-build TAG=ghcr.io/you/backrest-sidecar:dev` – builds the multi-arch image defined in `./Dockerfile`.
* `make docker-run CONFIG=/etc/backrest/config.json DOCKER_ARGS="daemon --interval 30s"` – builds (if needed) then launches a container that already mounts the Docker socket and Backrest config.
* The Makefile auto-detects when `go` is missing locally and falls back to `golang:1.23` via Docker (`GO_VERSION`/`GO_IMAGE` override the tag, and `USE_DOCKER_GO=1` forces the containerized toolchain).
* `Dockerfile` ships a root-owned distroless image, so the container can talk to `/var/run/docker.sock` and write bind-mounted configs without extra group plumbing. Drop privileges via Compose (`user:`) later if desired.

### Dockerfile

`./Dockerfile` is a two-stage build:

1. `golang:<version>` stage compiles the static binary with BuildKit cache mounts for fast rebuilds (`GO_VERSION` arg is overridable).
2. `gcr.io/distroless/base-debian12:debug` stage copies the binary, keeps a minimal BusyBox shell for troubleshooting, and runs as root with `ENTRYPOINT ["backrest-sidecar"]` / `CMD ["daemon","--config","/etc/backrest/config.json","--with-events","--apply"]`.

You can override the command when calling `docker run` or `make docker-run`, but the defaults assume the config is bind-mounted at `/etc/backrest/config.json`, `/var/run/docker.sock` is already provided by the host, and `/var/lib/docker` is mounted read-only for volume derivation.

### Hosted container workflow

```bash
docker run --rm \
  -v /etc/backrest/config.json:/etc/backrest/config.json \
  -v /var/run/docker.sock:/var/run/docker.sock:ro \
  -v /var/lib/docker:/var/lib/docker:ro \
  ghcr.io/you/backrest-sidecar:dev \
  daemon --config /etc/backrest/config.json --with-events --apply
```

That invocation mirrors `make docker-run` and is ready for systemd/nomad, since the Docker socket is already mounted on the host. Swap `daemon` for `reconcile --dry-run` during smoke tests, or use `--exclude-bind-mounts` in environments where only named volumes matter.

## Deployment (compose)

```yaml
services:
  backrest:
    image: <backrest-image>
    volumes:
      - backrest-config:/etc/backrest
    environment: [ ... repo auth ... ]

  sidecar:
    image: ghcr.io/you/backrest-sidecar:latest
    command: ["daemon","--config","/etc/backrest/config.json","--apply","--with-events"]
    volumes:
      - backrest-config:/etc/backrest
      - /var/run/docker.sock:/var/run/docker.sock:ro
      - /var/lib/docker:/var/lib/docker:ro
    environment:
      - BACKREST_CONFIG=/etc/backrest/config.json
volumes:
  backrest-config: {}
```

## Example service labels

```yaml
services:
  db:
    labels:
      backrest.enable: "true"
      backrest.repo: "default"
      backrest.schedule: "15 3 * * *"
      backrest.keep: "daily=7,weekly=4,monthly=6"
      backrest.paths.exclude: "/var/lib/postgresql/data/pg_wal"
      restic-compose-backup.volumes: "true"
      restic-compose-backup.quiesce: "true"
      backrest.pre: "sh -c 'docker stop $SELF'"
      backrest.post: "sh -c 'docker start $SELF'"
    volumes:
      - pgdata:/var/lib/postgresql/data
volumes: { pgdata: {} }
```

## Future work

* Hot-reload support (swap restart).
* Per-repo policy overlays.
* Snapshot reports + restore drills.
* Multi-engine discovery (remote Docker hosts).

**Ready to implement.**
