package veda

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestBenchmarkParsingAndAmbiguity(t *testing.T) {
	out := ""
	for _, n := range []int{100, 40, 60} {
		out += fmt.Sprintf("BenchmarkRender-2 1000 %d ns/op 120 B/op 2 allocs/op\n", n)
	}
	m, e := ParseBench(out)
	if e != nil || m.NS != 60 || m.Bytes != 120 || len(m.Samples) != 3 {
		t.Fatalf("bad measurement: %+v %v", m, e)
	}
	if _, e = ParseBench(out + "BenchmarkOther-2 100 12 ns/op 20 B/op 1 allocs/op\n"); e == nil {
		t.Fatal("ambiguous benchmark accepted")
	}
	if _, e = ParseBench("BenchmarkRender-2 1000 40 ns/op 120 B/op 2 allocs/op\n"); e == nil {
		t.Fatal("incomplete samples accepted")
	}
}
func TestEditsAndEvidence(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main\n"), 0644)
	os.WriteFile(filepath.Join(dir, "main_test.go"), []byte("package main\n"), 0644)
	for _, path := range []string{"../escape.go", "/tmp/escape.go", "main_test.go", "go.mod", "x/../main.go", ".git/config.go", "missing.go"} {
		if e := ApplyEdits(dir, []Edit{{path, "package main"}}); e == nil {
			t.Errorf("accepted unsafe path %s", path)
		}
	}
	if e := ApplyEdits(dir, []Edit{{"main.go", "package main\nfunc main(){}"}}); e != nil {
		t.Fatal(e)
	}
	if e := Seal(dir); e != nil {
		t.Fatal(e)
	}
	if e := Verify(dir); e != nil {
		t.Fatal(e)
	}
	os.WriteFile(filepath.Join(dir, "main.go"), []byte("modified"), 0644)
	if Verify(dir) == nil {
		t.Fatal("tampering not detected")
	}
}
func TestSnapshotRefusesSymlinks(t *testing.T) {
	root := t.TempDir()
	os.Symlink("/etc/passwd", filepath.Join(root, "leak.go"))
	if Snapshot(root, filepath.Join(t.TempDir(), "snapshot")) == nil {
		t.Fatal("symlink allowed")
	}
}
func TestEndpointPolicy(t *testing.T) {
	for _, v := range []string{"https://api.openai.com/v1", "http://localhost:8080/v1", "http://127.0.0.1.evil.test/v1", "http://user:pass@127.0.0.1:18080", "http://192.168.1.1/v1"} {
		if ValidateEndpoint(v) == nil {
			t.Errorf("unsafe endpoint accepted: %s", v)
		}
	}
	if e := ValidateEndpoint("http://127.0.0.1:18080/v1"); e != nil {
		t.Fatal(e)
	}
}

type transportFunc func(*http.Request) (*http.Response, error)

func (f transportFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func TestModelStructuredResponse(t *testing.T) {
	old := localClient
	defer func() { localClient = old }()
	localClient = &http.Client{Transport: transportFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.Path != "/v1/chat/completions" {
			t.Error(r.URL.Path)
		}
		var body map[string]any
		json.NewDecoder(r.Body).Decode(&body)
		if body["response_format"] == nil {
			t.Error("missing schema")
		}
		content := `{"hypotheses":[{"title":"one","rationale":"a"},{"title":"two","rationale":"b"},{"title":"three","rationale":"c"}]}`
		raw, _ := json.Marshal(map[string]any{"choices": []any{map[string]any{"finish_reason": "stop", "message": map[string]string{"content": content}}}})
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(string(raw))), Header: make(http.Header)}, nil
	})}
	h, _, e := (LocalModel{"http://127.0.0.1:18080/v1", "test"}).Plan(context.Background(), "optimize")
	if e != nil || len(h) != 3 {
		t.Fatalf("%v %+v", e, h)
	}
}

type fakeRunner struct{ n int }

