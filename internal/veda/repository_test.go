package veda

import (
	"context"
	"encoding/json"
	"errors"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"
)

type fixtureLoader struct {
	revision string
	files    map[string]string
	wait     bool
	fail     bool
}

func (f fixtureLoader) Load(ctx context.Context, address, ref, dest string) (Repository, error) {
	if f.wait {
		<-ctx.Done()
		return Repository{}, ctx.Err()
	}
	if f.fail {
		return Repository{}, errors.New("repository not accessible")
	}
	address, name, _ := CanonicalRepository(address)
	r := Repository{URL: address, Name: name, Commit: f.revision, Ref: ref, Root: filepath.Join(dest, "source"), Content: f.files, Files: []RepoFile{}}
	paths := []string{}
	for p := range f.files {
		paths = append(paths, p)
	}
	sort.Strings(paths)
	for _, p := range paths {
		body := f.files[p]
		target := filepath.Join(r.Root, p)
		if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
			return r, err
		}
		if err := os.WriteFile(target, []byte(body), 0644); err != nil {
			return r, err
		}
		r.Files = append(r.Files, RepoFile{p, int64(len(body)), fileLanguage(p), digest(body)})
	}
	return r, nil
}
func testHub(t *testing.T) *Hub {
	t.Helper()
	h, e := OpenHub(t.TempDir(), Config{})
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { h.Close() })
	h.Loader = fixtureLoader{revision: "aaaaaaaa", files: map[string]string{"main.go": "package main\nfunc main() {}\n", "go.mod": "module example.test/app\ngo 1.26\n"}}
	return h
}
func finishInvestigation(t *testing.T, h *Hub, in InvestigationRequest) Investigation {
	t.Helper()
	id, e := h.Start(in)
	if e != nil {
		t.Fatal(e)
	}
	deadline := time.Now().Add(4 * time.Second)
	for time.Now().Before(deadline) {
		r, e := h.Get(id)
		if e != nil {
			t.Fatal(e)
		}
		if r.Status != "running" && h.Active() == "" {
			return r
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("investigation did not finish")
	return Investigation{}
}
func TestRepositoryInputBoundaries(t *testing.T) {
	for _, s := range []string{"https://github.com/Owner/Repo.git", "git@github.com:Owner/Repo.git", "https://github.com/Owner/Repo/"} {
		u, n, e := CanonicalRepository(s)
		if e != nil || u != "https://github.com/owner/repo" || n != "owner/repo" {
			t.Fatalf("%s: %s %s %v", s, u, n, e)
		}
	}
	for _, s := range []string{"file:///tmp/repo", "https://github.com.evil.test/o/r", "https://token@github.com/o/r", "https://github.com/o/r/tree/main", "https://github.com/o/..", "--upload-pack=evil", "https://github.com/o/r?token=abc"} {
		if _, _, e := CanonicalRepository(s); e == nil {
			t.Fatalf("accepted %q", s)
		}
	}
	for _, s := range []string{"--help", "../main", "x..y", "main.lock", "x\ny"} {
		if ValidateRef(s) == nil {
			t.Fatalf("accepted ref %q", s)
		}
	}
	for _, p := range []string{"../x.go", "/etc/x.go", ".env", "config.tfvars", "terraform.tfstate", "vendor/x.go", "link\\x.go", ".git/config", "aws_credentials.json"} {
		if allowedRepoFile(p) {
			t.Fatalf("captured %q", p)
		}
	}
	if !allowedRepoFile(".github/workflows/check.yml") || !allowedRepoFile("services/api/main.go") {
		t.Fatal("ordinary source excluded")
	}
}
func TestArchitectureEvidence(t *testing.T) {
	files := map[string]string{
		"infra/main.tf":      "resource \"aws_vpc\" \"main\" {\n cidr_block = \"10.0.0.0/16\"\n}\n",
		"infra/subnet.tf":    "resource \"aws_subnet\" \"web\" {\n vpc_id = aws_vpc.main.id\n cidr_block = var.range\n}\nvariable \"range\" {}\n",
		"docker-compose.yml": "services:\n  api:\n    build: ./api\n    depends_on: [db]\n    networks: [internal]\n  db:\n    image: postgres:16\n    volumes: [data:/var/lib/postgresql/data]\nvolumes:\n  data: {}\nnetworks:\n  internal: {}\n",
		"api/Dockerfile":     "FROM golang:1.26-alpine AS build\n",
		"api/go.mod":         "module example/api\ngo 1.26\n",
		"secret.yaml":        "apiVersion: v1\nkind: Secret\nmetadata:\n  name: database\ndata:\n  password: NEVER_IN_GRAPH\n",
	}
	r, e := (fixtureLoader{revision: "123", files: files}).Load(context.Background(), "https://github.com/a/b", "", t.TempDir())
	if e != nil {
		t.Fatal(e)
	}
	g := Architecture(r)
	want := map[string]bool{"tf:infra:resource.aws_subnet.web|tf:infra:resource.aws_vpc.main|references": false, "compose:docker-compose.yml:api|compose:docker-compose.yml:db|depends on": false, "compose:docker-compose.yml:api|dockerfile:api/Dockerfile|built from": false, "component:api|dockerfile:api/Dockerfile|build definition": false}
	for _, edge := range g.Edges {
		key := edge.Source + "|" + edge.Target + "|" + edge.Label
		if _, ok := want[key]; ok {
			want[key] = true
		}
		if edge.Path == "" || edge.Line < 1 {
			t.Fatalf("missing source evidence: %+v", edge)
		}
	}
	for k, v := range want {
		if !v {
			t.Errorf("missing relationship %s", k)
		}
	}
	data, _ := json.Marshal(g)
	if strings.Contains(string(data), "NEVER_IN_GRAPH") {
		t.Fatal("secret payload in graph")
	}
	r.Content["infra/main.tf"] = "resource {"
	if len(Architecture(r).Warnings) == 0 {
		t.Fatal("invalid HCL not disclosed")
	}
}
func TestKnowledgeReuseAndRevisionInvalidation(t *testing.T) {
	h := testHub(t)
	in := InvestigationRequest{URL: "https://github.com/a/b", Objective: "Explain this repository"}
	first := finishInvestigation(t, h, in)
	if first.Status != "completed" || len(first.Architecture.Nodes) < 2 {
		t.Fatalf("%+v", first)
	}
	second := finishInvestigation(t, h, in)
	if second.ReusedFrom != first.ID || second.EvidenceCreated != first.Created || second.Experiments[0].Started != first.Experiments[0].Started {
		t.Fatal("matching evidence lost provenance")
	}
	in.Force = true
	third := finishInvestigation(t, h, in)
	if third.ReusedFrom != "" || third.PriorKnowledge != 2 {
		t.Fatal("force refresh did not execute")
	}
	in.Force = false
	in.Objective = "Find risky code in this repository"
	fourth := finishInvestigation(t, h, in)
	if fourth.ReusedFrom != "" || fourth.PriorKnowledge != 3 {
		t.Fatal("new objective should use prior context and fresh checks")
	}
	f := h.Loader.(fixtureLoader)
	f.revision = "bbbbbbbb"
	h.Loader = f
	fifth := finishInvestigation(t, h, in)
	if fifth.ReusedFrom != "" || fifth.PriorKnowledge != 0 {
		t.Fatal("cross-revision reuse")
	}
	if _, e := h.DB.Exec("UPDATE investigations SET data='{}' WHERE id=?", first.ID); e != nil {
		t.Fatal(e)
	}
	if _, e := h.Get(first.ID); e == nil {
		t.Fatal("tampered evidence accepted")
	}
}
func TestInvestigationFailureCancellationAndLock(t *testing.T) {
	h := testHub(t)
	if second, e := OpenHub(h.Root, Config{}); e == nil {
		second.Close()
		t.Fatal("second server acquired store")
	}
	h.Loader = fixtureLoader{fail: true}
	r := finishInvestigation(t, h, InvestigationRequest{URL: "https://github.com/a/b", Objective: "Explain this repository"})
	if r.Status != "failed" || !strings.Contains(r.Report, "not accessible") {
		t.Fatal("clone failure shown as success")
	}
	h.Loader = fixtureLoader{wait: true}
	in := InvestigationRequest{URL: "https://github.com/a/b", Objective: "Explain this repository"}
	id, e := h.Start(in)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = h.Start(in); e == nil {
		t.Fatal("parallel run accepted")
	}
	h.Cancel()
	h.wg.Wait()
	r, e = h.Get(id)
	if e != nil || r.Status != "cancelled" {
		t.Fatalf("cancel: %s %v", r.Status, e)
	}
	if reusableInvestigation(Investigation{Status: "completed", Experiments: []AuditExperiment{{Status: "blocked"}}}) {
		t.Fatal("blocked check cached")
	}
}
func TestRepositoryAPIAndSourceIntegrity(t *testing.T) {
	h := testHub(t)
	r := finishInvestigation(t, h, InvestigationRequest{URL: "https://github.com/a/b", Objective: "Explain this repository"})
	s := NewServer(t.TempDir(), nil, Config{})
	s.Hub = h
	handler := s.Handler("127.0.0.1:8787")
	request := func(method, path, body, token string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, "http://127.0.0.1:8787"+path, strings.NewReader(body))
		req.Header.Set("X-Veda-Token", token)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)
		return w
	}
	base := "/api/investigations/" + r.ID
	for _, suffix := range []string{"", "/report", "/evidence", "/source?path=main.go"} {
		if w := request("GET", base+suffix, "", ""); w.Code != 200 {
			t.Fatalf("%s: %d %s", suffix, w.Code, w.Body.String())
		}
	}
	for _, p := range []string{"../knowledge.db", ".veda/project.json", "/etc/passwd"} {
		if w := request("GET", base+"/source?path="+url.QueryEscape(p), "", ""); w.Code != 404 {
			t.Fatalf("source escape %s: %d", p, w.Code)
		}
	}
	if w := request("POST", "/api/investigations", `{"url":"https://github.com/a/b","objective":"Explain repository"}`, ""); w.Code != 403 {
		t.Fatal("unauthenticated mutation accepted")
	}
	if w := request("POST", "/api/investigations", `{"url":"file:///tmp/repo","objective":"Explain repository"}`, s.token); w.Code != 400 {
		t.Fatal("non-GitHub clone accepted")
	}
	if err := os.WriteFile(filepath.Join(h.Root, "investigations", r.ID, "source", "main.go"), []byte("tampered"), 0644); err != nil {
		t.Fatal(err)
	}
	if w := request("GET", base+"/source?path=main.go", "", ""); w.Code != 409 {
		t.Fatalf("tampered source accepted: %d", w.Code)
	}
}

