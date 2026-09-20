package veda

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

func (s *Server) repositoryRoutes(mux *http.ServeMux) {
	requireHub := func(w http.ResponseWriter) *Hub {
		if s.Hub == nil {
			apiError(w, errors.New("repository knowledge store is not enabled"), 503)
			return nil
		}
		return s.Hub
	}
	mux.HandleFunc("GET /api/library", func(w http.ResponseWriter, r *http.Request) {
		h := requireHub(w)
		if h == nil {
			return
		}
		runs, err := h.List()
		if err != nil {
			apiError(w, err, 500)
			return
		}
		summaries := []map[string]any{}
		repos := map[string]bool{}
		revisions := map[string]bool{}
		reused := 0
		for _, run := range runs {
			work := []map[string]any{}
			for _, step := range run.Experiments {
				work = append(work, map[string]any{"id": step.ID, "title": step.Title, "kind": step.Kind, "status": step.Status, "duration_seconds": step.Duration})
			}
			var activity *Event
			if len(run.Events) > 0 {
				activity = &run.Events[len(run.Events)-1]
			}
			if run.Repository.Name != "" {
				repos[run.Repository.Name] = true
				revisions[run.Repository.Name+"@"+run.Repository.Commit] = true
			}
			if run.ReusedFrom != "" {
				reused++
			}
			summaries = append(summaries, map[string]any{"id": run.ID, "objective": run.Request.Objective, "repository": run.Repository.Name, "url": run.Request.URL, "commit": run.Repository.Commit, "status": run.Status, "stage": run.Stage, "created": run.Created, "finished": run.Finished, "experiments": len(run.Experiments), "findings": len(run.Findings), "reused_from": run.ReusedFrom, "work": work, "request": run.Request, "activity": activity, "has_report": run.Report != ""})
		}
		jsonOut(w, map[string]any{"investigations": summaries, "active": h.Active(), "knowledge_dir": h.Root, "repositories": len(repos), "revisions": len(revisions), "reused": reused, "token": s.token, "model": s.Config.Model, "analyzer": AnalyzerVersion})
	})
	mux.HandleFunc("POST /api/investigations", func(w http.ResponseWriter, r *http.Request) {
		h := requireHub(w)
		if h == nil {
			return
		}
		var in InvestigationRequest
		decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8192))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&in); err != nil {
			apiError(w, err, 400)
			return
		}
		if decoder.Decode(&struct{}{}) != io.EOF {
			apiError(w, errors.New("send one JSON object"), 400)
			return
		}
		s.mu.Lock()
		defer s.mu.Unlock()
		if s.active != "" {
			apiError(w, errors.New("a benchmark investigation is running"), 409)
			return
		}
		if h.Active() != "" {
			apiError(w, errors.New("a repository investigation is running"), 409)
			return
		}
		id, err := h.Start(in)
		if err != nil {
			apiError(w, err, 400)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusAccepted)
		jsonOut(w, map[string]string{"id": id})
	})
	mux.HandleFunc("POST /api/investigations/cancel", func(w http.ResponseWriter, r *http.Request) {
		h := requireHub(w)
		if h == nil {
			return
		}
		h.Cancel()
		jsonOut(w, map[string]bool{"ok": true})
	})
	mux.HandleFunc("GET /api/investigations/{id}", func(w http.ResponseWriter, r *http.Request) {
		h := requireHub(w)
		if h == nil {
			return
		}
		run, err := h.Get(r.PathValue("id"))
		if err != nil {
			apiError(w, err, 404)
			return
		}
		jsonOut(w, run)
	})
	mux.HandleFunc("GET /api/investigations/{id}/report", func(w http.ResponseWriter, r *http.Request) {
		h := requireHub(w)
		if h == nil {
			return
		}
		run, err := h.Get(r.PathValue("id"))
		if err != nil {
			apiError(w, err, 404)
			return
		}
		w.Header().Set("Content-Type", "text/markdown; charset=utf-8")
		w.Header().Set("Content-Disposition", `attachment; filename="veda-investigation.md"`)
		_, _ = io.WriteString(w, run.Report)
	})
	mux.HandleFunc("GET /api/investigations/{id}/evidence", func(w http.ResponseWriter, r *http.Request) {
		h := requireHub(w)
		if h == nil {
			return
		}
		run, err := h.Get(r.PathValue("id"))
		if err != nil {
			apiError(w, err, 404)
			return
		}
		w.Header().Set("Content-Disposition", `attachment; filename="veda-evidence.json"`)
		jsonOut(w, run)
	})
	mux.HandleFunc("GET /api/investigations/{id}/source", func(w http.ResponseWriter, r *http.Request) {
		h := requireHub(w)
		if h == nil {
			return
		}
		run, err := h.Get(r.PathValue("id"))
		if err != nil {
			apiError(w, err, 404)
			return
		}
		path := r.URL.Query().Get("path")
		found := false
		checksum := ""
		for _, file := range run.Repository.Files {
			if file.Path == path {
				found = true
				checksum = file.SHA256
				break
			}
		}
		if !found || !allowedRepoFile(path) {
			apiError(w, errors.New("source is outside this investigation"), 404)
			return
		}
		root, err := os.OpenRoot(filepath.Join(h.Root, "investigations", run.ID, "source"))
		if err != nil {
			apiError(w, err, 404)
			return
		}
		defer root.Close()
		content, err := root.ReadFile(path)
		if err != nil {
			apiError(w, err, 404)
			return
		}
		if checksum == "" || digest(string(content)) != checksum {
			apiError(w, errors.New("captured source checksum mismatch"), 409)
			return
		}
		jsonOut(w, map[string]string{"path": path, "content": string(content), "revision": run.Repository.Commit})
	})
	lookupChange := func(w http.ResponseWriter, r *http.Request) (*Hub, Investigation, *SuggestedChange) {
		h := requireHub(w)
		if h == nil {
			return nil, Investigation{}, nil
		}
		run, err := h.Get(r.PathValue("id"))
		if err != nil {
			apiError(w, err, 404)
			return nil, run, nil
		}
		for _, c := range investigationChanges(run) {
			if c.ID == r.PathValue("change") {
				return h, run, &c
			}
		}
		apiError(w, errors.New("suggested change not found"), 404)
		return nil, run, nil
	}
	mux.HandleFunc("GET /api/investigations/{id}/changes/{change}/patch", func(w http.ResponseWriter, r *http.Request) {
		_, _, c := lookupChange(w, r)
		if c == nil {
			return
		}
		if c.Patch == "" {
			apiError(w, errors.New("no applicable patch recorded for this suggestion"), 404)
			return
		}
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.Header().Set("Content-Disposition", `attachment; filename="veda-suggested-change.patch"`)
		_, _ = io.WriteString(w, c.Patch)
	})
	mux.HandleFunc("GET /api/investigations/{id}/changes/{change}/preview/{image}", func(w http.ResponseWriter, r *http.Request) {
		h, run, c := lookupChange(w, r)
		if c == nil {
			return
		}
		name := r.PathValue("image")
		if name != "before" && name != "after" && name != "difference" {
			apiError(w, errors.New("unknown preview image"), 404)
			return
		}
		p := c.Preview
		if p == nil || (p.Status != "captured" && p.Status != "partial") || p.Hashes[name] == "" || !strings.HasPrefix(p.RunID, "RUN-") || filepath.Base(p.RunID) != p.RunID || filepath.Base(c.ID) != c.ID {
			apiError(w, errors.New("no captured UI preview"), 404)
			return
		}
		origin, err := h.Get(p.RunID)
		if err != nil || origin.Repository.Commit != run.Repository.Commit || origin.Repository.Name != run.Repository.Name {
			apiError(w, errors.New("preview provenance does not match this revision"), 409)
			return
		}
		root, err := os.OpenRoot(filepath.Join(h.Root, "investigations", p.RunID, "changes", c.ID))
		if err != nil {
			apiError(w, err, 404)
			return
		}
		defer root.Close()
		data, err := root.ReadFile(name + ".png")
		if err != nil {
			apiError(w, err, 404)
			return
		}
		if len(data) > 8<<20 || digest(string(data)) != p.Hashes[name] {
			apiError(w, errors.New("preview checksum mismatch"), 409)
			return
		}
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write(data)
	})
}
