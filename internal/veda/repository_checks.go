package veda

import (
	"context"
	"encoding/json"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"golang.org/x/mod/modfile"
)

var checkImages = map[string]string{"Go": DefaultImage, "Python": "python:3.12-alpine", "JavaScript": "node:22-alpine"}

func repoLanguages(repo Repository) map[string]bool {
	out := map[string]bool{}
	for _, f := range repo.Files {
		out[f.Language] = true
	}
	return out
}
func (h *Hub) prepareRuntimes(ctx context.Context, r *Investigation) {
	langs := repoLanguages(r.Repository)
	for _, lang := range []string{"Go", "Python", "JavaScript"} {
		if !langs[lang] {
			continue
		}
		tag := checkImages[lang]
		if lang == "Go" && h.Config.Image != "" {
			tag = h.Config.Image
		}
		_ = h.step(r, "Prepare "+lang+" runtime", "environment", "Official runtime image; no repository commands run during image preparation", func(x *AuditExperiment) {
			check, cancel := context.WithTimeout(ctx, 5*time.Second)
			id, err := DockerReady(check, tag)
			cancel()
			if err != nil {
				ready, cancel := context.WithTimeout(ctx, 5*time.Second)
				probe := exec.CommandContext(ready, "docker", "info", "--format", "{{.ServerVersion}}").Run()
				cancel()
				if probe == nil {
					pull, cancel := context.WithTimeout(ctx, 2*time.Minute)
					x.Command = []string{"docker", "pull", tag}
					out := &boundedBuffer{limit: 8000}
					cmd := exec.CommandContext(pull, "docker", "pull", tag)
					cmd.Stdout = out
					cmd.Stderr = out
					err = cmd.Run()
					cancel()
					x.Stdout = out.String()
					if err == nil {
						id, err = DockerReady(ctx, tag)
					}
				}
			}
			if err != nil {
				x.Status = "blocked"
				x.Conclusion = "Runtime unavailable; source analysis will continue."
				x.Stderr = err.Error()
				r.Images[lang] = "unavailable"
				r.Limits = append(r.Limits, lang+" runtime unavailable.")
			} else {
				r.Images[lang] = id
				x.Conclusion = lang + " checks pinned to " + id
			}
		})
	}
}
func (h *Hub) staticChecks(r *Investigation) {
	_ = h.step(r, "Validate captured source syntax", "static", "Go parser and JSON decoder; these checks do not execute repository code", func(x *AuditExperiment) {
		count := 0
		errors := []string{}
		for _, f := range r.Repository.Files {
			text := r.Repository.Content[f.Path]
			var err error
			if strings.HasSuffix(f.Path, ".go") {
				_, err = parser.ParseFile(token.NewFileSet(), f.Path, text, parser.AllErrors)
				count++
			} else if strings.HasSuffix(f.Path, ".json") {
				var value any
				err = json.Unmarshal([]byte(text), &value)
				count++
			} else {
				continue
			}
			if err != nil {
				message := f.Path + ": " + err.Error()
				errors = append(errors, message)
				r.Findings = append(r.Findings, Finding{Title: "Source parsing failed", Severity: "error", Path: f.Path, Line: 1, Evidence: err.Error(), Basis: "observed parser result"})
			}
		}
		x.Conclusion = fmt.Sprintf("Parsed %d Go/JSON files; %d errors.", count, len(errors))
		x.Stdout = strings.Join(errors, "\n")
		if len(errors) > 0 {
			x.Status = "failed"
		}
		if count == 0 {
			x.Status = "skipped"
			x.Conclusion = "No Go or JSON files captured; infrastructure parsing is shown in the architecture experiment."
		}
	})
}
func isolatedArgs(name, source, image string) []string {
	return []string{"run", "--pull=never", "--name", name, "--rm", "--network=none", "--read-only", "--cap-drop=ALL", "--security-opt=no-new-privileges", "--pids-limit=256", "--memory=2g", "--memory-swap=2g", "--cpus=2", "--user=65534:65534", "--tmpfs=/tmp:rw,exec,nosuid,size=768m,mode=1777", "--tmpfs=/work:rw,exec,nosuid,size=128m,mode=1777", "--mount", "type=bind,src=" + source + ",dst=/src,readonly"}
}
func containerCheck(ctx context.Context, source, image, script string, extra []string, x *AuditExperiment) {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	name := "veda-check-" + strings.ToLower(strings.ReplaceAll(NewID(), ".", "-"))
	args := isolatedArgs(name, source, image)
	args = append(args, extra...)
	args = append(args, image, "sh", "-c", script)
	x.Command = append([]string{"docker"}, args...)
	out := &boundedBuffer{limit: 64000}
	stderr := &boundedBuffer{limit: 64000}
	cmd := exec.CommandContext(ctx, "docker", args...)
	cmd.Stdout = out
	cmd.Stderr = stderr
	err := cmd.Run()
	if ctx.Err() != nil {
		clean, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		_ = exec.CommandContext(clean, "docker", "rm", "-f", name).Run()
		cancel()
	}
	x.Stdout = out.String()
	x.Stderr = stderr.String()
	if err != nil {
		x.Status = "failed"
		x.Conclusion = "Check did not pass: " + err.Error()
		if strings.Contains(x.Stderr, "module lookup disabled") || strings.Contains(x.Stderr, "cannot find module") || strings.Contains(x.Stderr, "no required module") || strings.Contains(x.Stderr, "requires go >=") {
			x.Status = "blocked"
			x.Conclusion = "Check blocked by dependencies or runtime version."
		}
	} else {
		x.Status = "passed"
		x.Conclusion = "Fixed check completed successfully."
	}
}

