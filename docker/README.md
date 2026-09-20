# Experiment image

Veda uses the official `golang:1.26.4-alpine` image, downloaded explicitly during setup. Every research run resolves the tag to a local immutable image ID and records it in `environment.json`. Experiments use `--pull=never`.

No custom image or host shell execution is needed. The Go project is mounted read-only at `/src`, copied into a disposable tmpfs, and tested as UID 65534. The container has no network, added capabilities, writable root filesystem, host credentials, or Docker socket. It has a 2 GB RAM limit, 2 CPU quota, 256 PID limit and bounded execution time. Go caches are in tmpfs. No host source changes are applied.

Go dependencies must be vendored before running, or the module must use only the standard library. Vendoring is a deliberate user preparation step and requires internet at that time. `GOPROXY=off`, `GOSUMDB=off`, `GOTOOLCHAIN=local` remain set in experiments.

## UI screenshot image

`../scripts/setup-preview.sh` builds the optional `veda-ui-preview:1` image from `ui-preview/Dockerfile`. Its trusted Node HTTP harness serves only captured static files; Chromium records a 1280 × 900 PNG. Node's built-in APIs are sufficient; there are no npm dependencies. [Chrome Headless](https://developer.chrome.com/docs/automation-and-testing/headless) supplies the unattended browser rendering.

Captures run as UID 65534 with no network, read-only mounts, dropped capabilities, no new privileges, resource limits, a fixed deadline, and an ephemeral browser profile in tmpfs. Chromium uses `--no-sandbox` inside that container, making the Docker boundary essential. Never use this harness to run untrusted pages directly on the host. Installation downloads system packages; actual captures use the recorded immutable image ID and `--pull=never`.
