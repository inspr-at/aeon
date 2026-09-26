# Release

Aeon uses INSPR Calendar Versioning v2 (`inspr-calendar-v2`). The coordinate is `YYMMDDhhmmss.0.0`: two-digit year, month, day, hour, minute, and second in UTC, then `.0.0`. It is SemVer-shaped and fixed width. `version.json` is the only source of that coordinate. The fields that matter for a release are `version_scheme`, `version`, `release_channel`, and `release_sequence`.

The git tag is `v` plus the `version` field, for example `v260926064658.0.0`. `scripts/release-tag.mjs` checks that a pushed tag has that shape. `scripts/verify-release.mjs` checks that `version.json` matches the scheme and that the vendored calendar presentation bundle under `web/src/vendor/calendar-version-display` matches `scripts/calendar-version-bundle-pin.json`. `just release-check` runs the verifier. A production web build runs the same check before it emits assets.

Development builds leave the linker version at `dev`. A release build sets:

```
-X github.com/inspr-at/aeon/internal/version.Version=<version>
```

with `CGO_ENABLED=0` and `-trimpath`. The server image uses the same linker setting (`Dockerfile`).

## Workflow

A push of a `v*` tag runs `.github/workflows/release.yml`.

1. Check out the repository with tags, so release history can see earlier coordinates.
2. Validate the tag and run `scripts/verify-release.mjs --release`. Fail if `version.json` disagrees with the tag.
3. Build `aeon-agentd` and `aeon-cli` for the platforms below.
4. Generate the release-history manifest embedded in the server image. That file is produced at release time. It is not committed.
5. Run the image smoke gate. Publishing waits for it.
6. Push the image to `ghcr.io/inspr-at/aeon:<version>`. There is no `latest` tag.
7. Attach the CLI, `aeon-agentd`, and `SHA256SUMS` to the GitHub release. The notes name the image and its digest.

## Image smoke gate

`scripts/smoke-image.sh` builds the release image and exercises it before anything is published. It checks the pinned Chromium and tini packages, their licenses, and `NOTICE`. It then starts a disposable Postgres and the server with mounted secret files, and checks startup, the database role, UID 65532, health, and headers. The script's dev mode is only for authenticated upload and quote calls. Live OIDC is not part of the gate, because the database is disposable and has no identity provider.

The gate needs Docker. It is a release check, not the day-to-day `just test` run.

## Release assets

| Asset | Where |
| --- | --- |
| Server image | `ghcr.io/inspr-at/aeon:<version>`, linux/amd64, provenance enabled |
| `aeon-cli-darwin-arm64`, `aeon-cli-darwin-amd64`, `aeon-cli-linux-amd64`, `aeon-cli-linux-arm64` | GitHub release for the `v` tag. Install the file as `aeon`. A symlink named `paimos` selects paimos mode. |
| `aeon-agentd-darwin-arm64`, `aeon-agentd-darwin-amd64`, `aeon-agentd-linux-amd64` | Same GitHub release. There is no linux/arm64 agentd asset. |
| `SHA256SUMS` | Same GitHub release, covering the CLI and agentd files above. Check it with `sha256sum -c` or `shasum -a 256 -c` before installing. |
| Flake | `flake.nix` in this repository. `packages.<system>.aeon` is the CLI plus a `paimos` symlink. `packages.<system>.aeon-agentd` is the supervisor. The version is the `version` field of `version.json`. |

Nix install of the CLI:

```
nix profile install github:inspr-at/aeon#aeon
```

The flake reference keeps resolving after the repository is renamed to `inspr-at/paimos`, because GitHub redirects the old name.

Screenshot data for a dev tenant is `aeon demo seed`. See [DEMO.md](DEMO.md). That command is not part of the release tag workflow.
