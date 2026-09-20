# Repository investigations

The repository hub accepts a GitHub URL, an objective, and an optional branch/tag. It resolves an exact commit, captures source, maps declared architecture, runs checks, asks the local model to interpret evidence, and saves the report. The UI updates after each step. Stop cancels work while retaining completed evidence.

## Architecture graph

Cytoscape.js 3.33.1 is bundled locally, including its license. No external UI CDN is needed. Native Terraform HCL parsing extracts resources, data sources, modules, variables, and their direct references within a module directory. Compose parsing extracts services, image/build links, dependencies, explicit networks, and named volume mounts. Dockerfiles show base images; Kubernetes manifests show resource identities. Go, Node, and Python manifests identify application components.

Nodes and edges include file/line citations with a captured-source inspector. These are source declarations, not live cloud resources or running-container state. Remote Terraform modules, interpolated build paths, Helm rendering, Terraform plan evaluation, and inferred application call graphs are not resolved. Docker stage aliases may appear as image labels. No infrastructure is deployed.

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
