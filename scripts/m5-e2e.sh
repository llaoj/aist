#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

echo "== target validation =="
go test ./internal/target -count=1

echo "== tools target schema =="
go test ./internal/tools -count=1 -run 'Shell' -v

echo "== SSH executor and router =="
go test ./internal/executor -count=1 -v

echo "== interactive multi-target confirmation =="
go test ./internal/interactive -count=1 -run 'Shell|Query|ApproveRemaining|Target' -v

echo "== M5 cross-package E2E =="
go test ./internal/e2e -count=1 -v

echo
echo "M5 E2E PASS"
echo "D27 real sudo/MFA/Ctrl+C remains a manual test; see docs/contributing/development.md"
