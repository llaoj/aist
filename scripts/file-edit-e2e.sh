#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

echo "== file edit core =="
go test ./internal/fileedit -count=1

echo "== file edit tool/session integration =="
go test ./internal/interactive -run '^TestFileEdit(EndToEnd|Tool)' -count=1 -v

echo "FILE EDIT E2E PASS"
