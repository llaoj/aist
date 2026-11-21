#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
WORK="$(mktemp -d "${TMPDIR:-/tmp}/aist-all-e2e.XXXXXX")"
cleanup() { rm -rf "$WORK"; }
trap cleanup EXIT

cd "$ROOT"

BIN="${BIN:-$WORK/aist}"
echo "Building $BIN"
go build -o "$BIN" ./cmd/aist
"$BIN" version

echo
echo "== all Go tests =="
go test -count=1 ./...

echo
echo "== file edit E2E (D16-D19) =="
./scripts/file-edit-e2e.sh

echo
echo "== multi-target / SSH E2E (D25-D32) =="
BIN="$BIN" ./scripts/m5-e2e.sh

echo
echo "ALL AUTOMATED E2E PASS"
echo "D27 real sudo/MFA/Ctrl+C remains a manual test; see docs/contributing/development.md"
