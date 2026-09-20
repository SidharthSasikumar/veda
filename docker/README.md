# Experiment image

Veda uses the official `golang:1.26.4-alpine` image, downloaded explicitly during setup. Every research run resolves the tag to a local immutable image ID and records it in `environment.json`. Experiments use `--pull=never`.

No custom image or host shell execution is needed. The Go project is mounted read-only at `/src`, copied into a disposable tmpfs, and tested as UID 65534. The container has no network, added capabilities, writable root filesystem, host credentials, or Docker socket. It has a 2 GB RAM limit, 2 CPU quota, 256 PID limit and bounded execution time. Go caches are in tmpfs. No host source changes are applied.

Go dependencies must be vendored before running, or the module must use only the standard library. Vendoring is a deliberate user preparation step and requires internet at that time. `GOPROXY=off`, `GOSUMDB=off`, `GOTOOLCHAIN=local` remain set in experiments.