// Download modules in a separate container that sees manifests only. Repository
// code and credentials are never mounted in this network-enabled phase.
func (h *Hub) prepareGoDependencies(ctx context.Context, r *Investigation, module, image string) string {
	manifest := r.Repository.Content[filepath.ToSlash(filepath.Join(module, "go.mod"))]
	parsed, err := modfile.Parse("go.mod", []byte(manifest), nil)
	if err != nil || len(parsed.Require) == 0 {
		return ""
	}
	archive := ""
	_ = h.step(r, "Resolve Go dependencies · "+module, "dependencies", "Only go.mod and go.sum are mounted; downloads use the public Go proxy and checksum database. Repository code is not executed.", func(x *AuditExperiment) {
		base := filepath.Join(filepath.Dir(r.Repository.Root), "dependencies", digest(module)[:16])
		metadata, output := filepath.Join(base, "metadata"), filepath.Join(base, "output")
		if err := os.MkdirAll(metadata, 0755); err != nil {
			x.Status = "blocked"
			x.Stderr = err.Error()
			return
		}
		if err := os.MkdirAll(output, 0777); err != nil {
			x.Status = "blocked"
			x.Stderr = err.Error()
			return
		}
		if err := os.Chmod(output, 0777); err != nil {
			x.Status = "blocked"
			x.Stderr = err.Error()
			return
		}
		for _, name := range []string{"go.mod", "go.sum"} {
			if body, ok := r.Repository.Content[filepath.ToSlash(filepath.Join(module, name))]; ok {
				if err := os.WriteFile(filepath.Join(metadata, name), []byte(body), 0644); err != nil {
					x.Status = "blocked"
					x.Stderr = err.Error()
					return
				}
			}
		}
		name := "veda-deps-" + strings.ToLower(NewID())
		args := isolatedArgs(name, metadata, image)
		for i, arg := range args {
			if arg == "--network=none" {
				args[i] = "--network=bridge"
			}
		}
		args = append(args, "--mount", "type=bind,src="+output+",dst=/out", "-e", "GOMODCACHE=/tmp/gomodcache", "-e", "GOCACHE=/tmp/gocache", "-e", "GOPROXY=https://proxy.golang.org", "-e", "GOSUMDB=sum.golang.org", "-e", "GOTOOLCHAIN=local", image, "sh", "-c", `set -eu
cp /src/go.* /work/
cd /work
go mod download
chmod -R u+w /tmp/gomodcache
tar -C /tmp/gomodcache -cf /out/modules.tar .
chmod 644 /out/modules.tar
`)
		x.Command = append([]string{"docker"}, args...)
		limited, cancel := context.WithTimeout(ctx, 2*time.Minute)
		defer cancel()
		out, stderr := &boundedBuffer{limit: 16000}, &boundedBuffer{limit: 16000}
		cmd := exec.CommandContext(limited, "docker", args...)
		cmd.Stdout = out
		cmd.Stderr = stderr
		err := cmd.Run()
		x.Stdout = out.String()
		x.Stderr = stderr.String()
		if limited.Err() != nil {
			clean, stop := context.WithTimeout(context.Background(), 10*time.Second)
			_ = exec.CommandContext(clean, "docker", "rm", "-f", name).Run()
			stop()
		}
		if err != nil {
			x.Status = "blocked"
			x.Conclusion = "Dependency download unavailable; offline tests will retain the exact blocker."
			return
		}
		archive = output
		x.Conclusion = "Dependencies downloaded from the Go proxy and verified by the Go toolchain; test execution remains offline."
	})
	return archive
}
func (h *Hub) runtimeChecks(ctx context.Context, r *Investigation) {
	langs := repoLanguages(r.Repository)
	for _, lang := range []string{"Python", "JavaScript"} {
		if !langs[lang] {
			continue
		}
		lang := lang
		_ = h.step(r, lang+" syntax check", "runtime", "Isolated parser checks only; application test suites and package install scripts are not executed", func(x *AuditExperiment) {
			image := r.Images[lang]
			if image == "" || image == "unavailable" {
				x.Status = "blocked"
				x.Conclusion = "Runtime image unavailable."
				return
			}
			script := `python - <<'PY'
import ast,pathlib,sys
count=0; errors=0
for p in sorted(pathlib.Path('/src').rglob('*.py')):
 try: ast.parse(p.read_text(),filename=str(p.relative_to('/src'))); count+=1
 except (SyntaxError,UnicodeError) as error: print(error); errors+=1
print(f'Parsed {count} Python files; {errors} errors. Application tests were not run.')
sys.exit(bool(errors))
PY`
			if lang == "JavaScript" {
				script = `node - <<'JS'
const fs=require('fs'),path=require('path'),cp=require('child_process');let count=0,failed=0;
function walk(dir){for(const e of fs.readdirSync(dir,{withFileTypes:true})){const p=path.join(dir,e.name);if(e.isDirectory())walk(p);else if(/\.(js|cjs|mjs)$/.test(e.name)){count++;const r=cp.spawnSync(process.execPath,['--check',p],{encoding:'utf8',timeout:10000,maxBuffer:64000});if(r.status!==0){failed++;console.log((r.stderr||String(r.error)).replaceAll('/src/',''));}}}}
walk('/src');console.log(` + "`Parsed ${count} JavaScript files; ${failed} errors. Application tests were not run.`" + `);process.exit(failed?1:0);
JS`
			}
			containerCheck(ctx, r.Repository.Root, image, script, nil, x)
		})
		r.Limits = append(r.Limits, lang+" runtime validation checks syntax only; framework/application tests were not run.")
		if ctx.Err() != nil {
			return
		}
	}
	modules := []string{}
	for _, f := range r.Repository.Files {
		if filepath.Base(f.Path) == "go.mod" {
			modules = append(modules, filepath.Dir(f.Path))
		}
	}
	sort.Strings(modules)
	if len(modules) > 4 {
		modules = modules[:4]
		r.Limits = append(r.Limits, "Runtime budget covers the first four Go modules.")
	}
	for _, module := range modules {
		if ctx.Err() != nil {
			return
		}
		module := module
		cache := ""
		if image := r.Images["Go"]; image != "" && image != "unavailable" {
			cache = h.prepareGoDependencies(ctx, r, module, image)
		}
		_ = h.step(r, "Go tests · "+module, "runtime", "Existing tests run unchanged, without network or credentials; missing dependencies are reported as blocked", func(x *AuditExperiment) {
			image := r.Images["Go"]
			if image == "" || image == "unavailable" {
				x.Status = "blocked"
				x.Conclusion = "Go runtime unavailable."
				return
			}
			source := filepath.Join(r.Repository.Root, module)
			extra := []string{"-e", "GOCACHE=/tmp/gocache", "-e", "GOMODCACHE=/tmp/gomodcache", "-e", "GOTOOLCHAIN=local", "-e", "GOPROXY=off", "-e", "GOSUMDB=off", "-e", "CGO_ENABLED=0", "-e", "GOMAXPROCS=2"}
			script := `set -eu
if [ -f /dependencies/modules.tar ]; then mkdir -p /tmp/gomodcache; tar -C /tmp/gomodcache -xf /dependencies/modules.tar; fi
cp -R /src/. /work/
cd /work
export GOFLAGS=-mod=readonly
go test ./... -count=1 -timeout=60s
go vet ./...
`
			if cache != "" {
				extra = append(extra, "--mount", "type=bind,src="+cache+",dst=/dependencies,readonly")
			}
			containerCheck(ctx, source, image, script, extra, x)
		})
	}
	if langs["TypeScript / JSX"] {
		r.Limits = append(r.Limits, "TypeScript / JSX was inventoried and available to model review; no framework compiler was run.")
	}
}