func TestKnowledgeRestartAndSettingsInvalidation(t *testing.T) {
	root := t.TempDir()
	h, err := OpenHub(root, Config{})
	if err != nil {
		t.Fatal(err)
	}
	h.Loader = fixtureLoader{revision: "abc", files: map[string]string{"main.go": "package main\n"}}
	in := InvestigationRequest{URL: "https://github.com/a/b", Objective: "Explain this repository"}
	first := finishInvestigation(t, h, in)
	interrupted := Investigation{ID: "RUN-interrupted", Created: now(), Status: "running", Stage: "reasoning"}
	if err = h.save(&interrupted); err != nil {
		t.Fatal(err)
	}
	if err = h.Close(); err != nil {
		t.Fatal(err)
	}
	h, err = OpenHub(root, Config{})
	if err != nil {
		t.Fatal(err)
	}
	defer h.Close()
	h.Loader = fixtureLoader{revision: "abc", files: map[string]string{"main.go": "package main\n"}}
	saved, err := h.Get(interrupted.ID)
	if err != nil || saved.Status != "interrupted" {
		t.Fatalf("lost interrupted state: %+v %v", saved, err)
	}
	second := finishInvestigation(t, h, in)
	if second.ReusedFrom != first.ID {
		t.Fatal("knowledge not reusable after restart")
	}
	h.Config.Model = "different-model"
	third := finishInvestigation(t, h, in)
	if third.ReusedFrom != "" || third.PriorKnowledge != 0 {
		t.Fatal("settings change reused old evidence")
	}
}
