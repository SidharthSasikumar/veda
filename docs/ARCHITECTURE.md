# Architecture and boundaries

```text
CLI / local browser
        ↓
Go experiment engine ↔ SQLite research memory
        ↓                   ↕
Loopback model gateway   saved outcomes
        ↓
plan → candidate → path/Go-source validation
        ↓
private source snapshot → disposable Docker execution
        ↓
unchanged tests → three benchmark samples → median evaluation
        ↓
logs + metrics + diff + checksum manifest → report + proposed patch
```

Model calls are restricted to HTTP endpoints with a literal loopback address. Proxy environment variables and redirects are disabled. The engine sends bounded Go source, its objective, and prior experiment evidence to the local model. It does not read dotfiles, keys, hidden directories, or symlink targets into a snapshot. Do not place secrets in normal Go source if you do not want them in local research records.

The model can replace one to four existing non-test `.go` files. Absolute paths, traversal, dot paths, vendor files, symlinks, tests, dependencies, non-Go files and malformed Go syntax are refused. No model-generated host commands are executed. The tests and benchmarks in the original snapshot remain fixed; passing them is useful evidence but cannot prove equivalence for all inputs.

Docker experiments use read-only source mounts, a read-only root filesystem, no network, an unprivileged UID, dropped capabilities, no-new-privileges, 2 CPU cores, 2 GB RAM, a 256 PID limit, and execution deadlines. Temporary filesystems contain builds and candidate execution. `/tmp` is executable because Go tests compile temporary binaries there. No home directory, host credentials, or Docker socket is mounted in the container. Container image IDs are pinned per run. On timeout or cancellation, only the specifically named Veda container is forcibly removed.

The local HTTP server accepts only the exact loopback Host and same-origin requests, checks a random per-session token for mutations, serves no external scripts, and applies a restrictive content-security policy. This is a single-user local application, not an authenticated public web service; do not expose it to a network.

The original workspace remains unchanged. Evidence records are append-only in the application workflow and carry SHA-256 manifests. There is no deletion endpoint. If the engine is interrupted, completed experiments stay in SQLite and on disk; the next investigation marks abandoned running records as interrupted. It does not resume unfinished model calls or experiments automatically.

The research lock serializes CLI and UI investigations against one workspace. Model failures, invalid proposals, failed tests, missing benchmarks and interrupted containers are observable failures. Failed baseline/plan runs produce a partial report. A completed run may miss its target. Demo proposals are explicitly labeled as scripted.
