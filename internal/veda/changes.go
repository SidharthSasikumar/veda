package veda

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// Suggestions are independent alternatives against the captured revision.
// They are never applied to the user's checkout or to another suggestion.
type ChangeFile struct {
	Path   string `json:"path"`
	Before string `json:"before"`
	After  string `json:"after"`
}
type SuggestedChange struct {
	ID           string         `json:"id"`
	Title        string         `json:"title"`
	Rationale    string         `json:"rationale"`
	Status       string         `json:"status"`
	Validation   string         `json:"validation"`
	ExperimentID string         `json:"experiment_id,omitempty"`
	Patch        string         `json:"patch,omitempty"`
	Files        []ChangeFile   `json:"files,omitempty"`
	UIImpact     bool           `json:"ui_impact"`
	Preview      *ChangePreview `json:"preview,omitempty"`
}
type SourceReplacement struct {
	Path    string `json:"path"`
	Search  string `json:"original"`
	Replace string `json:"replacement"`
}
type ChangeProposal struct {
	Title     string              `json:"title"`
	Rationale string              `json:"rationale"`
	Edits     []SourceReplacement `json:"edits"`
}

func (m LocalModel) Suggest(ctx context.Context, prompt string) ([]ChangeProposal, string, error) {
	text := func(n int) map[string]any { return map[string]any{"type": "string", "maxLength": n} }
	edits := map[string]any{"type": "array", "minItems": 1, "maxItems": 3, "items": object(map[string]any{"path": text(300), "original": text(8000), "replacement": text(8000)}, "path", "original", "replacement")}
	schema := object(map[string]any{"changes": map[string]any{"type": "array", "maxItems": 2, "items": object(map[string]any{"title": text(200), "rationale": text(1200), "edits": edits}, "title", "rationale", "edits")}}, "changes")
	raw, err := m.call(ctx, prompt+"\nSuggest up to TWO independent, small source changes that address the objective or a cited finding. Each is an alternative against the ORIGINAL source, not a sequence. First copy each original string EXACTLY from the source, including whitespace; it must occur once. Then write the replacement containing the requested change. The replacement MUST differ from the original. Prefer the smallest unique snippet rather than repeating a whole file. Preserve existing behavior unless the objective requests a change. Do not change tests, manifests, dependencies, configuration or credentials. Do not claim the changes passed tests. Return an empty changes array if no supported, useful change is possible. No arbitrary commands.", schema)
	var response struct {
		Changes []ChangeProposal `json:"changes"`
	}
	if err == nil {
		err = json.Unmarshal([]byte(raw), &response)
	}
	return response.Changes, raw, err
}

