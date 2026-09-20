#!/bin/bash
set -euo pipefail
VEDA_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
export PATH="/opt/homebrew/bin:/usr/local/go/bin:/usr/local/bin:$PATH"
WORKSPACE="${1:-$VEDA_ROOT/examples/allocdemo}"
mkdir -p "$VEDA_ROOT/.runtime"
if [ ! -x "$VEDA_ROOT/bin/veda" ]; then "$VEDA_ROOT/scripts/build.sh"; fi
if [ ! -f "$WORKSPACE/.veda/project.json" ]; then "$VEDA_ROOT/bin/veda" init --workspace "$WORKSPACE"; fi
if ! docker info >/dev/null 2>&1; then printf 'Start Docker Desktop before launching an investigation.\n'; fi
MODEL_PID=''
cleanup() { if [ -n "$MODEL_PID" ]; then kill "$MODEL_PID" 2>/dev/null || true; fi; }
trap cleanup EXIT INT TERM
if ! curl -fsS --max-time 2 http://127.0.0.1:18080/health >/dev/null; then
  "$VEDA_ROOT/scripts/start-model.sh" >"$VEDA_ROOT/.runtime/model.log" 2>&1 &
  MODEL_PID=$!
  printf 'Loading local model; log: %s/.runtime/model.log\n' "$VEDA_ROOT"
fi
printf 'Open http://127.0.0.1:8787 (Ctrl+C stops this session).\n'
"$VEDA_ROOT/bin/veda" serve --workspace "$WORKSPACE"
