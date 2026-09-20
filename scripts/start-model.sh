#!/bin/bash
set -euo pipefail
VEDA_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
MODEL_FILE="${VEDA_MODEL_FILE:-$VEDA_ROOT/models/Qwen3.5-9B-Q4_K_M.gguf}"
MODEL_ALIAS="${VEDA_MODEL_ALIAS:-Qwen3.5-9B-Q4_K_M}"
if [ ! -f "$MODEL_FILE" ]; then printf 'Model missing. Run scripts/download-model.sh first.\n'; exit 1; fi
command -v llama-server >/dev/null || { printf 'Install runtime: brew install llama.cpp\n'; exit 1; }
exec llama-server --model "$MODEL_FILE" --alias "$MODEL_ALIAS" --host 127.0.0.1 --port 18080 --ctx-size 8192 --parallel 1 --n-gpu-layers all --jinja --reasoning off --no-webui --cors-origins http://127.0.0.1:18080 --no-cors-credentials
