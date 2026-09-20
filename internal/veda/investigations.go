package veda

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"
)

type InvestigationRequest struct {
	URL       string `json:"url"`
	Ref       string `json:"ref"`
	Objective string `json:"objective"`
	Force     bool   `json:"force"`
	Runtime   bool   `json:"runtime"`
	Model     bool   `json:"model"`
	Optimize  bool   `json:"optimize"`
	Benchmark string `json:"benchmark"`
	Module    string `json:"module"`
}
type Finding struct {
	Title    string `json:"title"`
	Severity string `json:"severity"`
	Path     string `json:"path,omitempty"`
	Line     int    `json:"line,omitempty"`
	Evidence string `json:"evidence"`
	Basis    string `json:"basis"`
}
type AuditExperiment struct {
	ID         string       `json:"id"`
	Title      string       `json:"title"`
	Kind       string       `json:"kind"`
	Status     string       `json:"status"`
	Started    string       `json:"started"`
	Duration   float64      `json:"duration_seconds"`
	Input      string       `json:"input"`
	Command    []string     `json:"command,omitempty"`
	Stdout     string       `json:"stdout,omitempty"`
	Stderr     string       `json:"stderr,omitempty"`
	Conclusion string       `json:"conclusion"`
	Evidence   []string     `json:"evidence,omitempty"`
	Parent     string       `json:"parent,omitempty"`
	Patch      string       `json:"patch,omitempty"`
	Response   string       `json:"model_response,omitempty"`
	Metrics    *Measurement `json:"metrics,omitempty"`
}
type Investigation struct {
	ID              string               `json:"id"`
	Request         InvestigationRequest `json:"request"`
	Status          string               `json:"status"`
	Stage           string               `json:"stage"`
	Created         string               `json:"created"`
	Finished        string               `json:"finished,omitempty"`
	Error           string               `json:"error,omitempty"`
	Repository      Repository           `json:"repository"`
	Architecture    Graph                `json:"architecture"`
	Experiments     []AuditExperiment    `json:"experiments"`
	Findings        []Finding            `json:"findings"`
	Events          []Event              `json:"events"`
	Summary         string               `json:"summary"`
	Report          string               `json:"report"`
	Limits          []string             `json:"limits"`
	Signature       string               `json:"signature"`
	ReusedFrom      string               `json:"reused_from,omitempty"`
	EvidenceCreated string               `json:"evidence_created,omitempty"`
	PriorKnowledge  int                  `json:"prior_knowledge"`
	Analyzer        string               `json:"analyzer"`
	Images          map[string]string    `json:"images"`
}
type Hub struct {
	Root   string
	DB     *sql.DB
	Config Config
	Loader RepoLoader
	mu     sync.Mutex
	active string
	cancel context.CancelFunc
	wg     sync.WaitGroup
	lock   *os.File
}