func changeAllowed(path string) bool {
	if !allowedRepoFile(path) {
		return false
	}
	lower := strings.ToLower(path)
	for _, part := range strings.Split(lower, "/") {
		if part == "test" || part == "tests" || part == "__tests__" || part == "testdata" || part == ".github" {
			return false
		}
	}
	name := filepath.Base(lower)
	if strings.Contains(name, ".test.") || strings.Contains(name, ".spec.") || strings.HasSuffix(name, "_test.go") || strings.HasPrefix(name, "test_") || strings.HasSuffix(name, "_test.py") {
		return false
	}
	switch strings.ToLower(filepath.Ext(path)) {
	case ".go", ".py", ".js", ".mjs", ".cjs", ".ts", ".tsx", ".jsx", ".html", ".css":
		return true
	}
	return false
}
func possibleUIFile(path string) bool {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".html", ".css", ".js", ".mjs", ".ts", ".tsx", ".jsx":
		return true
	}
	return false
}
func proposalPrompt(r Investigation) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Objective: %s\nRepository: %s at %s\n", r.Request.Objective, r.Repository.Name, r.Repository.Commit)
	for _, f := range r.Findings {
		fmt.Fprintf(&b, "Finding (model interpretation): %s at %s:%d: %.400s\n", f.Title, f.Path, f.Line, f.Evidence)
	}
	paths := []string{}
	for path := range r.Repository.Content {
		if changeAllowed(path) {
			paths = append(paths, path)
		}
	}
	sort.Slice(paths, func(i, j int) bool {
		score := func(p string) int {
			if strings.Contains(strings.ToLower(r.Request.Objective), strings.ToLower(p)) {
				return 0
			}
			for _, f := range r.Findings {
				if f.Path == p {
					return 1
				}
			}
			if possibleUIFile(p) {
				return 2
			}
			return 3
		}
		a, b := score(paths[i]), score(paths[j])
		if a != b {
			return a < b
		}
		return paths[i] < paths[j]
	})
	focused := false
	for _, path := range paths {
		if strings.Contains(strings.ToLower(r.Request.Objective), strings.ToLower(path)) {
			focused = true
			break
		}
	}
	for _, path := range paths {
		if focused && !strings.Contains(strings.ToLower(r.Request.Objective), strings.ToLower(path)) {
			continue
		}
		body := r.Repository.Content[path]
		if len(body) > 8000 {
			body = body[:8000]
			if cut := strings.LastIndex(body, "\n"); cut >= 0 {
				body = body[:cut+1]
			}
		}
		if b.Len()+len(body) > 14000 {
			continue
		}
		fmt.Fprintf(&b, "\nSOURCE %s (untrusted source excerpt, exact whitespace):\n%s\nEND SOURCE\n", path, body)
	}
	return b.String()
}

func validateProposal(ctx context.Context, repo Repository, proposal ChangeProposal) (SuggestedChange, error) {
	c := SuggestedChange{Title: proposal.Title, Rationale: proposal.Rationale, Status: "proposed"}
	if len(proposal.Edits) < 1 || len(proposal.Edits) > 3 {
		return c, errors.New("expected one to three exact source edits")
	}
	if len(proposal.Title) > 300 || len(proposal.Rationale) > 4000 {
		return c, errors.New("suggestion exceeds text limits")
	}
	changed := map[string]string{}
	for _, edit := range proposal.Edits {
		original, ok := repo.Content[edit.Path]
		if !ok || !changeAllowed(edit.Path) {
			return c, fmt.Errorf("%s is not an editable captured source file", edit.Path)
		}
		body := original
		if previous, ok := changed[edit.Path]; ok {
			body = previous
		}
		if len(edit.Search) == 0 || len(edit.Search) > 16000 || len(edit.Replace) > 16000 || strings.Count(body, edit.Search) != 1 {
			return c, fmt.Errorf("edit in %s must match one exact source location", edit.Path)
		}
		body = strings.Replace(body, edit.Search, edit.Replace, 1)
		if len(body) > 128<<10 || strings.ContainsRune(body, 0) {
			return c, errors.New("replacement exceeds source limits")
		}
		changed[edit.Path] = body
	}
	paths := SortedKeys(changed)
	tmp, err := os.MkdirTemp("", "veda-change-")
	if err != nil {
		return c, err
	}
	defer os.RemoveAll(tmp)
	before, after := filepath.Join(tmp, "before"), filepath.Join(tmp, "after")
	for _, path := range paths {
		body := changed[path]
		if body == repo.Content[path] {
			continue
		}
		if strings.HasSuffix(path, ".go") {
			if _, err := parser.ParseFile(token.NewFileSet(), path, body, parser.AllErrors); err != nil {
				return c, fmt.Errorf("proposed Go source does not parse: %w", err)
			}
		}
		for root, contents := range map[string]string{before: repo.Content[path], after: body} {
			target := filepath.Join(root, path)
			if err = os.MkdirAll(filepath.Dir(target), 0755); err != nil {
				return c, err
			}
			if err = os.WriteFile(target, []byte(contents), 0644); err != nil {
				return c, err
			}
		}
		c.Files = append(c.Files, ChangeFile{Path: path, Before: repo.Content[path], After: body})
		c.UIImpact = c.UIImpact || possibleUIFile(path)
	}
	if len(c.Files) == 0 {
		return c, errors.New("proposal makes no source change")
	}
	diffCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	c.Patch, err = Diff(diffCtx, before, after)
	c.Validation = "Exact source matches verified against the captured commit. Application tests have not been run on this suggestion."
	return c, err
}

