# Repository investigations

The repository hub accepts a GitHub URL, an objective, and an optional branch/tag. It resolves an exact commit, captures source, maps declared architecture, runs checks, asks the local model to interpret evidence, and saves the report. The UI updates after each step. Stop cancels work while retaining completed evidence.

## Virtual agent dashboard

The landing dashboard shows investigations as selectable cards with a small animated crew, current stage, recorded outcomes, filters, and search. The sidebar groups investigations by repository. Opening a card expands **Agent office** for that investigation. The original minion characters have six designated stations: source capture (Scout), architecture (Mapper), checks (Tester), local model review (Thinker), optional benchmarks (Tinkerer), and saved knowledge (Keeper).

Selecting a character shows its real tasks and latest recorded outcome. Task links open the existing experiment evidence view. **Follow active work** selects the current role; manual selection turns following off. Evidence handoffs bring the sending and receiving characters to the shared table, with links to the recorded tasks. A handoff means evidence is available to the next workflow stage; it does not imply an agent conversation. These are visual roles over Veda's sequential pipeline, not additional concurrently running models or autonomous processes. Animation follows live stage status; reused evidence, disabled stages, failures, and cancellation remain distinct. A report may be saved even when some checks fail.

**Timeline** shows recorded task intervals and links to evidence. Candidate outcomes imported after optimization have no invented start time. Reused knowledge has no fresh execution timeline. **Play recorded events** advances one event every 1.2 seconds; use the slider or select an activity to inspect a point in history, then **Return to latest**. Replay reconstructs only the task outcomes known at the selected event. Evidence links explicitly open the final saved result, which may include later outcomes. Replay never starts work or changes saved evidence. Earlier investigations retain their summary history; detailed replay is available for investigations started after this update.

The overview and office refresh every 2.5 seconds. New investigations store ordered version-1 events with run/task identities, role, timestamp, outcome snapshot, and handoff evidence IDs in the same checksummed record as their task state. Fast transitions remain available in history even when they occur between polls. Full outputs stay in the existing task evidence rather than being duplicated in every event. Connection loss pauses character motion and marks activity as last saved. **Pause motion** stops animations, and system `prefers-reduced-motion` is respected. Playback stops when leaving the office or hiding the page. Keyboard-accessible controls and text statuses complement the decorative characters. The office uses locally bundled HTML, CSS, and SVG; it adds no graphics runtime or model calls.

## Architecture graph

Cytoscape.js 3.33.1 is bundled locally, including its license. No external UI CDN is needed. Native Terraform HCL parsing extracts resources, data sources, modules, variables, and their direct references within a module directory. Compose parsing extracts services, image/build links, dependencies, explicit networks, and named volume mounts. Dockerfiles show base images; Kubernetes manifests show resource identities. Go, Node, and Python manifests identify application components.

Nodes and edges include file/line citations with a captured-source inspector. These are source declarations, not live cloud resources or running-container state. Remote Terraform modules, interpolated build paths, Helm rendering, Terraform plan evaluation, and inferred application call graphs are not resolved. Docker stage aliases may appear as image labels. No infrastructure is deployed.

## Suggested changes and visual review

**Changes** presents saved proposals as a file-by-file review with split or unified code diffs, original/new line numbers, additions/deletions, a file filter, viewed markers for the current page session, and patch downloads. Candidate status and recorded validation stay beside the diff. Rejected benchmark candidates remain rejected. **Open validation evidence** opens the exact originating experiment. Downloading a patch does not apply it or create a GitHub pull request.

**Suggest source changes** enables a bounded local-model drafting step after the evidence review. It returns up to two independent alternatives, each with at most three exact replacements in existing captured source files. Tests, manifests, dependencies, hidden configuration, and uncaptured files are protected. Every original block must match exactly once; no-op, malformed Go, ambiguous, and oversized edits are rejected. If every proposed change fails validation, one correction attempt receives the validation errors; both rejected and corrected proposals remain visible (at most four records). Suggestions are checked against private copies. Go syntax parsing and exact source matching do not establish application correctness: ordinary investigation tests apply to the original revision, and new suggestions explicitly remain untested. The separately measured Go benchmark engine retains its candidate-specific test evidence.

**Generate suggestions** starts a new investigation pinned to the selected run's full commit, leaving its original evidence unchanged. Repository loading supports a branch/tag or a full 40-character commit ID. **Refresh evidence** retains the request's original ref behavior. The analyzer/settings signature includes suggestion and preview settings, the selected page, and the resolved preview image ID.

UI-related file extensions trigger a possible visual-impact check. With **Capture UI previews** enabled, Veda selects a captured HTML entry page (or uses **Preview page**) and renders the original and one independent suggestion at 1280 × 900. The trusted Chromium harness serves captured files in a fresh, network-disabled, unprivileged Docker container with read-only source/root mounts and 2 CPU / 2 GB limits. It never runs repository installation, build, or server scripts. The browser has an ephemeral profile; animations are disabled for comparison where the page permits. Chromium's process sandbox is disabled inside the restricted container. No preview HTML is embedded in the Veda UI; only checksummed PNGs are served.

