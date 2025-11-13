## Usage (Immutable)

- This file is used to provide instruction to the agent
- The code gen agent must update this file to capture DX preferences and codebase description
- Put your long term memory stuff about the project and how we like to operate below here as you see fit!
- This file contains what the agent should need to know to work in this project

## Amendments (Mutable)

### Codebase Snapshot
- Go 1.23 module that builds `backrest-sidecar`, a Docker-friendly helper that renders Backrest plans from labeled containers and writes `/etc/backrest/config.json`.
- Default CLI flags assume Docker socket at `/var/run/docker.sock`, config at `/etc/backrest/config.json`, docker root `/var/lib/docker`, and now a default volume prefix of `/var/lib/docker/volumes` (override via `--volume-prefix` or `BACKREST_VOLUME_PREFIX`).
- Plan IDs are prefixed with `backrest_sidecar_` by default; expose overrides via `--plan-id-prefix` / `BACKREST_PLAN_ID_PREFIX` if integrations depend on old names.
- Set the fallback repo via `--default-repo` or `BACKREST_DEFAULT_REPO`; if neither is provided, the sidecar inherits the repo ID from the first plan in `config.json` (or the first repo entry when no plans exist).
- Use `backrest.hooks.template=simple-stop-start` when you want automatic `docker stop/start` hooks; custom hooks still win if provided.
- Config writes use `fsutil.AtomicWrite`, preserve the existing UID/GID, and set permissions to `0644` so Backrest can read the file.
- `.dockerignore` excludes `backrest.config.json` (+ `.new`) to prevent accidental `COPY` of host configs.

### DX Preferences & Process
- When the task touches Backrest behavior, ground answers in official Backrest docs; fetch via Context7 (`/garethgeorge/backrest`) before summarizing.
- Watch for file permission regressions: config JSON must remain `0644` and retain ownership.
- Be explicit about docker volume prefixes and mounting assumptions so operators know how to override them.
- Default retention fallback is `daily=7,weekly=4`; expose overrides via `--default-retention` or `BACKREST_DEFAULT_RETENTION` when relevant.
- Local host lacks Go; run formatting/tests via `make fmt`, `make test`, etc., which automatically wrap `go` inside `golang:<GO_VERSION>` using Docker (see Makefile). Prefer invoking these common workflows through the Make targets rather than ad-hoc `docker run ...` lines.
