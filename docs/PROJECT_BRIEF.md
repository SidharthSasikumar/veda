# Veda project brief

Source: the [shared conversation](https://chatgpt.com/s/t_6aafbf992c288191942b5ea7626ab303), read in full on September 20, 2026. The concept was called Axiom in that conversation; the user subsequently named this project **Veda** and selected [SidharthSasikumar/veda](https://github.com/SidharthSasikumar/veda) as its repository.

## Implemented milestone

A Go-first, local-first experimentation engine: objective → baseline → three hypotheses → isolated experiments → measured evaluation → feedback to the next hypothesis → report. One agent, replaceable model interface, SQLite research memory, offline experiments, protected tests, saved source snapshots, reviewable diffs, and checksummed evidence. A local dashboard provides an accessible interface alongside the CLI.

The requested acceptance direction is an offline engineering investigation on an unfamiliar Go module with a measurable optimization objective. The prototype exposes this workflow for small Go modules, with explicit input limits and dependency prerequisites. Validation on the bundled example is reported separately from validation on an unfamiliar module.

## Deliberate V0 choices

- JSON configuration replaces the proposed YAML to avoid a configuration dependency.
- Private source copies replace Git worktrees so uncommitted files can be investigated without modifying the source repository or its Git metadata.
- Fixed Go test / benchmark commands replace arbitrary shell tools. Model output is constrained to source replacements. This keeps the first milestone bounded and reviewable.
- Three repeated samples and median comparisons provide a prototype screen; timing results are not formal statistical inference.
- A simple embedded browser UI complements the CLI without a frontend build tool or external CDN.

## Beyond this prototype

Web/paper research, PDF ingestion, arbitrary languages and commands, multi-agent execution, parallel research, semantic vector memory, automatic merging, robust long-run performance inference, and production multi-user hosting are future work.
