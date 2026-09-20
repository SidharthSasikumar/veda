# Veda

A local repository investigation workspace: enter a GitHub URL and a prompt to get a source-backed architecture graph, isolated checks, a report, and reusable knowledge. Optional Go benchmark investigations add measured optimization candidates.

Veda runs on your own computer. It uses a local model through a replaceable OpenAI-compatible protocol; it does **not** use the OpenAI service, a paid search API, or a cloud database. One researcher works sequentially. SQLite remembers successes and failures.

## Start the installed prototype

Double-click **Launch Veda.command**, or run:

```sh
./scripts/start.sh
```

Open **http://127.0.0.1:8787**. The launcher starts the local model on **127.0.0.1:18080** when needed, and serves the research dashboard on port 8787. Keep that terminal open; Ctrl+C stops the session. Docker Desktop must be running. The first model load takes a few seconds; **System status** checks readiness.

Click **New investigation**, paste a GitHub repository URL, and describe what you want to understand. Veda clones its default branch (or a selected branch/tag), records the exact commit, and automatically builds a report. Public repos work directly; private repos use an existing `gh auth login` session.

- **Agent dashboard:** the original minion crew works at six stations in an interactive office. Select an investigation and character to inspect real tasks, evidence, and recorded handoffs. Follow active work, switch to a task timeline, or replay recorded events. Pause motion or use your system’s reduced-motion setting. Characters represent the existing sequential workflow; detailed replay is available for new investigations.
- **Architecture:** interactive Cytoscape.js graph of Terraform resources/references, Docker image builds, Compose services/dependencies/networks/volumes, Kubernetes resource identities, and application manifests. Select a component to inspect its source file and line. Search, filter, change layout, and export PNG.
- **Experiments:** a workflow graph with each step's inputs, status, duration, exact command, output, errors, and source evidence. Optional Go optimization exposes individual candidate patches and benchmark sample charts.
- **Changes:** GitHub-style split/unified diffs, changed-file navigation, viewed markers, validation evidence, and downloadable patches. New investigations can ask the local model for up to two independent suggestions against the captured commit. Existing benchmark patches are also available, including rejected candidates. Source changes are proposed for review; they are never applied to your repository automatically.
- **UI comparison:** supported static HTML pages get real before/after screenshots, a swipe comparison, and a pixel-difference image. Enable **Capture UI previews** and optionally set **Preview page**. Build the screenshot runtime once with `./scripts/setup-preview.sh`. Missing assets and unsupported framework/backend startup are disclosed; screenshots do not mean the application tests passed.
- **Report:** source structure, observed checks, separately labeled local-model interpretations, and coverage limits. Export Markdown or the full evidence JSON.
- **Knowledge:** all repositories share a local SQLite store at `~/Library/Application Support/Veda`. Matching repository, exact commit, objective, analyzer/configuration, and immutable runtime image IDs allow evidence reuse. **Refresh evidence** forces a fresh investigation. New objectives receive relevant prior summaries from the same revision.

The original Go benchmark workspace remains available under **Open benchmark lab**. **Scripted example demo** there uses fixed proposals with real Docker measurements, explicitly labeled as a demonstration.

See [repository workflow and coverage](docs/REPOSITORY_WORKFLOW.md) for supported checks and limits.

## First-time setup from GitHub

Requirements: macOS Apple Silicon, Go 1.26+, Git, Docker Desktop, approximately 7 GB for the model and image, and internet for the initial downloads. The installed build uses llama.cpp 0.4.0. Install required tools from their official sources; `brew install go llama.cpp` installs the compiler and inference runtime when Homebrew is present.

```sh
git clone https://github.com/SidharthSasikumar/veda.git
cd veda
./scripts/setup.sh
./scripts/start.sh
```

`setup.sh` builds the CLI, initializes the example, pulls the official Go image, downloads the pinned model revision, and verifies its SHA-256 checksum. After setup, standard-library and vendored experiments require no internet. No Docker images or model weights are committed to Git.

## Go benchmark investigations (CLI and benchmark lab)

Benchmark optimization supports a **small Go module** with tests and one selectable benchmark. Runtime source context is limited to 30 Go/module files and 18,000 bytes; the repository snapshot is limited to 32 MiB / 3,000 files. Large repositories should expose a smaller standalone benchmark module. Source is sent only to a literal loopback model endpoint.

```sh
./bin/veda init --workspace /path/to/go-module
./bin/veda research --workspace /path/to/go-module \
  --benchmark '^BenchmarkYourFunction$' --metric B/op --target 20 \
  'Reduce allocations without changing observable behavior'
```