type RepositoryReview struct {
	Summary  string    `json:"summary"`
	Findings []Finding `json:"findings"`
}

func (m LocalModel) Review(ctx context.Context, prompt string) (RepositoryReview, string, error) {
	text := func(n int) map[string]any { return map[string]any{"type": "string", "maxLength": n} }
	schema := object(map[string]any{"summary": text(1000), "findings": map[string]any{"type": "array", "maxItems": 4, "items": object(map[string]any{"title": text(180), "severity": map[string]any{"type": "string", "enum": []string{"error", "warning", "info"}}, "path": text(300), "line": map[string]any{"type": "integer", "minimum": 1}, "evidence": text(500)}, "title", "severity", "path", "line", "evidence")}}, "summary", "findings")
	raw, err := m.call(ctx, prompt+"\nAnswer the objective in a concise 2-4 sentence summary without repetition. Distinguish observations from hypotheses. Cite only paths and line numbers present in supplied source. Do not assert tests passed without observed evidence. Return up to 4 actionable findings; omit speculative issues unsupported by the excerpts. Infrastructure is declared, not necessarily deployed.", schema)
	var review RepositoryReview
	if err == nil {
		err = json.Unmarshal([]byte(raw), &review)
	}
	return review, raw, err
}
func reviewPrompt(r Investigation, previous []Investigation) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Objective: %s\nRepository: %s at %s\nDeclared components: %d\n", r.Request.Objective, r.Repository.Name, r.Repository.Commit, len(r.Architecture.Nodes))
	for _, x := range r.Experiments {
		fmt.Fprintf(&b, "Evidence: %s [%s]: %.700s %.800s\n", x.Title, x.Status, x.Conclusion, x.Stderr)
	}
	for i, p := range previous {
		if i >= 2 {
			break
		}
		fmt.Fprintf(&b, "Previous knowledge at this SAME commit (untrusted context): %.800s\n", p.Summary)
	}
	// Prioritize manifest/infrastructure and source; cap source context for the installed model.
	files := append([]RepoFile(nil), r.Repository.Files...)
	var targeted []RepoFile
	for _, f := range files {
		if strings.Contains(strings.ToLower(r.Request.Objective), strings.ToLower(f.Path)) {
			targeted = append(targeted, f)
		}
	}
	if len(targeted) > 0 {
		files = targeted
	}
	sort.SliceStable(files, func(i, j int) bool {
		score := func(f RepoFile) int {
			if strings.Contains(strings.ToLower(r.Request.Objective), strings.ToLower(f.Path)) {
				return -1
			}
			if f.Language == "Terraform" || f.Language == "Docker" || f.Language == "YAML" {
				return 0
			}
			if f.Language == "Documentation" {
				return 2
			}
			return 1
		}
		return score(files[i]) < score(files[j])
	})
	for _, f := range files {
		if b.Len() > 11500 {
			break
		}
		text := r.Repository.Content[f.Path]
		if len(text) > 5000 {
			text = text[:5000]
		}
		fmt.Fprintf(&b, "\nSOURCE %s (excerpt):\n", f.Path)
		for line, s := range strings.Split(text, "\n") {
			if b.Len() > 12000 {
				break
			}
			fmt.Fprintf(&b, "%d: %s\n", line+1, s)
		}
	}
	return b.String()
}
func (h *Hub) optimize(ctx context.Context, r *Investigation, dir string) {
	module := r.Request.Module
	root := filepath.Join(r.Repository.Root, module)
	benchmark := r.Request.Benchmark
	if benchmark == "" {
		names := []string{}
		for _, f := range r.Repository.Files {
			if !strings.HasSuffix(f.Path, "_test.go") || module != "" && !strings.HasPrefix(f.Path, module+"/") {
				continue
			}
			file, err := parser.ParseFile(token.NewFileSet(), f.Path, r.Repository.Content[f.Path], 0)
			if err != nil {
				continue
			}
			for _, decl := range file.Decls {
				if fn, ok := decl.(*ast.FuncDecl); ok && strings.HasPrefix(fn.Name.Name, "Benchmark") {
					names = append(names, fn.Name.Name)
				}
			}
		}
		sort.Strings(names)
		if len(names) > 0 {
			benchmark = "^" + names[0] + "$"
		}
	}
	if benchmark == "" {
		_ = h.event(r, "optimizing", "Checking benchmark availability.")
		_ = h.step(r, "Go optimization", "candidate", "Existing Go benchmark required", func(x *AuditExperiment) {
			x.Status = "blocked"
			x.Conclusion = "No Go benchmark found. Add a benchmark or select a module with one."
		})
		r.Limits = append(r.Limits, "Optimization requires an existing Go benchmark.")
		return
	}
	_ = h.event(r, "optimizing", "Running isolated Go optimization candidates; each candidate retains tests, measurements and a patch.")
	_ = h.step(r, "Go benchmark investigation", "candidate", "Existing benchmark "+benchmark+"; original tests are protected", func(x *AuditExperiment) {
		c, err := Init(root)
		if err != nil {
			x.Status = "blocked"
			x.Stderr = err.Error()
			r.Limits = append(r.Limits, "Optimization unavailable: "+err.Error())
			return
		}
		c.Endpoint = h.Config.Endpoint
		c.Model = h.Config.Model
		c.Image = h.Config.Image
		c.Benchmark = benchmark
		store, err := OpenStore(root)
		if err != nil {
			x.Status = "failed"
			x.Stderr = err.Error()
			return
		}
		defer store.DB.Close()
		engine := Engine{Root: root, Store: store, Model: LocalModel{c.Endpoint, c.Model}, Runner: DockerRunner{}, Log: func(msg string) { _ = h.event(r, "optimizing", msg) }}
		run, err := engine.Run(ctx, Options{Objective: r.Request.Objective, Config: c})
		x.Stdout = run.Report
		x.Metrics = run.Baseline
		if err != nil {
			x.Status = "blocked"
			x.Stderr = err.Error()
			x.Conclusion = "Benchmark investigation did not complete."
			r.Limits = append(r.Limits, x.Conclusion)
		} else {
			x.Conclusion = fmt.Sprintf("%d candidate experiments; best improvement %.2f%%", len(run.Experiments), run.Improvement)
		}
		// Candidate detail is persisted after the enclosing step finishes.
		for _, exp := range run.Experiments {
			p := filepath.Join(root, ".veda", "runs", run.ID, "experiments", exp.ID)
			read := func(name string) string {
				b, _ := os.ReadFile(filepath.Join(p, name))
				if len(b) > 64000 {
					b = b[:64000]
				}
				return string(b)
			}
			details := AuditExperiment{ID: "OPT-" + exp.ID, Title: exp.Hypothesis, Kind: "candidate", Status: exp.Status, Input: exp.Rationale, Stdout: read("stdout.log"), Stderr: read("stderr.log"), Patch: read("diff.patch"), Metrics: exp.Measurement, Conclusion: exp.Error, Parent: x.ID}
			var command struct {
				Program string   `json:"program"`
				Args    []string `json:"args"`
			}
			if json.Unmarshal([]byte(read("command.json")), &command) == nil && command.Program != "" {
				details.Command = append([]string{command.Program}, command.Args...)
			}
			if prompt := read("prompt.txt"); prompt != "" {
				details.Input = prompt
			}
			details.Response = read("response.txt")
			if exp.Measurement != nil {
				details.Duration = exp.Measurement.Duration
			}
			if details.Conclusion == "" {
				details.Conclusion = fmt.Sprintf("%s · %.2f%% reduction in %s against baseline; three benchmark samples.", exp.Status, exp.Improvement, c.Metric)
			}
			_ = WriteJSON(filepath.Join(dir, details.ID+".json"), details)
		}
	})
	entries, _ := filepath.Glob(filepath.Join(dir, "OPT-*.json"))
	for _, p := range entries {
		data, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		var x AuditExperiment
		if json.Unmarshal(data, &x) == nil {
			r.Experiments = append(r.Experiments, x)
			taskEvent(r, "task.recorded", x)
		}
	}
	_ = h.save(r)
}