func DefaultKnowledgeDir() string {
	if path := os.Getenv("VEDA_HOME"); path != "" {
		return path
	}
	dir, err := os.UserConfigDir()
	if err != nil {
		dir = os.TempDir()
	}
	return filepath.Join(dir, "Veda")
}
func OpenHub(root string, c Config) (*Hub, error) {
	var err error
	root, err = filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	if strings.Contains(root, ",") {
		return nil, errors.New("knowledge path cannot contain commas")
	}
	if err = os.MkdirAll(filepath.Join(root, "investigations"), 0700); err != nil {
		return nil, err
	}
	lock, err := os.OpenFile(filepath.Join(root, "hub.lock"), os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, err
	}
	if err = syscall.Flock(int(lock.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		lock.Close()
		return nil, errors.New("this knowledge store is already open in another Veda server")
	}
	db, err := sql.Open("sqlite3", filepath.Join(root, "knowledge.db")+"?_busy_timeout=5000&_journal_mode=WAL")
	if err != nil {
		lock.Close()
		return nil, err
	}
	db.SetMaxOpenConns(1)
	_, err = db.Exec(`CREATE TABLE IF NOT EXISTS investigations(id TEXT PRIMARY KEY,created TEXT NOT NULL,repo TEXT NOT NULL,revision TEXT NOT NULL,signature TEXT NOT NULL,objective TEXT NOT NULL,status TEXT NOT NULL,data TEXT NOT NULL,checksum TEXT NOT NULL); CREATE INDEX IF NOT EXISTS knowledge_version ON investigations(repo,revision,signature,created);`)
	if err != nil {
		db.Close()
		lock.Close()
		return nil, err
	}
	h := &Hub{Root: root, DB: db, Config: c, Loader: GitLoader{}, lock: lock}
	runs, err := h.List()
	if err != nil {
		h.Close()
		return nil, err
	}
	for _, r := range runs {
		if r.Status == "running" {
			r.Status = "interrupted"
			r.Stage = "finished"
			r.Error = "Veda stopped before this investigation finished; completed evidence was retained."
			r.Finished = now()
			r.Report = InvestigationReport(r)
			if err = h.save(&r); err != nil {
				h.Close()
				return nil, err
			}
		}
	}
	return h, nil
}
func now() string { return time.Now().UTC().Format(time.RFC3339Nano) }
func (h *Hub) Close() error {
	h.Cancel()
	h.wg.Wait()
	err := h.DB.Close()
	if h.lock != nil {
		_ = syscall.Flock(int(h.lock.Fd()), syscall.LOCK_UN)
		err = errors.Join(err, h.lock.Close())
	}
	return err
}
func (h *Hub) Active() string { h.mu.Lock(); defer h.mu.Unlock(); return h.active }
func (h *Hub) Cancel() {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.cancel != nil {
		h.cancel()
	}
}
func (h *Hub) save(r *Investigation) error {
	data, err := json.Marshal(r)
	if err != nil {
		return err
	}
	_, err = h.DB.Exec(`INSERT INTO investigations VALUES(?,?,?,?,?,?,?,?,?) ON CONFLICT(id) DO UPDATE SET repo=excluded.repo,revision=excluded.revision,signature=excluded.signature,objective=excluded.objective,status=excluded.status,data=excluded.data,checksum=excluded.checksum`, r.ID, r.Created, r.Repository.Name, r.Repository.Commit, r.Signature, r.Request.Objective, r.Status, string(data), digest(string(data)))
	return err
}
func decodeInvestigation(data, checksum string) (Investigation, error) {
	var r Investigation
	if digest(data) != checksum {
		return r, errors.New("stored evidence checksum mismatch")
	}
	err := json.Unmarshal([]byte(data), &r)
	return r, err
}
func (h *Hub) Get(id string) (Investigation, error) {
	var data, checksum string
	err := h.DB.QueryRow("SELECT data,checksum FROM investigations WHERE id=?", id).Scan(&data, &checksum)
	if err != nil {
		return Investigation{}, err
	}
	return decodeInvestigation(data, checksum)
}
func (h *Hub) List() ([]Investigation, error) {
	rows, err := h.DB.Query("SELECT data,checksum FROM investigations ORDER BY created DESC LIMIT 100")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []Investigation{}
	for rows.Next() {
		var d, c string
		if err = rows.Scan(&d, &c); err != nil {
			return nil, err
		}
		r, e := decodeInvestigation(d, c)
		if e != nil {
			return nil, e
		}
		result = append(result, r)
	}
	return result, rows.Err()
}
func (h *Hub) knowledge(repo, revision, signature string) ([]Investigation, error) {
	rows, err := h.DB.Query("SELECT data,checksum FROM investigations WHERE repo=? AND revision=? AND signature=? AND status='completed' ORDER BY created DESC LIMIT 12", repo, revision, signature)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Investigation{}
	for rows.Next() {
		var d, c string
		if err = rows.Scan(&d, &c); err != nil {
			return nil, err
		}
		r, e := decodeInvestigation(d, c)
		if e != nil {
			return nil, e
		}
		out = append(out, r)
	}
	return out, rows.Err()
}
func (h *Hub) Start(in InvestigationRequest) (string, error) {
	url, _, err := CanonicalRepository(in.URL)
	if err != nil {
		return "", err
	}
	in.URL = url
	if err = ValidateRef(in.Ref); err != nil {
		return "", err
	}
	in.Objective = strings.TrimSpace(in.Objective)
	if len(in.Objective) < 8 || len(in.Objective) > 2000 {
		return "", errors.New("describe the investigation in 8–2,000 characters")
	}
	if in.Module != "" && (!filepath.IsLocal(in.Module) || strings.Contains(in.Module, "\\") || strings.HasPrefix(in.Module, ".")) {
		return "", errors.New("module must be a repository-relative directory")
	}
	if in.Optimize {
		in.Runtime = true
		in.Model = true
		c := h.Config
		if in.Benchmark != "" {
			c.Benchmark = in.Benchmark
		}
		if err = c.Validate(); err != nil {
			return "", err
		}
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.active != "" {
		return "", errors.New("another repository investigation is running")
	}
	r := Investigation{ID: NewID(), Request: in, Status: "running", Stage: "queued", Created: now(), Analyzer: AnalyzerVersion, Experiments: []AuditExperiment{}, Findings: []Finding{}, Events: []Event{}, Limits: []string{}, Images: map[string]string{}}
	if err = h.save(&r); err != nil {
		return "", err
	}
	ctx, cancel := context.WithCancel(context.Background())
	h.active = r.ID
	h.cancel = cancel
	h.wg.Add(1)
	go func() {
		defer h.wg.Done()
		defer cancel()
		defer func() { h.mu.Lock(); h.active = ""; h.cancel = nil; h.mu.Unlock() }()
		h.investigate(ctx, &r)
	}()
	return r.ID, nil
}
func (h *Hub) event(r *Investigation, stage, msg string) error {
	r.Stage = stage
	r.Events = append(r.Events, Event{ID: int64(len(r.Events) + 1), RunID: r.ID, At: now(), Message: msg})
	return h.save(r)
}
func (h *Hub) step(r *Investigation, title, kind, input string, fn func(*AuditExperiment)) error {
	x := AuditExperiment{ID: fmt.Sprintf("EXP-%03d", len(r.Experiments)+1), Title: title, Kind: kind, Status: "running", Started: now(), Input: input, Evidence: []string{}}
	r.Experiments = append(r.Experiments, x)
	if err := h.save(r); err != nil {
		return err
	}
	start := time.Now()
	fn(&x)
	x.Duration = time.Since(start).Seconds()
	if x.Status == "running" {
		x.Status = "passed"
	}
	r.Experiments[len(r.Experiments)-1] = x
	return h.save(r)
}
func (h *Hub) investigate(ctx context.Context, r *Investigation) {
	dir := filepath.Join(h.Root, "investigations", r.ID)
	var failure error
	defer func() {
		if x := recover(); x != nil {
			failure = fmt.Errorf("investigation stopped: %v", x)
		}
		if failure != nil {
			r.Status = "failed"
			r.Error = failure.Error()
		}
		if ctx.Err() != nil {
			r.Status = "cancelled"
			r.Error = ctx.Err().Error()
		}
		if r.Status == "running" {
			r.Status = "completed"
		}
		r.Stage = "finished"
		r.Finished = now()
		r.Report = InvestigationReport(*r)
		if e := os.MkdirAll(dir, 0700); e == nil {
			_ = os.WriteFile(filepath.Join(dir, "report.md"), []byte(r.Report), 0600)
			_ = WriteJSON(filepath.Join(dir, "evidence.json"), r)
		}
		if e := h.save(r); e != nil {
			fmt.Fprintln(os.Stderr, "Veda knowledge persistence:", e)
		}
	}()
	if failure = h.event(r, "cloning", "Cloning GitHub repository and resolving its exact revision."); failure != nil {
		return
	}
	failure = h.step(r, "Clone and capture revision", "source", "GitHub source · regular, bounded source files only", func(x *AuditExperiment) {
		var err error
		r.Repository, err = h.Loader.Load(ctx, r.Request.URL, r.Request.Ref, dir)
		if err != nil {
			x.Status = "failed"
			x.Stderr = err.Error()
			failure = err
			return
		}
		x.Conclusion = fmt.Sprintf("Captured %d source files at %s", len(r.Repository.Files), r.Repository.Commit)
		x.Evidence = []string{r.Repository.URL + "/tree/" + r.Repository.Commit}
	})
	// The loader's error is also recorded on the step; do not overwrite it with save success.
	if failure != nil {
		return
	}
	if len(r.Experiments) > 0 && r.Experiments[0].Status == "failed" {
		failure = errors.New(r.Experiments[0].Stderr)
		return
	}
	if ctx.Err() != nil {
		return
	}
	r.Limits = append(r.Limits, r.Repository.Warnings...)
	if r.Request.Runtime {
		if failure = h.event(r, "environment", "Resolving isolated check runtimes."); failure != nil {
			return
		}
		h.prepareRuntimes(ctx, r)
	}
	settings, _ := json.Marshal(map[string]any{"analyzer": AnalyzerVersion, "config": h.Config, "runtime": r.Request.Runtime, "model_review": r.Request.Model, "images": r.Images, "optimize": r.Request.Optimize, "benchmark": r.Request.Benchmark, "module": r.Request.Module})
	r.Signature = digest(string(settings))
	previous, err := h.knowledge(r.Repository.Name, r.Repository.Commit, r.Signature)
	if err != nil {
		r.Limits = append(r.Limits, "Previous knowledge could not be verified: "+err.Error())
	}
	r.PriorKnowledge = len(previous)
	if !r.Request.Force {
		for _, prior := range previous {
			if prior.Request.Objective == r.Request.Objective && reusableInvestigation(prior) {
				r.Architecture = prior.Architecture
				r.Findings = prior.Findings
				r.Summary = prior.Summary
				r.ReusedFrom = prior.ID
				r.EvidenceCreated = prior.Created
				if prior.EvidenceCreated != "" {
					r.EvidenceCreated = prior.EvidenceCreated
				}
				r.Experiments = prior.Experiments
				r.Limits = prior.Limits
				_ = h.event(r, "reusing", "Reusing verified knowledge for this exact commit, objective and analysis settings. Original evidence times are retained.")
				return
			}
		}
	}
	if failure = h.event(r, "architecture", "Extracting components and relationships from source declarations."); failure != nil {
		return
	}
	failure = h.step(r, "Map declared architecture", "static", "Terraform HCL, Dockerfiles, Compose, Kubernetes and package manifests", func(x *AuditExperiment) {
		r.Architecture = Architecture(r.Repository)
		x.Conclusion = fmt.Sprintf("%d components and %d relationships", len(r.Architecture.Nodes), len(r.Architecture.Edges))
		r.Limits = append(r.Limits, r.Architecture.Warnings...)
		for _, node := range r.Architecture.Nodes {
			if node.Path != "" {
				x.Evidence = append(x.Evidence, fmt.Sprintf("%s:%d", node.Path, node.Line))
			}
		}
	})
	if failure != nil {
		return
	}
	if failure = h.event(r, "checks", "Running source checks and recording their evidence."); failure != nil {
		return
	}
	h.staticChecks(r)
	if r.Request.Runtime {
		h.runtimeChecks(ctx, r)
	}
	if ctx.Err() != nil {
		return
	}
	if r.Request.Model {
		if failure = h.event(r, "reasoning", "Reviewing source and experimental evidence with the local model."); failure != nil {
			return
		}
		failure = h.step(r, "Local model evidence review", "model", "Objective + bounded source excerpts + observed results + knowledge at the same revision", func(x *AuditExperiment) {
			prompt := reviewPrompt(*r, previous)
			x.Input = prompt
			review, raw, err := (LocalModel{h.Config.Endpoint, h.Config.Model}).Review(ctx, prompt)
			x.Stdout = raw
			if err != nil {
				x.Status = "blocked"
				x.Stderr = err.Error()
				x.Conclusion = "Report retains observed evidence; local model review unavailable."
				r.Limits = append(r.Limits, x.Conclusion)
				return
			}
			r.Summary = review.Summary
			x.Conclusion = review.Summary
			for _, finding := range review.Findings {
				if text, ok := r.Repository.Content[finding.Path]; ok && finding.Line > 0 && finding.Line <= len(strings.Split(text, "\n")) {
					finding.Basis = "local model interpretation · requires review"
					r.Findings = append(r.Findings, finding)
				}
			}
		})
		if failure != nil {
			return
		}
	}
	if r.Request.Optimize && ctx.Err() == nil {
		h.optimize(ctx, r, dir)
	}
	_ = h.event(r, "reporting", "Saving the report and revision-matched knowledge to the shared library.")
}
func InvestigationReport(r Investigation) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# Veda investigation\n\n**Objective:** %s\n\n**Repository:** %s\n\n**Revision:** `%s`\n\n**Status:** %s · %s\n\n", r.Request.Objective, r.Repository.URL, r.Repository.Commit, r.Status, r.Created)
	if r.ReusedFrom != "" {
		fmt.Fprintf(&b, "Reused evidence from `%s`, recorded %s. This is not a fresh runtime measurement.\n\n", r.ReusedFrom, r.EvidenceCreated)
	}
	if r.Error != "" {
		fmt.Fprintf(&b, "Error: %s\n\n", r.Error)
	}
	if r.Summary != "" {
		fmt.Fprintf(&b, "## Local model interpretation\n\n%s\n\n", r.Summary)
	}
	fmt.Fprintf(&b, "## Observed structure\n\n%d captured files, %d declared components, %d relationships. Infrastructure declarations are not live deployment state.\n\n", len(r.Repository.Files), len(r.Architecture.Nodes), len(r.Architecture.Edges))
	b.WriteString("## Experiments and evidence\n\n")
	for _, x := range r.Experiments {
		fmt.Fprintf(&b, "### %s · %s\n\n%s / %s · %.2fs\n\n%s\n\n", x.ID, x.Title, x.Kind, x.Status, x.Duration, x.Conclusion)
		if x.Stderr != "" {
			fmt.Fprintf(&b, "Details: %s\n\n", x.Stderr)
		}
	}
	b.WriteString("## Findings to review\n\n")
	if len(r.Findings) == 0 {
		b.WriteString("No source-cited findings were produced. This does not prove the repository is defect-free.\n\n")
	}
	for _, f := range r.Findings {
		fmt.Fprintf(&b, "- **%s** %s — `%s:%d` (%s). %s\n", f.Severity, f.Title, f.Path, f.Line, f.Basis, f.Evidence)
	}
	b.WriteString("\n## Coverage and limits\n\n")
	for _, limit := range r.Limits {
		fmt.Fprintf(&b, "- %s\n", limit)
	}
	b.WriteString("\nRuntime checks execute in isolated containers; no infrastructure was deployed. Static checks, model interpretations and runtime tests are separately labeled.\n")
	return b.String()
}

func reusableInvestigation(r Investigation) bool {
	for _, x := range r.Experiments {
		if x.Status == "blocked" || x.Status == "running" {
			return false
		}
	}
	return r.Status == "completed"
}
