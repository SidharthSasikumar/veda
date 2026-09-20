package veda

import (
	"bytes"
	"context"
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestSourceSuggestionBoundariesAndPatch(t *testing.T) {
	const original = "package example\nfunc Value() int { return 1 }\n"
	repo := Repository{Content: map[string]string{"source.go": original, "x_test.go": "package example", "go.mod": "module example", "web.css": "body { color: red; }\n"}}
	proposal := ChangeProposal{Title: "Update value", Edits: []SourceReplacement{{Path: "source.go", Search: "return 1", Replace: "return 2"}}}
	c, err := validateProposal(context.Background(), repo, proposal)
	if err != nil {
		t.Fatal(err)
	}
	if len(c.Files) != 1 || !strings.Contains(c.Patch, "+func Value() int { return 2 }") || repo.Content["source.go"] != original {
		t.Fatal("patch or original source changed unexpectedly")
	}
	dir := t.TempDir()
	if err = os.WriteFile(filepath.Join(dir, "source.go"), []byte(original), 0644); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("git", "apply", "--check", "-")
	cmd.Dir = dir
	cmd.Stdin = strings.NewReader(c.Patch)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("patch cannot be applied: %s %v", out, err)
	}
	for _, edit := range []SourceReplacement{{Path: "../source.go", Search: "1", Replace: "2"}, {Path: "go.mod", Search: "module", Replace: "other"}, {Path: "x_test.go", Search: "example", Replace: "other"}, {Path: "missing.go", Search: "1", Replace: "2"}, {Path: "source.go", Search: "not present", Replace: "2"}, {Path: "source.go", Search: "return 1", Replace: "return ("}, {Path: "source.go", Search: "return 1", Replace: "return 1"}} {
		if _, err := validateProposal(context.Background(), repo, ChangeProposal{Edits: []SourceReplacement{edit}}); err == nil {
			t.Fatalf("unsafe or invalid proposal accepted: %+v", edit)
		}
	}
	for _, p := range []string{"tests/helper.js", "src/a.spec.tsx", "__tests__/x.js", "test_main.py", ".github/script.js"} {
		if changeAllowed(p) {
			t.Fatalf("protected source accepted: %s", p)
		}
	}
	repo.Content["web.css"] = "red red"
	if _, err := validateProposal(context.Background(), repo, ChangeProposal{Edits: []SourceReplacement{{Path: "web.css", Search: "red", Replace: "blue"}}}); err == nil {
		t.Fatal("ambiguous replacement accepted")
	}
}

func TestSuggestionModelPipelineAndReuse(t *testing.T) {
	previous := localClient
	defer func() { localClient = previous }()
	drafts := 0
	localClient = &http.Client{Transport: transportFunc(func(r *http.Request) (*http.Response, error) {
		var body map[string]json.RawMessage
		_ = json.NewDecoder(r.Body).Decode(&body)
		content := `{"summary":"A small page.","findings":[]}`
		if bytes.Contains(body["response_format"], []byte(`"changes"`)) {
			drafts++
			content = `{"changes":[{"title":"Improve button contrast","rationale":"Makes the action easier to read.","edits":[{"path":"index.html","original":"color:#aaa","replacement":"color:#123"}]}]}`
			if drafts == 1 {
				content = strings.Replace(content, "color:#123", "color:#aaa", 1)
			}
		}
		encoded, _ := json.Marshal(map[string]any{"choices": []any{map[string]any{"message": map[string]string{"content": content}, "finish_reason": "stop"}}})
		return &http.Response{StatusCode: 200, Body: io.NopCloser(bytes.NewReader(encoded)), Header: make(http.Header)}, nil
	})}
	h := testHub(t)
	h.Config.Endpoint = "http://127.0.0.1:18080/v1"
	h.Loader = fixtureLoader{revision: "abcdef", files: map[string]string{"index.html": "<html><head></head><body><button style=\"color:#aaa\">Continue</button></body></html>"}}
	r := finishInvestigation(t, h, InvestigationRequest{URL: "https://github.com/a/b", Objective: "Improve the button contrast", Changes: true})
	if r.Status != "completed" || len(r.Changes) != 2 || r.Changes[0].Status != "rejected" || r.Changes[1].Status != "proposed" || r.Changes[1].Preview.Status != "not_requested" || drafts != 2 {
		t.Fatalf("unexpected change state: %+v", r.Changes)
	}
	if !r.Request.Model {
		t.Fatal("suggestions require local model review")
	}
	again := finishInvestigation(t, h, r.Request)
	if again.ReusedFrom != r.ID || len(again.Changes) != 2 || again.Changes[1].Patch != r.Changes[1].Patch || drafts != 2 {
		t.Fatal("change evidence not preserved during reuse")
	}
}

