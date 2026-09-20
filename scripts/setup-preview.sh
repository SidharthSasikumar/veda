#!/bin/sh
set -eu
preview_root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
docker build -t veda-ui-preview:1 "$preview_root/docker/ui-preview"
printf '\nVeda UI screenshot runtime is ready. Refresh an investigation with suggested changes enabled.\n'
