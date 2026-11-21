# Operations, Backup, Upgrade & Fly.io

## Health Check & Version Inspection

The server exposes a lightweight unauthenticated liveness endpoint:

```bash
curl -fsS https://aist.example.com/v1/healthz
# {"status":"ok","version":"v0.6.0"}
```

Inspect the running server binary version inside the container:

```bash
docker compose exec server server version
```

## Backups

All server state resides inside `/var/lib/aist/pb_data` (`data.db`, `auxiliary.db`, WAL files, and `storage/`).

You have two safe ways to back up:

1. **PocketBase Built-in Backups (Recommended for online hot backups)**:
   - Navigate to `/_/` → **Settings → Backups**.
   - Click **Create backup** for an immediate consistent SQLite snapshot `.zip`, or enable automatic cron backups (optionally uploading to S3-compatible object storage).
2. **Host Volume Backup**:
   - Stop the container briefly (`docker compose stop server`) or use SQLite online backup before archiving the host volume directory mapped to `/var/lib/aist`.

> [!WARNING]
> If you run the server with `--encryptionEnv`, store your 32-character encryption key in a separate password manager / KMS — encrypted backups cannot restore `Settings` or `upstream_providers.api_key` without the same key.

## Upgrading & Rolling Back

### Upgrade

1. Create a backup via `/_/` → **Settings → Backups**.
2. Update `AIST_SERVER_VERSION` (or pull the new tag) and recreate the container:
   ```bash
   AIST_SERVER_VERSION=0.6.0 docker compose up -d
   ```
3. Built-in database migrations run automatically in a transaction on startup.
4. Users upgrade their CLI automatically the next time they run `curl -fsSL https://<server>/run.sh | sh`.

### Rollback

Stop the container, restore your pre-upgrade backup into `/var/lib/aist/pb_data`, and start the previous image tag.

## Deploying on Fly.io (`deploy/server/fly.toml`)

The repository includes `deploy/server/fly.toml` for single-machine, single-volume deployment on Fly.io:

```bash
fly apps create my-aist-server
fly volumes create aist_data --app my-aist-server --region sin --size 1
fly deploy --config deploy/server/fly.toml --app my-aist-server

fly ssh console --app my-aist-server -C \
  "su-exec aist server superuser upsert admin@example.com '<password>'"
```

Operational notes on Fly.io:
- **Keep machine count at `1`**: PocketBase uses embedded single-node SQLite on the attached volume.
- **Cold starts**: `auto_stop_machines` stops idle machines after inactivity and wakes them automatically on incoming HTTP requests (~2s cold start; change to `"suspend"` if you want sub-second resume with ≤ 2 GB RAM).

## Production Launch Checklist

- [ ] `GET /v1/healthz` returns `200` and expected `version`
- [ ] `/_/` → **Settings → Application → Application URL** matches your external HTTPS URL
- [ ] **Trusted proxy headers** configured in **Settings → Application**
- [ ] **Rate limits** enabled in **Settings → Rate limits**
- [ ] **SMTP** tested in **Settings → Mail settings** (if registration, OTP, or password reset is used)
- [ ] `/_/` / superuser endpoints restricted via reverse proxy or Zero Trust
- [ ] Fresh client verified end-to-end with `curl -fsSL https://<server>/run.sh | sh`