func testPNG(t *testing.T, c color.Color) []byte {
	t.Helper()
	img := image.NewNRGBA(image.Rect(0, 0, 2, 2))
	for y := 0; y < 2; y++ {
		for x := 0; x < 2; x++ {
			img.Set(x, y, c)
		}
	}
	var b bytes.Buffer
	if err := png.Encode(&b, img); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}
func TestScreenshotDifferencesAndIntegrity(t *testing.T) {
	before := testPNG(t, color.Black)
	after := testPNG(t, color.White)
	if _, percent, err := imageDifference(before, before); err != nil || percent != 0 {
		t.Fatal("identical screenshots differ")
	}
	if _, percent, err := imageDifference(before, after); err != nil || percent != 100 {
		t.Fatal("changed pixels not measured")
	}
	h := testHub(t)
	r := finishInvestigation(t, h, InvestigationRequest{URL: "https://github.com/a/b", Objective: "Review this captured source"})
	r.Changes = []SuggestedChange{{ID: "CHG-001", Patch: "test patch", Preview: &ChangePreview{Status: "captured", RunID: r.ID, Hashes: map[string]string{"before": digest(string(before))}}}}
	if err := h.save(&r); err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(h.Root, "investigations", r.ID, "changes", "CHG-001")
	_ = os.MkdirAll(dir, 0700)
	target := filepath.Join(dir, "before.png")
	_ = os.WriteFile(target, before, 0600)
	s := NewServer(t.TempDir(), nil, Config{})
	s.Hub = h
	handler := s.Handler("127.0.0.1:8787")
	get := func(suffix string) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, httptest.NewRequest("GET", "http://127.0.0.1:8787/api/investigations/"+r.ID+"/changes/CHG-001/"+suffix, nil))
		return w
	}
	if w := get("preview/before"); w.Code != 200 || w.Header().Get("Content-Type") != "image/png" || !bytes.Equal(w.Body.Bytes(), before) {
		t.Fatal("recorded preview inaccessible")
	}
	if w := get("patch"); w.Code != 200 || w.Body.String() != "test patch" {
		t.Fatal("patch export unavailable")
	}
	if get("preview/unknown").Code != 404 {
		t.Fatal("unknown screenshot accepted")
	}
	_ = os.WriteFile(target, after, 0600)
	if get("preview/before").Code != 409 {
		t.Fatal("modified screenshot served as recorded evidence")
	}
	_ = os.WriteFile(target, before, 0600)
	origin := r
	r.ID = NewID()
	r.ReusedFrom = origin.ID
	if err := h.save(&r); err != nil {
		t.Fatal(err)
	}
	if get("preview/before").Code != 200 {
		t.Fatal("reused preview lost original provenance")
	}
	r.Repository.Commit = "changed"
	_ = h.save(&r)
	if get("preview/before").Code != 409 {
		t.Fatal("cross-revision screenshot accepted")
	}
}

func TestPreviewPageSelection(t *testing.T) {
	repo := Repository{Content: map[string]string{"site/index.html": "html", "other/index.html": "html"}}
	c := SuggestedChange{Files: []ChangeFile{{Path: "site/styles/main.css"}}}
	if p, err := previewPage(repo, c, ""); err != nil || p != "site/index.html" {
		t.Fatal("nearest entry was not selected")
	}
	if _, err := previewPage(repo, c, "missing.html"); err == nil {
		t.Fatal("missing page accepted")
	}
	if _, err := previewPage(repo, SuggestedChange{Files: []ChangeFile{{Path: "src/app.tsx"}}}, ""); err == nil {
		t.Fatal("ambiguous framework entry guessed")
	}
}

