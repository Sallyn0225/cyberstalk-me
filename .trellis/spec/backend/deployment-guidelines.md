# Deployment Guidelines

> How `server/` is packaged and shipped: the container image, the compose
> deployment, and the CI/CD gates. Established 2026-07-30 in task
> `07-30-deploy-docker-cicd`; every constraint below was verified there.

---

## The load-bearing constraint: the runtime stage has zero `RUN`

The image is built for `linux/amd64` and `linux/arm64`. The build stage is
pinned to `--platform=$BUILDPLATFORM` and reaches the target architecture
through Go cross-compilation (`GOARCH=$TARGETARCH`), while the runtime stage
contains only `COPY` / `ENV` / `USER` / `EXPOSE` / `VOLUME` / `ENTRYPOINT`.

Because nothing ever has to execute on the target architecture, the multi-arch
build needs no QEMU emulation — an arm64 image costs the same to build as an
amd64 one (measured: 2m57s for both, in one CD run).

> **Adding a `RUN` to the runtime stage breaks this.** The build will then need
> `docker/setup-qemu-action` in `.github/workflows/release.yml` and build time
> grows several-fold. If you must add one, add the QEMU setup in the same
> change and say why in the commit message.

Anything needed at runtime should be produced in the build stage and copied
across instead. The `/data` directory, for example, is `mkdir`-ed in the build
stage and copied with `--chown=65532:65532`.

## Image invariants

| Invariant | Why |
|-----------|-----|
| `CGO_ENABLED=0` | `modernc.org/sqlite` is pure Go; a static binary means the runtime stage needs no libc matching and cross-compilation is free |
| `GOWORK=off`, and `go.work` is not copied into the image | The workspace lists the Windows-only `client-windows` module; resolving it under `GOOS=linux` fails. `server/go.mod`'s `replace cyberstalk.me/shared => ../shared` is sufficient |
| Build context is the **repo root** | Same `replace` directive: the build must be able to see `shared/` |
| Runs as `USER 65532:65532` | Non-root. Numeric UID rather than `adduser` precisely because `adduser` would be a `RUN` in the runtime stage |
| `-trimpath -ldflags='-s -w'` | No local paths or debug symbols in the shipped binary |
| No `HEALTHCHECK` in the image | Health-check semantics belong to the deployment, not the image; it lives in `compose.yaml` where a deployer can override it |

`alpine` is the runtime base rather than distroless, deliberately: deploying
this means running `docker compose exec app cyberstalk-server register-device`,
`wget`-based health checks, and occasionally looking inside `/data`. A few MB
buys those.

## Configuration surface

The server is env-only — no config files (see `internal/config`). Two rules
keep the deployment coherent:

- **The container always listens on `:8080`.** Only the host side of the port
  mapping is tunable (`HOST_PORT`). Letting a deployer change `ADDR` would
  desynchronize the health check and the port mapping at once, so `ADDR` is not
  exposed in `.env.example`.
- **Every compose variable has an inline default** (`${VAR:-default}`), so a
  deployment with no `.env` file starts correctly. `.env.example` documents the
  knobs; it is not a prerequisite.

The screen-time statistics added four variables, all with inline compose
defaults (established 2026-07-30 in `07-30-screen-time-server`):

| Variable | Default | Notes |
|----------|---------|-------|
| `DISPLAY_TIMEZONE` | `Asia/Shanghai` | IANA name; validated with `time.LoadLocation` at startup, an unknown name stops the server |
| `USAGE_RETENTION_DAYS` | `365` | Positive integer. ~12 KB per device per day |
| `USAGE_PRUNE_INTERVAL` | `1h` | The retention sweep also runs once at startup, so a frequently-restarted server still prunes |
| `USAGE_MAX_GAP` | empty → `OFFLINE_THRESHOLD` | Empty in compose on purpose: the server resolves the default, so there is nothing to keep in sync |

### `time/tzdata` is embedded, and that is a deployment constraint

`server/cmd/server/main.go` imports `_ "time/tzdata"`. This is not a
convenience — **alpine ships no tzdata**, so without it
`time.LoadLocation("Asia/Shanghai")` fails and the server does not start, while
every local build and CI job stays green. The failure only appears in the
container.

The obvious fix — `RUN apk add tzdata` in the runtime stage — is exactly what
the zero-`RUN` constraint above forbids. Embedding costs ~450 KB of binary and
removes the whole failure path, so **do not solve a timezone problem in the
Dockerfile.**

The health check targets `GET /api/v1/snapshot`, not `/`. The latter only proves
static files are being served; the former also reads the store, so a green check
means "serving *and* the database is readable". Do not add a dedicated health
endpoint for this — `/api/v1/snapshot` is public and cheap.

## The shipped compose file carries no resource limits

`compose.yaml` is a deliverable for other people's machines, whose size is
unknown. Memory/CPU caps, pinned local image names, and similar
environment-specific settings belong in a local `compose.override.yaml`
(gitignored), which Compose layers on automatically.

The same principle applies to the Dockerfile: `GOFLAGS` is an `ARG` with an
empty default, so a memory-constrained builder can pass
`--build-arg GOFLAGS=-p=2` without slowing every other build down permanently.

## CI gates mirror this spec, they do not invent a second standard

`.github/workflows/ci.yml` runs the same commands documented in
[Quality Guidelines](./quality-guidelines.md), including the repo-root-plus-
explicit-module-paths form that `go.work` forces. Two details are easy to get
wrong:

- `client-windows` must be checked with `GOOS=windows GOARCH=amd64`. Under
  `GOOS=linux` every file is excluded by build constraint and the build fails
  with "build constraints exclude all Go files".
- The **embedded frontend freshness check** is a hard failure, not a warning.
  `vite build` writes into `server/cmd/server/web/`, which the binary embeds, so
  CI rebuilds the frontend and diffs that directory. Vite output was measured to
  be deterministic across repeated builds, which is what makes a hard failure
  safe here. See the frontend spec for the committing discipline this enforces.

## Release flow

Tags on `v*` publish semver-tagged images plus `latest`; pushes to `main`
publish `edge`. Both go to `ghcr.io/sallyn0225/cyberstalk-me` with
`permissions: {contents: read, packages: write}` and GHA layer caching.

- `docker/metadata-action` lowercases the image name automatically, which
  matters because the repository owner is mixed-case and GHCR rejects uppercase
  paths.
- `release.yml` uses `paths-ignore` for `.trellis/**` and `**.md` to skip the
  rolling `edge` rebuild on docs-only commits. This is safe alongside the tag
  trigger: GitHub does not evaluate path filters for tag pushes.
- A newly created GHCR package is **private** until someone flips it in the
  GitHub UI. Until then deployers get `denied` on `docker pull`.

### Verifying which architecture an image really is

`docker pull --platform linux/arm64 <tag>` does **not** re-pull when the same
tag already exists locally, and `docker image inspect` will keep reporting the
old architecture. Pull by manifest digest instead when the architecture is what
you are trying to prove.
