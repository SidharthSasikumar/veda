# Validation — September 20, 2026

Validated on Apple M5 Pro / 24 GB unified memory, macOS 26.5.2, Go 1.26.4, Docker 29.6.1 and llama.cpp 0.4.0. The model download passed its pinned SHA-256 checksum.

## Virtual agent dashboard

The crew overview and per-investigation control room were validated against saved successes, blocked checks, rejected benchmark candidates, and reused evidence. A fresh browser-started investigation (`RUN-20260920-133016.767655000`) exercised the live state: Thinker alone had an active work animation during model review, while completed roles and the waiting Keeper did not. The investigation then completed with all seven recorded steps saved.

Browser checks covered selecting an investigation, selecting Tinkerer, opening its exact rejected candidate and patch, pausing motion, returning to the overview, and searching investigations. The 413px viewport had six crew stations and no horizontal document overflow. Six Node tests cover live role assignment, cancellation/interruption, failed/blocked results, cached evidence, summary/detail consistency, and rejected/disabled tasks. Existing Go tests pass with the race detector; Go vet, JavaScript syntax, and diff checks pass.

The characters are decorative representations of recorded pipeline roles. They do not add parallel model agents or fabricate intermediate task activity.

## Repository hub (Veda 0.2)

The GitHub URL + objective workflow was exercised through the browser against `SidharthSasikumar/Personal-Engineering-Brain`, commit `6e89169327` (full commit stored in evidence). Final fresh run: `RUN-20260920-131516.330984000`.

- Captured 30 files and extracted 13 components with source-backed relationships.
- Clicked the Compose API component and opened `docker-compose.yml` at its cited line.
- Downloaded Go dependencies in a separate manifests-only container; then existing tests in three Go packages and `go vet` passed with test-container networking disabled.
- Ran the local 9B model to produce the report, with interpretations separately labeled.
- Inspected architecture and experiment graphs, source excerpts, and detailed check results in the browser.
- Repeated the identical request as `RUN-20260920-131609.888182000`; all seven completed steps were reused from the final fresh run with the original timestamp and explicit reuse banner.
- Terraform cross-file references, Compose dependency/build links, source integrity, GitHub input validation, cancellation, single-server locking, same-commit reuse, changed-commit invalidation, settings invalidation, and restart persistence are covered by automated tests.
- `go test -race ./...`, `go vet ./...`, JavaScript syntax, and `git diff --check` passed for this update.

Earlier hub trials are retained as history. They exposed missing offline dependencies and archive permissions; both were corrected before the final fresh trial. A completed investigation can contain failed or blocked checks; completion means the report was saved, not that the repository passed every check. Python syntax checks were not exercised in this live trial. A second UI trial on the Veda repo (`RUN-20260920-131700.039306000`, branch `codex/usable-prototype`) passed JavaScript syntax checks and the example Go tests, then recorded a three-sample baseline and three rejected optimization candidates. All three candidates failed compilation due to a missing import, so this run achieved no improvement; the UI retains their patches/errors and does not display zero-value failed measurements as successful results. The repository-wide root Go checks exposed its CGO requirement, separately from the passing example module. The original benchmark engine also has the successful trial below.

## Automated checks (original benchmark engine)

- `go test -race ./...`: passed, eight test cases.
- `go vet ./...`: passed.
- JavaScript syntax and launcher shell syntax: passed.
- Benchmark parsing requires three samples and exactly one benchmark identity; ambiguity and incomplete samples are rejected.
- Unsafe paths, symlinks, non-Go and test-file edits are rejected.
- Evidence modification is detected by checksum verification.
- Literal loopback model endpoints are required; remote endpoints are rejected.
- Structured model requests and response parsing are exercised without external services.
- Lifecycle tests retain failed observations, rank accepted candidates, label demo mode, and preserve original source.
- HTTP Host, Origin and mutation token protections reject unsafe requests.
- Exported patches are checked with `git apply --check` in an isolated directory.

## Scripted pipeline trial

Run `RUN-20260920-112546.184667000` used predefined demo proposals, with real Docker tests and benchmarks. It rejected a deliberately behavior-breaking candidate and found a best **96.91%** reduction. This validates the pipeline and rejection handling, not autonomous reasoning. An initial run exposed a non-executable container tmpfs; executable test binaries were then allowed only in the disposable container `/tmp`.

## Live local-model trial

Delivered validation run: **`RUN-20260920-113216.227485000`** in `examples/allocdemo/.veda/runs/`.

| Observation | Measured result |
|---|---:|
| Baseline median | 464,262 B/op |
| Winning candidate median | 24,554 B/op |
| Allocation reduction | 94.71% |
| Hypotheses / experiments | 3 / 3 |
| Rejected compile failures | 2 |
| Accepted candidate | EXP-000003 |
| Existing tests | Passed for baseline and winner |

The model generated the hypotheses and full source replacements through the loopback llama.cpp endpoint. Prior failed outcomes were available through SQLite memory. All tests ran in network-disabled Docker containers. No external AI provider was called.

The accepted code changes repeated string concatenation to a `strings.Builder`. The model's hypothesis labels and explanations are unverified proposals; a SUPPORTED result means that the **candidate** met the metric target while passing existing tests. It does not establish the truth of the model's claimed mechanism or statistical significance.

An earlier live run (`RUN-20260920-112904.532706000`) found the same reduction but exposed nonportable patch paths. Its historical evidence was retained. The exporter was corrected, a patch regression test was added, and the delivered validation run was repeated with portable paths. Use the delivered run above when reviewing or applying a patch.

## Independent reproduction

`veda reproduce --workspace examples/allocdemo RUN-20260920-113216.227485000 EXP-000003` passed tests again. All three new samples measured **24,554 B/op** and **411 allocs/op**. Median runtime was 13,508 ns/op; timing varied, so allocation is the claimed outcome.

Reproduction evidence: `.veda/reproductions/RUN-20260920-113337.625666000/` under the example workspace.

All baseline/experiment manifests for the delivered run verified. The best patch was applied to a separate temporary copy and the resulting source matched the measured candidate byte for byte. Original example source was independently compared with the original snapshot and remained unchanged.

## Browser and operational checks

- Created and completed the live investigation using the dashboard.
- Inspected candidate diffs, real stdout and evidence-integrity status.
- Verified research memory and hardware/model readiness screens.
- Started and cancelled a real investigation from the UI; it persisted as cancelled.
- Restarted the dashboard and verified the saved history remained accessible.
- Inspected the desktop layout and a 390 × 844 viewport: no horizontal overflow.
- Browser error/warning log was empty during the checked workflow.

## Scope of this evidence

Validation used the bundled small Go module, not a broad set of unfamiliar repositories. Wi-Fi was not physically disabled; offline experiment execution was enforced through Docker `--network=none` and loopback-only inference. Three short benchmark samples screen candidates and are not a substitute for longer production benchmarks or comprehensive behavioral tests. Two invalid proposals were rejected, so this is a working prototype rather than a claim that every model proposal is correct.

Runtime databases, weights, logs and source snapshots remain local and are intentionally excluded from Git.
