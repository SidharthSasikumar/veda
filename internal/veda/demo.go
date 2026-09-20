package veda

import (
	"context"
	"encoding/json"
)

// DemoModel uses fixed proposals; only Docker tests and benchmarks are measured.
type DemoModel struct{ step int }

func (d *DemoModel) Plan(_ context.Context, _ string) ([]Hypothesis, string, error) {
	h := []Hypothesis{{"Remove all formatting work", "Negative control: tests must reject a behavior-breaking optimization."}, {"Replace repeated concatenation with a builder", "Retain fmt formatting, amortize buffer growth."}, {"Preallocate the output buffer and write integers directly", "Avoid formatting allocations and intermediate strings."}}
	b, _ := json.Marshal(map[string]any{"hypotheses": h})
	return h, string(b), nil
}
func (d *DemoModel) Propose(_ context.Context, _ string) (Proposal, string, error) {
	codes := []string{
		`package allocdemo
func Render(values []string) string { return "" }
`,
		`package allocdemo
import("fmt";"strings")
func Render(values []string) string { var out strings.Builder; for i,v := range values { fmt.Fprintf(&out,"%d:%s\n",i,v) }; return out.String() }
`,
		`package allocdemo
import "strconv"
func Render(values []string) string { n:=0; for _,v:=range values {n+=len(v)+24}; out:=make([]byte,0,n); for i,v:=range values {out=strconv.AppendInt(out,int64(i),10);out=append(out,':');out=append(out,v...);out=append(out,'\n')};return string(out) }
`}
	p := Proposal{Title: "Scripted demonstration candidate", Rationale: "Fixed proposal for pipeline verification", Edits: []Edit{{"render.go", codes[d.step%len(codes)]}}}
	d.step++
	b, _ := json.Marshal(p)
	return p, string(b), nil
}
