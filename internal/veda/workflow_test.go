package veda

import (
	"encoding/json"
	"testing"
	"time"
)

func TestWorkflowHistoryMatchesPersistedTasks(t *testing.T) {
	h := testHub(t)
	r := finishInvestigation(t, h, InvestigationRequest{URL: "https://github.com/a/b", Objective: "Explain this repository"})
	if r.HistoryVersion != 1 || r.Events[0].Type != "run.started" || r.Events[len(r.Events)-1].Type != "run.finished" {
		t.Fatal("missing workflow boundaries")
	}
	started, ended := map[string]Event{}, map[string]Event{}
	handoffs := 0
	for i, e := range r.Events {
		if e.ID != int64(i+1) || e.RunID != r.ID || e.Version != 1 {
			t.Fatalf("invalid identity: %+v", e)
		}
		if _, err := time.Parse(time.RFC3339Nano, e.At); err != nil {
			t.Fatal(err)
		}
		if e.Type == "task.started" {
			started[e.TaskID] = e
		}
		if e.Task != nil && e.Type != "task.started" {
			ended[e.TaskID] = e
		}
		if e.Type == "artifact.handoff" {
			handoffs++
			if e.Actor == e.Recipient || e.Actor == "" || e.Recipient == "" || len(e.ArtifactIDs) == 0 {
				t.Fatal("empty or self handoff")
			}
			for _, id := range e.ArtifactIDs {
				outcome, ok := ended[id]
				if !ok || outcome.ID >= e.ID || outcome.Actor != e.Actor {
					t.Fatalf("handoff before its evidence: %+v", e)
				}
			}
		}
	}
	if handoffs < 2 {
		t.Fatal("stage transitions missing")
	}
	for _, task := range r.Experiments {
		if started[task.ID].Task == nil || started[task.ID].Task.Status != "running" {
			t.Fatal("start snapshot overwritten")
		}
		if ended[task.ID].Task == nil || ended[task.ID].Task.Status != task.Status {
			t.Fatal("outcome and task disagree")
		}
	}
}

func TestWorkflowReuseKeepsOnlyCurrentExecutionEvents(t *testing.T) {
	h := testHub(t)
	in := InvestigationRequest{URL: "https://github.com/a/b", Objective: "Explain this repository"}
	first := finishInvestigation(t, h, in)
	second := finishInvestigation(t, h, in)
	found := false
	for _, e := range second.Events {
		if e.RunID != second.ID {
			t.Fatal("copied prior execution events")
		}
		if e.Type == "knowledge.reused" {
			found = true
		}
		if e.Task != nil && e.Task.Kind != "source" {
			t.Fatal("reused evidence claimed as fresh checks")
		}
	}
	if !found || second.ReusedFrom != first.ID {
		t.Fatal("missing reuse provenance")
	}
}

func TestWorkflowFailureAndInterruptedTasks(t *testing.T) {
	h := testHub(t)
	h.Loader = fixtureLoader{fail: true}
	r := finishInvestigation(t, h, InvestigationRequest{URL: "https://github.com/a/b", Objective: "Explain this repository"})
	if r.Events[len(r.Events)-1].Status != "failed" {
		t.Fatal("failed run celebrated")
	}
	found := false
	for _, e := range r.Events {
		if e.Type == "task.failed" {
			found = true
		}
	}
	if !found {
		t.Fatal("missing failure event")
	}
	pending := Investigation{ID: "interrupted", HistoryVersion: 1, Status: "cancelled", Stage: "finished", Experiments: []AuditExperiment{{ID: "one", Kind: "runtime", Status: "running"}}}
	finishWorkflow(&pending)
	if pending.Experiments[0].Status != "cancelled" || pending.Events[0].Type != "task.cancelled" {
		t.Fatal("unfinished work remains running")
	}
}

func TestLegacyInvestigationDecodeRemainsCompatible(t *testing.T) {
	data := `{"id":"old","status":"completed","events":[{"id":1,"at":"2026-09-19T10:00:00Z","message":"Saved"}],"experiments":[]}`
	r, err := decodeInvestigation(data, digest(data))
	if err != nil {
		t.Fatal(err)
	}
	if r.HistoryVersion != 0 || r.Events[0].Type != "" || r.Events[0].Message != "Saved" {
		t.Fatal("legacy event changed")
	}
	encoded, err := json.Marshal(r)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = decodeInvestigation(string(encoded), digest(string(encoded))); err != nil {
		t.Fatal(err)
	}
}
