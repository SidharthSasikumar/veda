package allocdemo

import (
	"fmt"
	"strings"
	"testing"
)

func TestRender(t *testing.T) {
	cases := [][]string{nil, {}, {""}, {"alpha", "beta"}, {"日本語", "a\nb", "<>&", "é"}}
	for _, values := range cases {
		var want strings.Builder
		for i, value := range values {
			fmt.Fprintf(&want, "%d:%s\n", i, value)
		}
		if got := Render(values); got != want.String() {
			t.Fatalf("Render(%q) = %q; want %q", values, got, want.String())
		}
	}
}
func TestRenderLarge(t *testing.T) {
	values := make([]string, 1200)
	for i := range values {
		values[i] = strings.Repeat("x", i%100)
	}
	var want strings.Builder
	for i, value := range values {
		fmt.Fprintf(&want, "%d:%s\n", i, value)
	}
	if Render(values) != want.String() {
		t.Fatal("large output mismatch")
	}
}

var sink string

func BenchmarkRender(b *testing.B) {
	values := make([]string, 200)
	for i := range values {
		values[i] = "research-evidence"
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		sink = Render(values)
	}
}