func (r *fakeRunner) Measure(_ context.Context, _, _ string, _ Config) (Measurement, error) {
	r.n++
	m := Measurement{Benchmark: "BenchmarkRender", Bytes: 100, NS: 100, Allocs: 10, TestsPassed: true, Samples: []Sample{{100, 100, 10}, {100, 100, 10}, {100, 100, 10}}}
	switch r.n {
	case 2:
		return m, fmt.Errorf("behavior test failed")
	case 3:
		m.Bytes = 80
	case 4:
		m.Bytes = 50
	}
	return m, nil
}
func TestResearchLifecycleAndOriginalPreserved(t *testing.T) {
	root := t.TempDir()
	src := "package allocdemo\nfunc Render(values []string) string { return \"baseline\" }\n"
	os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.local/allocdemo\ngo 1.26\n"), 0644)
	os.WriteFile(filepath.Join(root, "render.go"), []byte(src), 0644)
	c, e := Init(root)
	if e != nil {
		t.Fatal(e)
	}
	s, e := OpenStore(root)
	if e != nil {
		t.Fatal(e)
	}
	defer s.DB.Close()
	engine := Engine{Root: root, Store: s, Model: &DemoModel{}, Runner: &fakeRunner{}}
	r, e := engine.Run(context.Background(), Options{Objective: "Reduce memory by 20 percent", Config: c, Demo: true})
	if e != nil {
		t.Fatal(e)
	}
	if r.Status != "completed" || r.Best != "EXP-000003" || r.Improvement != 50 || len(r.Experiments) != 3 || r.Experiments[0].Status != "REJECTED" {
		t.Fatalf("bad result %+v", r)
	}
	raw, _ := os.ReadFile(filepath.Join(root, "render.go"))
	if string(raw) != src {
		t.Fatal("original changed")
	}
	if e = VerifyRun(root, r.ID); e != nil {
		t.Fatal(e)
	}
	if !strings.Contains(s.History(), "behavior test failed") {
		t.Fatal("failed experiment not retained in memory")
	}
	if !strings.Contains(r.Report, "scripted-demo") {
		t.Fatal("demo not labeled")
	}
}
func TestServerRejectsCrossOriginAndUnsafeHost(t *testing.T) {
	root := t.TempDir()
	s, e := OpenStore(root)
	if e != nil {
		t.Fatal(e)
	}
	defer s.DB.Close()
	srv := NewServer(root, s, Config{})
	h := srv.Handler("127.0.0.1:8787")
	for _, tc := range []struct{ host, origin, token string }{{"evil.test", "", ""}, {"127.0.0.1:8787", "https://evil.test", srv.token}, {"127.0.0.1:8787", "", "bad"}} {
		r := httptest.NewRequest("POST", "http://"+tc.host+"/api/cancel", strings.NewReader("{}"))
		r.Header.Set("Origin", tc.origin)
		r.Header.Set("X-Veda-Token", tc.token)
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != 403 {
			t.Fatalf("unsafe request got %d", w.Code)
		}
	}
}

func TestExportedPatchApplies(t *testing.T) {
	base := filepath.Join(t.TempDir(), "base")
	candidate := filepath.Join(t.TempDir(), "candidate")
	os.MkdirAll(base, 0755)
	os.MkdirAll(candidate, 0755)
	os.WriteFile(filepath.Join(base, "value.go"), []byte("package value\nconst V = 1\n"), 0644)
	os.WriteFile(filepath.Join(candidate, "value.go"), []byte("package value\nconst V = 2\n"), 0644)
	patch, e := Diff(context.Background(), base, candidate)
	if e != nil {
		t.Fatal(e)
	}
	if !strings.Contains(patch, "--- a/value.go") || !strings.Contains(patch, "+++ b/value.go") {
		t.Fatalf("nonportable patch: %s", patch)
	}
	path := filepath.Join(t.TempDir(), "candidate.patch")
	os.WriteFile(path, []byte(patch), 0600)
	cmd := exec.Command("git", "apply", "--check", path)
	cmd.Dir = base
	if out, e := cmd.CombinedOutput(); e != nil {
		t.Fatalf("patch does not apply: %v %s", e, out)
	}
}
