#!/bin/bash
set -euo pipefail
VEDA_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$VEDA_ROOT"
mkdir -p bin .cache/go-build .cache/go-mod
export GOCACHE="${GOCACHE:-$VEDA_ROOT/.cache/go-build}"
export GOMODCACHE="${GOMODCACHE:-$VEDA_ROOT/.cache/go-mod}"
go build -trimpath -o bin/veda ./cmd/veda
printf 'Built %s/bin/veda\n' "$VEDA_ROOT"
