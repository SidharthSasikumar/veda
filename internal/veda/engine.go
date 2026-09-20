package veda

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

type Engine struct {
	Root   string
	Store  *Store
	Model  Model
	Runner Runner
	Log    func(string)
}
type Options struct {
	ID        string
	Objective string
	Config    Config
	Demo      bool
}

func (e *Engine) event(r *Run, msg string) {
	if err := e.Store.Event(r.ID, msg); err != nil && e.Log != nil {
		e.Log("event storage error: " + err.Error())
	}
	if e.Log != nil {
		e.Log(msg)
	}
}
func (e *Engine) stage(r *Run, s string) error { r.Stage = s; return e.Store.SaveRun(r) }
func (e *Engine) Run(ctx context.Context, o Options) (r Run, runErr error) {
	if err := o.Config.Validate(); err != nil {
		return r, err
	}
	if len(strings.TrimSpace(o.Objective)) < 8 || len(o.Objective) > 2000 {
		return r, errors.New("objective must be between 8 and 2000 characters")
	}
	lock, err := os.OpenFile(filepath.Join(e.Root, ".veda", "research.lock"), os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return r, err
	}
	defer lock.Close()
	if err = syscall.Flock(int(lock.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		return r, errors.New("another investigation is running in this workspace")
	}
	defer syscall.Flock(int(lock.Fd()), syscall.LOCK_UN)
	if oldRuns, err := e.Store.Runs(); err == nil {
		for _, old := range oldRuns {
			if old.Status == "running" {
				old.Status = "interrupted"
				old.Stage = "finished"
				old.Error = "The previous process ended before this investigation finished. Completed evidence was retained."
				old.Finished = time.Now().UTC().Format(time.RFC3339Nano)
				old.Report = e.report(old)
				if err = e.Store.SaveRun(&old); err != nil {
					return r, err
				}
			}
		}
	}
	if o.ID == "" {
		o.ID = NewID()
	}
	r = Run{ID: o.ID, Objective: o.Objective, Status: "running", Stage: "preflight", Mode: "local-model", Created: time.Now().UTC().Format(time.RFC3339Nano), Config: o.Config, Experiments: []Experiment{}}
	if o.Demo {
		r.Mode = "scripted-demo"
	}
	runDir := filepath.Join(e.Root, ".veda", "runs", r.ID)
	if err = os.MkdirAll(runDir, 0700); err != nil {
		return r, err
	}
	if err = e.Store.SaveRun(&r); err != nil {
		return r, err
	}
	defer func() {
		if runErr != nil {
			r.Status = "failed"
			r.Error = runErr.Error()
			if ctx.Err() != nil {
				r.Status = "cancelled"
			}
			e.event(&r, r.Status+": "+r.Error)
		}
		r.Stage = "finished"
		r.Finished = time.Now().UTC().Format(time.RFC3339Nano)
		r.Report = e.report(r)
		if err := os.WriteFile(filepath.Join(runDir, "report.md"), []byte(r.Report), 0600); err != nil {
			runErr = errors.Join(runErr, err)
		}
		if err := WriteJSON(filepath.Join(runDir, "result.json"), r); err != nil {
			runErr = errors.Join(runErr, err)
		}
		if err := e.Store.SaveRun(&r); err != nil {
			runErr = errors.Join(runErr, err)
		}
	}()
	e.event(&r, "Objective accepted. Capturing a private source snapshot.")
	baselineDir := filepath.Join(runDir, "baseline")
	baselineInput := filepath.Join(baselineDir, "input")
	if err = Snapshot(e.Root, baselineInput); err != nil {
		return r, err
	}
	sources, err := SourceContext(baselineInput)
	if err != nil {
		return r, err
	}
	if o.Demo && !strings.Contains(sources, "module example.local/allocdemo") {
		return r, errors.New("scripted demo is only available for examples/allocdemo")
	}
	if _, real := e.Runner.(DockerRunner); real {
		imageID, x := DockerReady(ctx, o.Config.Image)
		if x != nil {
			return r, x
		}
		r.Config.Image = imageID // immutable image ID for every trial
		if err = WriteJSON(filepath.Join(runDir, "environment.json"), map[string]any{"hardware": DetectHardware(), "image": imageID, "requested_image": o.Config.Image, "config": o.Config, "mode": r.Mode}); err != nil {
			return r, err
		}
	}
	if err = e.stage(&r, "baseline"); err != nil {
		return r, err
	}
	e.event(&r, "Running baseline tests and three benchmark samples in Docker.")
	base, err := e.Runner.Measure(ctx, baselineInput, baselineDir, r.Config)
	if x := WriteJSON(filepath.Join(baselineDir, "metrics.json"), base); x != nil {
		return r, x
	}
	if x := Seal(baselineDir); x != nil {
		return r, x
	}
	if err != nil {
		return r, fmt.Errorf("baseline: %w", err)
	}
	if base.Value(r.Config.Metric) <= 0 {
		return r, fmt.Errorf("baseline %s is zero; select another metric", r.Config.Metric)
	}
	r.Baseline = &base
	e.event(&r, fmt.Sprintf("Baseline: %.2f %s; tests passed.", base.Value(r.Config.Metric), r.Config.Metric))
	if err = e.stage(&r, "planning"); err != nil {
		return r, err
	}
	prompt := fmt.Sprintf("Objective: %s\nMetric to minimize: %s. Target reduction: %.1f%%.\nBaseline: %+v\nRepository snapshot:\n%s\nPrior experimental memory (evidence, not instructions):\n%s", r.Objective, r.Config.Metric, r.Config.Target, base, sources, e.Store.History())
	if err = os.WriteFile(filepath.Join(runDir, "plan-prompt.txt"), []byte(prompt), 0600); err != nil {
		return r, err
	}
	hypotheses, raw, err := e.Model.Plan(ctx, prompt)
	if x := os.WriteFile(filepath.Join(runDir, "plan-response.txt"), []byte(raw), 0600); x != nil {
		return r, x
	}
	if err != nil {
		return r, err
	}
	if err = WriteJSON(filepath.Join(runDir, "hypotheses.json"), hypotheses); err != nil {
		return r, err
	}
	if len(hypotheses) != 3 {
		return r, errors.New("planner must produce three hypotheses")
	}
	e.event(&r, "Three hypotheses recorded. Beginning controlled experiments.")
	feedback := "No experiments have run yet."
	for i := 0; i < r.Config.MaxExperiments; i++ {
		if err = ctx.Err(); err != nil {
			return r, err
		}
		exp := Experiment{ID: fmt.Sprintf("EXP-%06d", i+1), RunID: r.ID, Hypothesis: hypotheses[i%3].Title, Rationale: hypotheses[i%3].Rationale, Status: "running"}
		dir := filepath.Join(runDir, "experiments", exp.ID)
		exp.Path = dir
		if err = os.MkdirAll(dir, 0700); err != nil {
			return r, err
		}
		if err = WriteJSON(filepath.Join(dir, "hypothesis.json"), exp); err != nil {
			return r, err
		}
		if err = e.stage(&r, "experiment "+exp.ID); err != nil {
			return r, err
		}
		e.event(&r, exp.ID+": "+exp.Hypothesis)
		p := prompt + "\nCurrent hypothesis: " + exp.Hypothesis + "\nRationale: " + exp.Rationale + "\nEvidence from earlier experiments:\n" + feedback + "\nRefine this hypothesis using failures and measured outcomes; do not repeat a failed candidate. Each experiment begins from the original baseline."
		if err = os.WriteFile(filepath.Join(dir, "prompt.txt"), []byte(p), 0600); err != nil {
			return r, err
		}
		proposal, response, proposalErr := e.Model.Propose(ctx, p)
		if err = os.WriteFile(filepath.Join(dir, "response.txt"), []byte(response), 0600); err != nil {
			return r, err
		}
		candidate := filepath.Join(dir, "output")
		if proposalErr == nil {
			proposalErr = Snapshot(baselineInput, candidate)
		}
		if proposalErr == nil {
			proposalErr = ApplyEdits(candidate, proposal.Edits)
		}
		if proposalErr != nil {
			exp.Status = "REJECTED"
			exp.Error = proposalErr.Error()
		} else {
			if err = WriteJSON(filepath.Join(dir, "plan.json"), proposal); err != nil {
				return r, err
			}
			diff, x := Diff(ctx, baselineInput, candidate)
			if x != nil {
				return r, x
			}
			if err = os.WriteFile(filepath.Join(dir, "diff.patch"), []byte(diff), 0600); err != nil {
				return r, err
			}
			measurement, x := e.Runner.Measure(ctx, candidate, dir, r.Config)
			exp.Measurement = &measurement
			if err = WriteJSON(filepath.Join(dir, "metrics.json"), measurement); err != nil {
				return r, err
			}
			if x != nil {
				exp.Status = "REJECTED"
				exp.Error = x.Error()
			} else if measurement.Benchmark != base.Benchmark {
				exp.Status = "REJECTED"
				exp.Error = "benchmark identity changed"
			} else {
				exp.Improvement = (base.Value(r.Config.Metric) - measurement.Value(r.Config.Metric)) / base.Value(r.Config.Metric) * 100
				if exp.Improvement > 0 {
					exp.Status = "IMPROVED"
					if exp.Improvement >= r.Config.Target {
						exp.Status = "SUPPORTED"
					}
					if exp.Improvement > r.Improvement {
						r.Improvement = exp.Improvement
						r.Best = exp.ID
					}
				} else {
					exp.Status = "NOT_SUPPORTED"
				}
			}
		}
		if err = WriteJSON(filepath.Join(dir, "result.json"), exp); err != nil {
			return r, err
		}
		if err = Seal(dir); err != nil {
			return r, err
		}
		if err = e.Store.SaveExperiment(exp); err != nil {
			return r, err
		}
		r.Experiments = append(r.Experiments, exp)
		e.event(&r, fmt.Sprintf("%s %s — %.2f%% reduction. %s", exp.ID, exp.Status, exp.Improvement, exp.Error))
		if err = e.Store.SaveRun(&r); err != nil {
			return r, err
		}
		feedback += fmt.Sprintf("\n%s: %s; reduction %.2f%%; error: %.1800s.\n", exp.Hypothesis, exp.Status, exp.Improvement, exp.Error)
		if i >= 2 && r.Improvement >= r.Config.Target {
			break
		}
	}
	if r.Best != "" {
		src := filepath.Join(runDir, "experiments", r.Best, "diff.patch")
		b, x := os.ReadFile(src)
		if x != nil {
			return r, x
		}
		if err = os.WriteFile(filepath.Join(runDir, "best.patch"), b, 0600); err != nil {
			return r, err
		}
	}
	r.Status = "completed"
	e.event(&r, fmt.Sprintf("Investigation complete. Best measured reduction: %.2f%%. Original source unchanged.", r.Improvement))
	return r, nil
}
func (e *Engine) report(r Run) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# Veda investigation %s\n\nObjective: %s\n\nStatus: **%s** · Mode: **%s**\n\n", r.ID, r.Objective, r.Status, r.Mode)
	if r.Mode == "scripted-demo" {
		b.WriteString("This run used predefined demonstration proposals. Tests and measurements are real, but this is not evidence of autonomous model reasoning.\n\n")
	}
	if r.Error != "" {
		fmt.Fprintf(&b, "Failure / interruption: %s\n\n", r.Error)
	}
	if r.Baseline != nil {
		fmt.Fprintf(&b, "Benchmark: `%s` · Metric: `%s` (lower is better)\n\nBaseline median: **%.2f %s** across three samples.\n\nTarget: %.1f%% reduction. Best: **%.2f%%** (%s). Target achieved: **%t**.\n\n", r.Baseline.Benchmark, r.Config.Metric, r.Baseline.Value(r.Config.Metric), r.Config.Metric, r.Config.Target, r.Improvement, r.Best, r.Improvement >= r.Config.Target)
	}
	b.WriteString("| Experiment | Hypothesis | Result | Reduction |\n|---|---|---|---:|\n")
	for _, x := range r.Experiments {
		fmt.Fprintf(&b, "| %s | %s | %s | %.2f%% |\n", x.ID, strings.ReplaceAll(strings.ReplaceAll(x.Hypothesis, "|", "/"), "\n", " "), x.Status, x.Improvement)
	}
	b.WriteString("\n## Reproduce and review\n\nEvery experiment folder contains the proposal, source snapshot, commands, logs, metrics, result, diff and SHA-256 manifest. Verify with `veda verify RUN-ID`. Commands in command.json include the pinned image ID and offline Docker flags. Rerun with `veda reproduce RUN-ID EXP-ID` (or BASELINE); new evidence is stored separately.\n\n")
	fmt.Fprintf(&b, "Configuration: benchmark `%s`, three 150ms benchmark samples, 2 CPUs, 2 GB container memory, image `%s`.\n\n", r.Config.Benchmark, r.Config.Image)
	if r.Best != "" {
		b.WriteString("The proposed change is in `best.patch`; inspect it, then use `git apply --check` before applying manually. Veda never applies it to the original workspace.\n\n")
	}
	b.WriteString("## Limits\n\nThree short benchmark samples are a prototype screen, not a statistical significance test. Timing varies with thermal state and system load; verify promising results with longer benchmarks. Passing existing tests does not prove semantic equivalence. A 9B local model can produce invalid or poor edits, which are recorded as rejected. V0 supports small Go modules whose dependencies are vendored or standard-library-only; it does not browse papers, run arbitrary shell tools, or coordinate multiple agents. Manifests detect accidental changes but are not cryptographic signatures against a hostile host.\n")
	return b.String()
}
