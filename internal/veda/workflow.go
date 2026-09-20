package veda

import "strings"

// WorkflowTask captures only the evidence available at an event's timestamp.
// Full outputs remain in Experiments, avoiding duplicated source/model payloads.
type WorkflowTask struct {
	ID         string  `json:"id"`
	Title      string  `json:"title"`
	Kind       string  `json:"kind"`
	Status     string  `json:"status"`
	Started    string  `json:"started,omitempty"`
	Duration   float64 `json:"duration_seconds,omitempty"`
	Conclusion string  `json:"conclusion,omitempty"`
	Parent     string  `json:"parent,omitempty"`
}

func stageActor(stage string) string {
	return map[string]string{"cloning": "scout", "environment": "tester", "architecture": "mapper", "checks": "tester", "reasoning": "thinker", "optimizing": "tinkerer", "reporting": "keeper", "reusing": "keeper"}[stage]
}

func taskActor(x AuditExperiment) string {
	switch x.Kind {
	case "source":
		return "scout"
	case "model":
		return "thinker"
	case "candidate":
		return "tinkerer"
	}
	if x.Kind == "static" && x.Title == "Map declared architecture" {
		return "mapper"
	}
	return "tester"
}

func workflowEvent(r *Investigation, event Event) {
	var last int64
	for _, prior := range r.Events {
		if prior.ID > last {
			last = prior.ID
		}
	}
	event.ID, event.RunID, event.Version = last+1, r.ID, 1
	if event.At == "" {
		event.At = now()
	}
	if event.Stage == "" {
		event.Stage = r.Stage
	}
	r.Events = append(r.Events, event)
}

func taskEvent(r *Investigation, kind string, x AuditExperiment) {
	workflowEvent(r, Event{Type: kind, Actor: taskActor(x), TaskID: x.ID, Status: x.Status,
		Message: x.Title + " · " + x.Status,
		Task:    &WorkflowTask{ID: x.ID, Title: x.Title, Kind: x.Kind, Status: x.Status, Started: x.Started, Duration: x.Duration, Conclusion: x.Conclusion, Parent: x.Parent}})
}

func finishWorkflow(r *Investigation) {
	// A process stop can leave an in-flight record; preserve completed evidence.
	for i := range r.Experiments {
		x := &r.Experiments[i]
		if x.Status != "running" {
			continue
		}
		x.Status = "interrupted"
		if r.Status == "cancelled" {
			x.Status = "cancelled"
		}
		x.Conclusion = "Investigation stopped before this task produced an outcome."
		if r.HistoryVersion > 0 {
			taskEvent(r, "task."+x.Status, *x)
		}
	}
	if r.HistoryVersion > 0 {
		workflowEvent(r, Event{Type: "run.finished", Status: r.Status, Actor: "keeper", Message: "Investigation " + r.Status + "; report and recorded evidence retained."})
	}
}

func outcomeEvent(status string) string {
	switch strings.ToLower(status) {
	case "failed", "rejected", "not_supported":
		return "task.failed"
	case "blocked":
		return "task.blocked"
	case "skipped":
		return "task.skipped"
	case "cancelled", "interrupted":
		return "task." + strings.ToLower(status)
	default:
		return "task.completed"
	}
}
