# Changelog

All notable changes to this project are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/). The client and the server share one version number and are released together with a `vX.Y.Z` tag.

## [Unreleased]

### Changed

- `run.sh` prompts for confirmation before downgrading when the installed client is newer than the server version, keeping silent upgrades when older. (client install script)
- `aist login` remembers the attempted server before authenticating when not logged in, so a cancelled or failed login (for example, started by `run.sh`) still defaults to that server next time. (client)
- License changed from proprietary to [AGPL-3.0](LICENSE).
- Client and server now share one version number and are released together from a single `v*` tag by the `release` workflow, replacing the separate `client/v*` and `server/v*` release lines.
- `run.sh` now downloads the client from, and logs in to, the server that served it, installs from `tar.gz` archives, and no longer depends on external object storage. (client install script, server)

### Added

- Server distributes the client under `/releases/client/`; the server image bundles the client built from the same commit. (server)
- Community files: `CONTRIBUTING.md`, `SECURITY.md`, `CODE_OF_CONDUCT.md`, issue and pull request templates.
- Contributor License Agreement (`CLA.md`) enforced on pull requests by the CLA Assistant workflow.

### Removed

- `AIST_VERSION` in `run.sh`: the server hosts a single client version matching its own, so the script always installs it.
- `release-client` and `release-server` workflows (replaced by `release`) and `scripts/deploy-client.sh`: client releases are no longer uploaded to R2; the server image distributes the client.

## Earlier Releases

Releases up to `client/v0.5.1` and `server/v0.5.1` were made before the project was open-sourced. See the [git tags](https://github.com/llaoj/aist/tags) and commit history for details.

[Unreleased]: https://github.com/llaoj/aist/compare/client/v0.5.1...HEAD