Flags go **before** the objective. Supported metrics: `B/op`, `allocs/op`, `ns/op`. If the selector matches multiple benchmarks, Veda refuses the comparison rather than combining unrelated numbers. Dependencies must be standard-library-only or vendored in advance. Go toolchains are never downloaded during an experiment. Benchmark optimization does not support host execution or CGO projects inside experiments.

For a dashboard pointed at a different module:

```sh
./scripts/start.sh /path/to/go-module
```

The benchmark lab is scoped to this module. The repository hub accepts GitHub URLs independently and captures them inside the shared knowledge directory; HTTP requests cannot choose host filesystem paths.

## Evidence and reproduction

```sh
./bin/veda runs --workspace /path/to/go-module
./bin/veda report --workspace /path/to/go-module RUN-ID
./bin/veda verify --workspace /path/to/go-module RUN-ID
./bin/veda reproduce --workspace /path/to/go-module RUN-ID EXP-000003
./bin/veda reproduce --workspace /path/to/go-module RUN-ID BASELINE
```

Each run establishes a fresh baseline, proposes three distinct hypotheses, and starts every candidate from the original source snapshot. Prior failures and outcomes are supplied to later proposals. Three experiments are always attempted before stopping for a met target; you can set a budget of up to eight. The best candidate must pass all existing tests. A run can complete without meeting the target; reports state that explicitly.

```text
.veda/
  project.json                  # editable local configuration
  model.json                    # hardware-derived model profile
  memory.db                     # SQLite runs, experiments and observations
  research.lock                 # process-wide exclusive investigation lock
  runs/RUN-ID/
    environment.json            # image ID, config and hardware
    hypotheses.json
    plan-prompt.txt / plan-response.txt
    baseline/input/             # original private source snapshot
    baseline/{stdout.log,stderr.log,metrics.json,manifest.json}
    experiments/EXP-ID/
      hypothesis.json / plan.json
      prompt.txt / response.txt
      output/                   # complete candidate source
      command.json / diff.patch
      stdout.log / stderr.log / metrics.json / result.json
      manifest.json             # SHA-256 hashes of evidence files
    best.patch / report.md / result.json
  reproductions/RUN-ID/          # new output; original records stay intact
```

`best.patch` is a reviewable proposal. Veda never applies it to your source. Review it and use `git apply --check /path/to/best.patch` before applying it yourself. Hash manifests detect changed evidence; they are not signatures against a hostile local administrator. Patches use paths relative to the investigated module. For a module nested inside a larger Git repository, run `git apply --directory=path/to/module --check /path/to/best.patch` from the repository root. Runtime data and snapshots are ignored by Git. Repository-hub evidence is kept in the shared store described in the repository workflow guide; the CLI commands here address benchmark-lab runs.

## Model profile for this Mac

The prototype was configured for an **Apple M5 Pro, 24 GB unified memory, 15 CPU cores, 16 GPU cores**:

- **Qwen3.5-9B, Q4_K_M**, Unsloth GGUF, 5,680,522,464 bytes.
- **llama.cpp with Metal**, all layers offloaded, 8,192-token context, one model slot.
- Thinking disabled for concise structured proposals; 3,200 output-token budget.
- One 2-CPU / 2-GB Docker experiment at a time.

This is a conservative fit decision, not a guarantee about free memory or model quality. A 35B model is not the default on a 24 GB machine. Close other heavy applications if memory pressure rises. See [model details](docs/MODEL.md) for the pinned source and checksums.

`veda model setup --workspace PATH --profile auto` detects memory and saves the recommendation. `light` selects a 4B model; `balanced` selects the installed 9B model. Setup downloads only the validated 9B model. To use another profile, supply your own compatible GGUF with `VEDA_MODEL_FILE` and `VEDA_MODEL_ALIAS` to `scripts/start-model.sh`, and match the `model` field in `.veda/project.json`. Ollama, LM Studio and other compatible local servers can be used by changing that configuration; only llama.cpp was validated in this build.

## Development

```sh
./scripts/build.sh
go test ./...
go vet ./...
node --test web/agent-model.test.cjs web/office-model.test.cjs web/changes-model.test.cjs
```

The Go runtime owns planning, experiment orchestration, evaluation, memory, HTTP API, and the embedded dashboard. `internal/veda/model.go` defines the model interface. Go's existing tests and benchmarks provide experimental observations; the model does not supply its own measurements.

See [architecture and boundaries](docs/ARCHITECTURE.md), [validation evidence](docs/VALIDATION.md), and [source brief](docs/PROJECT_BRIEF.md).