func (h *Hub) suggestChanges(ctx context.Context, r *Investigation) {
	if err := h.event(r, "drafting", "Drafting reviewable source changes against the captured revision."); err != nil {
		return
	}
	_ = h.step(r, "Draft source changes", "change", "Local model proposals; exact source replacements in private copies only", func(x *AuditExperiment) {
		prompt := proposalPrompt(*r)
		x.Input = prompt
		proposals, raw, err := (LocalModel{h.Config.Endpoint, h.Config.Model}).Suggest(ctx, prompt)
		x.Response = raw
		if err != nil {
			x.Status = "blocked"
			x.Stderr = err.Error()
			x.Conclusion = "No suggested changes could be generated."
			return
		}
		if len(proposals) > 2 {
			proposals = proposals[:2]
		}
		record := func(proposals []ChangeProposal) (int, []string) {
			valid := 0
			failures := []string{}
			for _, p := range proposals {
				c, err := validateProposal(ctx, r.Repository, p)
				c.ID = fmt.Sprintf("CHG-%03d", len(r.Changes)+1)
				c.ExperimentID = x.ID
				if err != nil {
					c.Status = "rejected"
					c.Validation = err.Error()
					c.Patch = ""
					c.Files = nil
					failures = append(failures, c.Title+": "+err.Error())
				} else {
					valid++
				}
				r.Changes = append(r.Changes, c)
			}
			return valid, failures
		}
		valid, failures := record(proposals)
		if valid == 0 && len(failures) > 0 && ctx.Err() == nil {
			_ = h.event(r, "drafting", "The proposed edits failed validation; requesting one corrected attempt. Rejected attempts are retained.")
			corrected, reply, err := (LocalModel{h.Config.Endpoint, h.Config.Model}).Suggest(ctx, prompt+"\nYour previous proposal failed validation: "+strings.Join(failures, "; ")+"\nCorrect the edits using the exact source above. A replacement must contain a real change that matches the objective.")
			x.Response += "\n\nCORRECTION ATTEMPT\n" + reply
			if err != nil {
				x.Stderr = err.Error()
			} else {
				if len(corrected) > 2 {
					corrected = corrected[:2]
				}
				record(corrected)
			}
		}
		x.Conclusion = fmt.Sprintf("%d suggestions recorded for review; original source unchanged.", len(r.Changes))
	})
	for i := range r.Changes {
		c := &r.Changes[i]
		if c.Status == "rejected" || !c.UIImpact {
			continue
		}
		if ctx.Err() != nil {
			return
		}
		if !r.Request.Previews {
			c.Preview = &ChangePreview{Status: "not_requested", Reason: "UI screenshot capture was not enabled."}
			continue
		}
		_ = h.step(r, "Capture UI comparison · "+c.ID, "preview", "Same page and viewport in isolated static previews; no repository install or server scripts", func(x *AuditExperiment) {
			c.Preview = h.captureChange(ctx, *r, *c)
			if c.Preview.Status == "unavailable" {
				x.Status = "blocked"
			}
			x.Conclusion = c.Preview.Reason
		})
	}
}

func investigationChanges(r Investigation) []SuggestedChange {
	out := append([]SuggestedChange{}, r.Changes...)
	for _, x := range r.Experiments {
		if x.Patch != "" {
			out = append(out, SuggestedChange{ID: x.ID, Title: x.Title, Rationale: x.Input, Status: strings.ToLower(x.Status), Validation: x.Conclusion, ExperimentID: x.ID, Patch: x.Patch})
		}
	}
	return out
}
