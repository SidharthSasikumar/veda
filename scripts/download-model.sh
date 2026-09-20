#!/bin/bash
set -euo pipefail
VEDA_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
MODEL_DIR="$VEDA_ROOT/models"
MODEL_FILE="$MODEL_DIR/Qwen3.5-9B-Q4_K_M.gguf"
MODEL_SHA=03b74727a860a56338e042c4420bb3f04b2fec5734175f4cb9fa853daf52b7e8
mkdir -p "$MODEL_DIR"
if [ ! -f "$MODEL_FILE" ]; then
  curl -fL --retry 3 --continue-at - 'https://huggingface.co/unsloth/Qwen3.5-9B-GGUF/resolve/3885219b6810b007914f3a7950a8d1b469d598a5/Qwen3.5-9B-Q4_K_M.gguf' -o "$MODEL_FILE.part"
  printf '%s  %s\n' "$MODEL_SHA" "$MODEL_FILE.part" | shasum -a 256 -c -
  mv "$MODEL_FILE.part" "$MODEL_FILE"
else
  printf '%s  %s\n' "$MODEL_SHA" "$MODEL_FILE" | shasum -a 256 -c -
fi
printf 'Local model ready: %s\n' "$MODEL_FILE"
