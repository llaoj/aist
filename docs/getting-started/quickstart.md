# Quickstart (Public Trial)

You can try AIST in under two minutes using the public trial server at `https://aistrun.cc`.

## 1. Register an Account with an Invite Code

1. Open `https://aistrun.cc/register` (or follow a prefilled invite link `https://aistrun.cc/register?code=<CODE>`).
2. Enter your invite code, email address, and password (8–71 characters).
3. If email verification is enabled on the server, click the activation link sent to your inbox before signing in.

## 2. Install & Sign In with One Command

Run the bootstrap script on Linux or macOS (`amd64` / `arm64`, plus `linux/arm`, `linux/386`, and `linux/riscv64`):

```bash
curl -fsSL https://aistrun.cc/run.sh | sh
```

Or with `wget`:

```bash
wget -qO- https://aistrun.cc/run.sh | sh
```

What `run.sh` does automatically:

1. Detects your OS and CPU architecture, downloads the version-matched `aist` archive from `https://aistrun.cc/releases/client/`, and verifies its SHA-256 checksum against `checksums.txt`.
2. Installs `aist` into a writable directory on your `PATH` (`/usr/local/bin`, `~/.local/bin`, or `~/bin`).
3. Initializes `~/.aist/preferences.json` (`0600`) by inferring your preferred language (`zh` or `en`) from your system locale on first run.
4. Starts `aist login https://aistrun.cc` interactively so you can sign in with your email and password.

## 3. Run Your First Troubleshooting Session

Start interactive mode:

```bash
aist
```

Or ask a single question directly from your shell:

```bash
aist "check disk usage and find the largest directories under /var/log"
```

Or pipe logs into AIST for read-only analysis:

```bash
dmesg -T | tail -n 100 | aist "any hardware or OOM warnings?"
```

## 4. Switch Language Anytime

Display text and AI reply language are stored locally on your machine — switch anytime without signing in again:

```bash
aist lang       # print current language (zh or en)
aist lang en    # switch to English
aist lang zh    # switch to Simplified Chinese
```

## Useful Keyboard Shortcuts

| Context | Key | Action |
|---|---|---|
| While waiting for model response | `Esc` | Cancel the current request and return to input without losing session history |
| Input prompt or confirmation menu | `Ctrl+C` | Save session and exit (`aist sessions` resumes anytime) |
| Command / file edit confirmation | `↑` / `↓` + `Enter` | Approve, skip single action, or reject remaining actions to add more context |
