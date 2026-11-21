# Install, Upgrade, Uninstall & Language

## Supported Platforms

The `aist` client is distributed as a static zero-dependency binary (`CGO_ENABLED=0`) for:

- **Linux**: `amd64`, `arm64`, `arm` (`GOARM=6`, compatible with ARMv6/v7), `386`, `riscv64`
- **macOS**: `arm64` (Apple Silicon), `amd64` (Intel)

## Install & Upgrade via `run.sh`

Every AIST server hosts `/run.sh` and its matching client release archives under `/releases/client/`:

```bash
curl -fsSL https://<your-server>/run.sh | sh
```

### Version Matching Behavior

Client and server share a single unified release version (e.g. `v0.6.0`). Running `/run.sh` again keeps your binary synchronized with the server:

| Installed State | `run.sh` Behavior |
|---|---|
| Not installed | Downloads `<version>` archive, verifies SHA-256 against `checksums.txt`, extracts to `/usr/local/bin`, `~/.local/bin`, or `~/bin`, and starts `aist` |
| Same version as server | Starts `aist` immediately |
| Older than server | Upgrades silently after checksum verification, then starts `aist` (falls back to existing binary if download fails) |
| Newer than server | Prompts `[y/N]` for confirmation before downgrading (defaults to keeping installed binary in non-interactive shells) |

Before entering interactive mode, `run.sh` checks `aist whoami`: if you are not signed in yet, it automatically runs `aist login <server-url>` on your terminal.

## Manual Archive Installation

You can also download `aist-<version>-<os>-<arch>.tar.gz` and `checksums.txt` from GitHub Releases or `https://<your-server>/releases/client/<version>/`:

```bash
VERSION=v0.6.0
OS=linux      # linux or darwin
ARCH=amd64    # amd64, arm64, arm, 386, riscv64

curl -fsSLO "https://aistrun.cc/releases/client/${VERSION}/aist-${VERSION}-${OS}-${ARCH}.tar.gz"
curl -fsSLO "https://aistrun.cc/releases/client/${VERSION}/checksums.txt"
grep "aist-${VERSION}-${OS}-${ARCH}.tar.gz" checksums.txt | shasum -a 256 -c -

tar -xzf "aist-${VERSION}-${OS}-${ARCH}.tar.gz"
install -m 0755 "aist-${VERSION}-${OS}-${ARCH}" /usr/local/bin/aist
```

Check the installed binary version:

```bash
aist version
```

## Client Language Preference (`aist lang`)

UI prompts, tool confirmations, localized error hints, and the AI's reply language are controlled locally on the client and stored in `~/.aist/preferences.json` (`0600`).

- **Automatic first-run inference**: When `~/.aist/preferences.json` does not exist yet, `aist` inspects `LC_ALL` → `LC_MESSAGES` → `LANG` (falling back to macOS preferred system language when POSIX locale variables are unset or encoding-only like `UTF-8`), chooses `zh` or `en`, and persists it once.
- **View or switch anytime** (works without signing in and survives `aist logout`):

```bash
aist lang       # show current language: zh or en
aist lang en    # switch to English
aist lang zh    # switch to Simplified Chinese
```

Switching language immediately updates both new model replies and how historical tool calls are rendered when resuming older sessions via `aist sessions`.

## Uninstall

Run the server-hosted uninstall script to remove both the `aist` binary and local state directory (`~/.aist`):

```bash
curl -fsSL https://<your-server>/uninstall.sh | sh
```

In non-interactive environments, pass `--yes` or `AIST_YES=1`:

```bash
curl -fsSL https://<your-server>/uninstall.sh | sh -s -- --yes
```

`uninstall.sh` never modifies your shell profile (`~/.zshrc`, `~/.bashrc`) and never touches server data.