The screenshot runtime is optional: install it with `./scripts/setup-preview.sh`. Setup builds Node + Chromium from package repositories; each investigation records the immutable resulting image ID and capture timestamps. **Side by side**, **Swipe**, and **Difference** show the same recorded page/viewport. Magenta marks changed pixels; the percentage is an exact image comparison, not a quality or correctness score. Captures are fixed-viewport images, not a full-page or interaction test. Page/renderer timing can still introduce incidental changes.

Static previews contain only bounded captured source. Excluded images, fonts, vendored/minified scripts, remote assets, APIs, and framework build outputs can be missing. Missing local resources and detected external assets produce a **Partial capture** warning; an absent HTML entry or renderer produces **Preview unavailable** with a reason. Backend-dependent and unbuilt framework applications are not fully supported. A zero visual difference can mean the changed code did not affect this page; it does not prove there is no UI impact elsewhere. The self-contained `examples/ui-review` fixture demonstrates the complete capture workflow.

PNG hashes, capture provenance, patch, rationale, and status remain in the common knowledge record; PNG files live under `investigations/RUN-ID/changes/CHG-ID/`. Reused screenshots point to their original run and are served only when repository/revision and checksums match.

## Automated checks

| Source | Automated validation |
| --- | --- |
| Go / JSON | Parse captured source without execution |
| Go modules | Download dependencies in a separate manifests-only container, then run existing tests and `go vet` offline |
| Python | Parse Python syntax in an isolated Python runtime |
| JavaScript | Run Node syntax checks, without application execution |
| TypeScript / JSX | Inventory and local-model review; framework build/test suites are not run |
| Terraform / Compose | Parse declared structure and relationships; no apply, plan, or deployment |

Official runtime images are downloaded if needed and resolved to immutable image IDs. Dependency preparation exposes only `go.mod` and `go.sum`, with the Go public proxy and checksum database enabled. It cannot access repository code or host credentials. Downloads use a bounded temporary filesystem; private/unavailable dependencies or unsupported toolchains remain explicit blockers.

Actual test containers have no network, read-only source mounts, a read-only root filesystem, an unprivileged user, no capabilities, and 2 CPU / 2 GB limits. At most four Go modules are tested per investigation. CGO is disabled. Repository install scripts and infrastructure commands are never automatically executed. Python/JavaScript checks do not claim application-test coverage. Missing Docker or model services produce visible blocked steps; source analysis remains available.

Optional Go benchmark optimization uses the existing three-candidate engine. It requires a small standalone Go module with an existing benchmark and dependencies available to that engine. Candidate inputs, logs, patches, and three-sample metrics remain separate. Dependency preparation for ordinary tests is not yet shared with the optimization engine.

## Shared knowledge and provenance

Default macOS storage:

```text
~/Library/Application Support/Veda/
  knowledge.db                 # all repository investigations, checksummed JSON records
  hub.lock                     # one server per shared store
  investigations/RUN-ID/
    checkout/                  # filtered Git object database, no checked-out files
    source/                    # bounded captured regular source files
    dependencies/              # manifests-only download artifacts
    report.md
    evidence.json
    OPT-EXP-*.json             # optional benchmark candidate details
```

Override with `VEDA_HOME` or `veda serve --knowledge-dir PATH`. The library currently lists the latest 100 investigations; older records remain stored. Artifacts stay local and are not added to the investigated GitHub repo. Source hashes are verified before displaying captured files. Payload checksums detect accidental evidence modification; they are not signatures against a privileged attacker.

Automatic reuse requires repository identity, exact commit, objective, analyzer/configuration and resolved runtime image IDs to match. It retains original evidence timestamps and shows a reuse banner. Blocked/incomplete investigations are not reused as completed results. Different objectives can receive earlier summaries at the same revision as context. A branch advancing to another commit triggers fresh evidence. Force refresh bypasses reuse.

The model alias and endpoint are part of configuration identity; replacing model weights under the same alias is not detected automatically. Use **Refresh evidence** after such a change. Knowledge is shared within this local installation, not synchronized across computers.

## Capture and coverage bounds

Capture is limited to 1,200 supported source files, 24 MiB total, and 128 KiB per file; repositories with more than 25,000 tree entries are rejected. Git cloning has a three-minute time limit and a blob-size filter. The filter does not impose a strict Git pack download-size limit. Links, submodules, hidden paths except `.github`, common credential/state filenames, dependencies, and generated directories are excluded. These filters are not a comprehensive secret scanner. Tests apply to the captured snapshot, which may omit required assets or vendor trees.

The local model receives approximately 18,000 characters of prioritized source excerpts and observed results; it does not review every line of a large repository. Model findings are interpretations requiring review, never substituted for measured test outcomes. Reports and experiment details show observed failures and coverage limits explicitly.
