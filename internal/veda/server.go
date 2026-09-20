package veda

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"
	"veda.local/veda/web"
)

type Server struct {
	Hub       *Hub
	Root      string
	Store     *Store
	Config    Config
	mu        sync.Mutex
	wg        sync.WaitGroup
	cancel    context.CancelFunc
	active    string
	token     string
	lastError string
}

func NewServer(root string, s *Store, c Config) *Server {
	b := make([]byte, 32)
	if _, e := rand.Read(b); e != nil {
		panic(e)
	}
	return &Server{Root: root, Store: s, Config: c, token: hex.EncodeToString(b)}
}
func (s *Server) Handler(host string) http.Handler {
	mux := http.NewServeMux()
	s.repositoryRoutes(mux)
	mux.HandleFunc("GET /api/state", func(w http.ResponseWriter, r *http.Request) {
		runs, e := s.Store.Runs()
		if e != nil {
			apiError(w, e, 500)
			return
		}
		s.mu.Lock()
		active, lastErr := s.active, s.lastError
		s.mu.Unlock()
		jsonOut(w, map[string]any{"runs": runs, "workspace": s.Root, "config": s.Config, "hardware": DetectHardware(), "token": s.token, "active": active, "last_error": lastErr})
	})
	mux.HandleFunc("GET /api/health", func(w http.ResponseWriter, r *http.Request) { jsonOut(w, Doctor(r.Context(), s.Config)) })
	mux.HandleFunc("GET /api/runs/{id}", func(w http.ResponseWriter, r *http.Request) {
		run, e := s.Store.Get(r.PathValue("id"))
		if e != nil {
			apiError(w, e, 404)
			return
		}
		events, e := s.Store.Events(run.ID)
		if e != nil {
			apiError(w, e, 500)
			return
		}
		jsonOut(w, map[string]any{"run": run, "events": events})
	})
	mux.HandleFunc("GET /api/runs/{id}/report", func(w http.ResponseWriter, r *http.Request) {
		run, e := s.Store.Get(r.PathValue("id"))
		if e != nil {
			apiError(w, e, 404)
			return
		}
		w.Header().Set("Content-Type", "text/markdown; charset=utf-8")
		w.Header().Set("Content-Disposition", `attachment; filename="veda-report.md"`)
		io.WriteString(w, run.Report)
	})
	mux.HandleFunc("GET /api/runs/{id}/patch", func(w http.ResponseWriter, r *http.Request) {
		run, e := s.Store.Get(r.PathValue("id"))
		if e != nil || run.Best == "" {
			apiError(w, errors.New("no accepted patch for this run"), 404)
			return
		}
		b, e := os.ReadFile(filepath.Join(s.Root, ".veda", "runs", run.ID, "best.patch"))
		if e != nil {
			apiError(w, e, 404)
			return
		}
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.Header().Set("Content-Disposition", `attachment; filename="veda-best.patch"`)
		w.Write(b)
	})
	mux.HandleFunc("GET /api/runs/{id}/experiments/{exp}", func(w http.ResponseWriter, r *http.Request) {
		run, e := s.Store.Get(r.PathValue("id"))
		if e != nil {
			apiError(w, e, 404)
			return
		}
		var selected *Experiment
		for _, x := range run.Experiments {
			if x.ID == r.PathValue("exp") {
				v := x
				selected = &v
			}
		}
		if selected == nil {
			apiError(w, errors.New("experiment not found"), 404)
			return
		}
		dir := filepath.Join(s.Root, ".veda", "runs", run.ID, "experiments", selected.ID)
		read := func(name string) string {
			b, _ := os.ReadFile(filepath.Join(dir, name))
			if len(b) > 64000 {
				b = b[:64000]
			}
			return string(b)
		}
		verify := Verify(dir)
		verified := verify == nil
		jsonOut(w, map[string]any{"experiment": selected, "patch": read("diff.patch"), "stdout": read("stdout.log"), "stderr": read("stderr.log"), "verified": verified})
	})
	mux.HandleFunc("POST /api/runs", func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			Objective string  `json:"objective"`
			Benchmark string  `json:"benchmark"`
			Metric    string  `json:"metric"`
			Target    float64 `json:"target"`
			Max       int     `json:"max_experiments"`
			Demo      bool    `json:"demo"`
		}
		if e := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8192)).Decode(&in); e != nil {
			apiError(w, e, 400)
			return
		}
		c := s.Config
		c.Benchmark = in.Benchmark
		c.Metric = in.Metric
		c.Target = in.Target
		c.MaxExperiments = in.Max
		if e := c.Validate(); e != nil {
			apiError(w, e, 400)
			return
		}
		if len(strings.TrimSpace(in.Objective)) < 8 || len(in.Objective) > 2000 {
			apiError(w, errors.New("objective must be between 8 and 2000 characters"), 400)
			return
		}
		s.mu.Lock()
		if s.active != "" || s.Hub != nil && s.Hub.Active() != "" {
			s.mu.Unlock()
			apiError(w, errors.New("an investigation is already running"), 409)
			return
		}
		ctx, cancel := context.WithCancel(context.Background())
		id := NewID()
		s.active = id
		s.cancel = cancel
		s.lastError = ""
		s.wg.Add(1)
		s.mu.Unlock()
		go func() {
			defer s.wg.Done()
			defer cancel()
			var m Model = LocalModel{c.Endpoint, c.Model}
			if in.Demo {
				m = &DemoModel{}
			}
			engine := Engine{Root: s.Root, Store: s.Store, Model: m, Runner: DockerRunner{}}
			_, err := engine.Run(ctx, Options{id, in.Objective, c, in.Demo})
			s.mu.Lock()
			defer s.mu.Unlock()
			s.active = ""
			s.cancel = nil
			if err != nil {
				s.lastError = err.Error()
			}
		}()
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(202)
		jsonOut(w, map[string]string{"id": id})
	})
	mux.HandleFunc("POST /api/cancel", func(w http.ResponseWriter, r *http.Request) {
		s.mu.Lock()
		defer s.mu.Unlock()
		if s.cancel != nil {
			s.cancel()
		}
		jsonOut(w, map[string]bool{"ok": true})
	})
	mux.Handle("/", http.FileServer(http.FS(web.Files)))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data:; connect-src 'self'; frame-ancestors 'none'; base-uri 'none'; form-action 'self'")
		if r.Host != host {
			apiError(w, errors.New("invalid local host"), 403)
			return
		}
		if origin := r.Header.Get("Origin"); origin != "" && origin != "http://"+host {
			apiError(w, errors.New("cross-origin request refused"), 403)
			return
		}
		if r.Method != "GET" && r.Method != "HEAD" {
			if subtle.ConstantTimeCompare([]byte(r.Header.Get("X-Veda-Token")), []byte(s.token)) != 1 {
				apiError(w, errors.New("missing session token; reload dashboard"), 403)
				return
			}
		}
		mux.ServeHTTP(w, r)
	})
}
func jsonOut(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}
func apiError(w http.ResponseWriter, e error, status int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]string{"error": e.Error()})
}
func (s *Server) Serve(ctx context.Context, address string) error {
	host, _, e := net.SplitHostPort(address)
	if e != nil || net.ParseIP(host) == nil || !net.ParseIP(host).IsLoopback() {
		return errors.New("dashboard must bind to a literal loopback IP")
	}
	srv := &http.Server{Addr: address, Handler: s.Handler(address), ReadHeaderTimeout: 5 * time.Second, IdleTimeout: 60 * time.Second}
	shutdownDone := make(chan struct{})
	go func() {
		defer close(shutdownDone)
		<-ctx.Done()
		s.mu.Lock()
		if s.cancel != nil {
			s.cancel()
		}
		s.mu.Unlock()
		stop, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		srv.Shutdown(stop)
		s.mu.Lock()
		if s.cancel != nil {
			s.cancel()
		}
		s.mu.Unlock()
	}()
	fmt.Printf("Veda dashboard: http://%s\nWorkspace: %s\n", address, s.Root)
	err := srv.ListenAndServe()
	if errors.Is(err, http.ErrServerClosed) {
		<-shutdownDone
		s.wg.Wait()
		return nil
	}
	return err
}
func Doctor(ctx context.Context, c Config) map[string]any {
	out := map[string]any{"hardware": DetectHardware(), "endpoint": c.Endpoint, "model": c.Model, "docker": false, "model_ready": false}
	check, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	image, e := DockerReady(check, c.Image)
	if e == nil {
		out["docker"] = true
		out["image"] = image
	} else {
		out["docker_error"] = e.Error()
	}
	check2, cancel2 := context.WithTimeout(ctx, 3*time.Second)
	defer cancel2()
	if e = ValidateEndpoint(c.Endpoint); e != nil {
		out["model_error"] = e.Error()
		return out
	}
	req, e := http.NewRequestWithContext(check2, "GET", strings.TrimRight(c.Endpoint, "/")+"/models", nil)
	if e != nil {
		out["model_error"] = e.Error()
		return out
	}
	res, e := localClient.Do(req)
	if e == nil {
		defer res.Body.Close()
		var models struct {
			Data []struct {
				ID string `json:"id"`
			} `json:"data"`
		}
		if res.StatusCode == 200 && json.NewDecoder(res.Body).Decode(&models) == nil {
			for _, m := range models.Data {
				if m.ID == c.Model {
					out["model_ready"] = true
				}
			}
		}
		if out["model_ready"] == false {
			out["model_error"] = "endpoint reachable but configured model is not loaded"
		}
	} else {
		out["model_error"] = e.Error()
	}
	return out
}

