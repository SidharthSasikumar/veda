#!/bin/bash
set -euo pipefail
VEDA_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$VEDA_ROOT"
command -v go >/dev/null || { printf 'Install Go 1.26+ from https://go.dev/dl/ and rerun.\n'; exit 1; }
command -v docker >/dev/null || { printf 'Install and start Docker Desktop, then rerun.\n'; exit 1; }
command -v llama-server >/dev/null || { command -v brew >/dev/null && brew install llama.cpp; }
"$VEDA_ROOT/scripts/build.sh"
"$VEDA_ROOT/bin/veda" init --workspace "$VEDA_ROOT/examples/allocdemo"
"$VEDA_ROOT/bin/veda" model setup --workspace "$VEDA_ROOT/examples/allocdemo" --profile auto
docker info >/dev/null
docker image inspect golang:1.26.4-alpine >/dev/null 2>&1 || docker pull golang:1.26.4-alpine
"$VEDA_ROOT/scripts/download-model.sh"
printf '\nSetup complete. Run scripts/start.sh, then open http://127.0.0.1:8787\n'