func TestFocusedReviewContext(t *testing.T) {
	r := Investigation{Request: InvestigationRequest{Objective: "Improve site/index.html button contrast"}, Repository: Repository{Files: []RepoFile{{Path: "site/index.html"}, {Path: "unrelated.go"}}, Content: map[string]string{"site/index.html": "<button>Continue</button>", "unrelated.go": strings.Repeat("unrelated source\n", 1000)}}}
	prompt := reviewPrompt(r, nil)
	if !strings.Contains(prompt, "SOURCE site/index.html") || strings.Contains(prompt, "SOURCE unrelated.go") {
		t.Fatal("a focused review included unrelated source")
	}
	r.Request.Objective = "Review this repository"
	for i := 0; i < 20; i++ {
		p := strings.Repeat("a", i+1) + ".go"
		r.Repository.Files = append(r.Repository.Files, RepoFile{Path: p})
		r.Repository.Content[p] = strings.Repeat("package source\n", 400)
	}
	if n := len(reviewPrompt(r, nil)); n > 12500 {
		t.Fatalf("source context left insufficient local-model output capacity: %d bytes", n)
	}
}

// Opt-in integration test exercises the actual isolated product renderer.
func TestDockerUIComparison(t *testing.T) {
	if os.Getenv("VEDA_TEST_UI_PREVIEW") != "1" {
		t.Skip("set VEDA_TEST_UI_PREVIEW=1 with the screenshot runtime installed")
	}
	h := testHub(t)
	base := "<!doctype html><html><head><style>body{margin:0;background:#f4f6f4;font:24px sans-serif;padding:80px}button{background:#aaa;color:white;padding:20px;border:0}</style></head><body><h1>Review example</h1><p>A real static-page screenshot.</p><button>Continue</button></body></html>"
	r := Investigation{ID: NewID(), Repository: Repository{Name: "validation/static-page", Commit: "fixture", Content: map[string]string{"index.html": base}}, Images: map[string]string{}, Request: InvestigationRequest{PreviewPage: "index.html"}}
	// A different page must not cause a false partial-capture warning.
	r.Repository.Content["unrelated.html"] = `<img src="https://example.invalid/unrelated.png">`
	imageID, err := DockerReady(context.Background(), PreviewImage)
	if err != nil {
		t.Fatal(err)
	}
	r.Images["UI preview"] = imageID
	c := SuggestedChange{ID: "CHG-001", UIImpact: true, Files: []ChangeFile{{Path: "index.html", Before: base, After: strings.Replace(base, "background:#aaa", "background:#176b4b", 1)}}}
	p := h.captureChange(context.Background(), r, c)
	if p.Status != "captured" || p.ChangedPercent <= 0 || p.ChangedPercent > 15 {
		t.Fatalf("capture failed or unexpected visual change: %+v", p)
	}
	if out := os.Getenv("VEDA_PREVIEW_TEST_OUTPUT"); out != "" {
		if err := os.MkdirAll(out, 0700); err != nil {
			t.Fatal(err)
		}
		for _, name := range []string{"before", "after", "difference"} {
			b, err := os.ReadFile(filepath.Join(h.Root, "investigations", r.ID, "changes", c.ID, name+".png"))
			if err != nil {
				t.Fatal(err)
			}
			if err = os.WriteFile(filepath.Join(out, name+".png"), b, 0600); err != nil {
				t.Fatal(err)
			}
		}
	}
	t.Logf("Captured %dx%d comparison: %.3f%% changed", p.Width, p.Height, p.ChangedPercent)
	// Missing assets on the selected page must remain visible in the evidence.
	r.Repository.Content["index.html"] = strings.Replace(base, "</body>", `<img src="missing.png"></body>`, 1)
	c.Files[0].Before = r.Repository.Content["index.html"]
	c.Files[0].After = strings.Replace(c.Files[0].Before, "background:#aaa", "background:#176b4b", 1)
	c.ID = "CHG-002"
	p = h.captureChange(context.Background(), r, c)
	if p.Status != "partial" || len(p.Warnings) != 1 || !strings.Contains(p.Warnings[0], "missing.png") {
		t.Fatalf("missing asset evidence was not retained and deduplicated: %+v", p)
	}
}