var validRunID = regexp.MustCompile(`^RUN-[0-9]{8}-[0-9]{6}\.[0-9]{9}$`)

func VerifyRun(root, id string) error {
	if !validRunID.MatchString(id) {
		return errors.New("invalid run ID")
	}
	dir := filepath.Join(root, ".veda", "runs", id)
	if e := Verify(filepath.Join(dir, "baseline")); e != nil {
		return e
	}
	entries, e := os.ReadDir(filepath.Join(dir, "experiments"))
	if e != nil {
		return e
	}
	for _, entry := range entries {
		if entry.IsDir() {
			if e = Verify(filepath.Join(dir, "experiments", entry.Name())); e != nil {
				return fmt.Errorf("%s: %w", entry.Name(), e)
			}
		}
	}
	return nil
}
func Reproduce(ctx context.Context, root string, s *Store, id, exp string) (Measurement, string, error) {
	var empty Measurement
	if !validRunID.MatchString(id) {
		return empty, "", errors.New("invalid run ID")
	}
	r, e := s.Get(id)
	if e != nil {
		return empty, "", e
	}
	source := ""
	if exp == "BASELINE" {
		source = filepath.Join(root, ".veda", "runs", id, "baseline", "input")
	} else {
		for _, x := range r.Experiments {
			if x.ID == exp {
				source = filepath.Join(root, ".veda", "runs", id, "experiments", exp, "output")
			}
		}
	}
	if source == "" {
		return empty, "", errors.New("experiment not found")
	}
	if e = Verify(filepath.Dir(source)); e != nil {
		return empty, "", e
	}
	dir := filepath.Join(root, ".veda", "reproductions", NewID())
	if e = os.MkdirAll(dir, 0700); e != nil {
		return empty, dir, e
	}
	m, e := (DockerRunner{}).Measure(ctx, source, dir, r.Config)
	writeErr := WriteJSON(filepath.Join(dir, "metrics.json"), m)
	return m, dir, errors.Join(e, writeErr)
}

// Confirm the binary can locate Docker without invoking a shell.
func HasDocker() bool { _, e := exec.LookPath("docker"); return e == nil }
