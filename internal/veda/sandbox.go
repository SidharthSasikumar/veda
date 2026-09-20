package veda

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

type Sample struct {
	NS     float64 `json:"ns_op"`
	Bytes  float64 `json:"bytes_op"`
	Allocs float64 `json:"allocs_op"`
}
type Measurement struct {
	Benchmark   string   `json:"benchmark"`
	Samples     []Sample `json:"samples"`
	NS          float64  `json:"ns_op"`
	Bytes       float64  `json:"bytes_op"`
	Allocs      float64  `json:"allocs_op"`
	TestsPassed bool     `json:"tests_passed"`
	Duration    float64  `json:"duration_seconds"`
}

func (m Measurement) Value(metric string) float64 {
	switch metric {
	case "B/op":
		return m.Bytes
	case "allocs/op":
		return m.Allocs
	default:
		return m.NS
	}
}
func ParseBench(out string) (Measurement, error) {
	m := Measurement{Samples: []Sample{}}
	names := map[string]bool{}
	suffix := regexp.MustCompile(`-\d+$`)
	for _, line := range strings.Split(out, "\n") {
		f := strings.Fields(line)
		if len(f) < 8 || !strings.HasPrefix(f[0], "Benchmark") {
			continue
		}
		if _, e := strconv.Atoi(f[1]); e != nil {
			continue
		}
		name := suffix.ReplaceAllString(f[0], "")
		s := Sample{}
		units := map[string]bool{}
		for i := 2; i+1 < len(f); i += 2 {
			v, e := strconv.ParseFloat(f[i], 64)
			if e != nil {
				return m, e
			}
			switch f[i+1] {
			case "ns/op":
				s.NS = v
				units["ns"] = true
			case "B/op":
				s.Bytes = v
				units["b"] = true
			case "allocs/op":
				s.Allocs = v
				units["a"] = true
			}
		}
		if len(units) != 3 {
			return m, errors.New("benchmark output lacks required ns/op, B/op and allocs/op")
		}
		names[name] = true
		m.Benchmark = name
		m.Samples = append(m.Samples, s)
	}
	if len(names) != 1 {
		return m, fmt.Errorf("select exactly one benchmark with --benchmark; found %v", SortedKeys(names))
	}
	if len(m.Samples) != 3 {
		return m, fmt.Errorf("expected 3 benchmark samples; got %d", len(m.Samples))
	}
	median := func(f func(Sample) float64) float64 {
		a := []float64{}
		for _, s := range m.Samples {
			a = append(a, f(s))
		}
		sort.Float64s(a)
		return a[len(a)/2]
	}
	m.NS = median(func(s Sample) float64 { return s.NS })
	m.Bytes = median(func(s Sample) float64 { return s.Bytes })
	m.Allocs = median(func(s Sample) float64 { return s.Allocs })
	return m, nil
}

type Runner interface {
	Measure(context.Context, string, string, Config) (Measurement, error)
}
type DockerRunner struct{}
type boundedBuffer struct {
	bytes.Buffer
	limit int
}

func (b *boundedBuffer) Write(p []byte) (int, error) {
	n := len(p)
	if b.Len() < b.limit {
		remaining := b.limit - b.Len()
		if len(p) > remaining {
			p = p[:remaining]
		}
		b.Buffer.Write(p)
	}
	return n, nil
}
func DockerReady(ctx context.Context, image string) (string, error) {
	b, e := exec.CommandContext(ctx, "docker", "image", "inspect", "--format", "{{.Id}}", image).CombinedOutput()
	if e != nil {
		return "", fmt.Errorf("Docker image unavailable: start Docker and run docker pull %s: %.500s", image, b)
	}
	return strings.TrimSpace(string(b)), nil
}
func (DockerRunner) Measure(ctx context.Context, source, dir string, c Config) (Measurement, error) {
	m := Measurement{}
	ctx, cancel := context.WithTimeout(ctx, time.Duration(c.TimeoutSeconds)*time.Second)
	defer cancel()
	name := "veda-" + strings.ToLower(strings.ReplaceAll(NewID(), ".", "-"))
	// No network, privileges, host credentials, Docker socket, or writable host mount.
	script := `set -eu
cp -R /src/. /work/
cd /work
if [ -d vendor ]; then export GOFLAGS=-mod=vendor; else export GOFLAGS=-mod=readonly; fi
go test ./... -count=1 -timeout=60s
printf '\nVEDA_TESTS_PASSED\n'
go test ./... -run '^$' -bench "$VEDA_BENCH" -benchmem -count=3 -benchtime=150ms -timeout=90s
`
	args := []string{"run", "--pull=never", "--name", name, "--rm", "--network=none", "--read-only", "--cap-drop=ALL", "--security-opt=no-new-privileges", "--pids-limit=256", "--memory=2g", "--memory-swap=2g", "--cpus=2", "--user=65534:65534", "--tmpfs=/tmp:rw,exec,nosuid,size=768m,mode=1777", "--tmpfs=/work:rw,nosuid,size=128m,mode=1777", "--mount", "type=bind,src=" + source + ",dst=/src,readonly", "-e", "GOCACHE=/tmp/gocache", "-e", "GOMODCACHE=/tmp/gomodcache", "-e", "GOPROXY=off", "-e", "GOSUMDB=off", "-e", "GOTOOLCHAIN=local", "-e", "CGO_ENABLED=0", "-e", "GOMAXPROCS=2", "-e", "VEDA_BENCH=" + c.Benchmark, c.Image, "sh", "-c", script}
	if strings.Contains(source, ",") {
		return m, errors.New("workspace path cannot contain a comma")
	}
	if e := WriteJSON(filepath.Join(dir, "command.json"), map[string]any{"program": "docker", "args": args}); e != nil {
		return m, e
	}
	stdout := &boundedBuffer{limit: 2 << 20}
	stderr := &boundedBuffer{limit: 2 << 20}
	cmd := exec.CommandContext(ctx, "docker", args...)
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	start := time.Now()
	err := cmd.Run()
	if ctx.Err() != nil {
		clean, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		exec.CommandContext(clean, "docker", "rm", "-f", name).Run()
		cancel()
		err = ctx.Err()
	}
	if e := os.WriteFile(filepath.Join(dir, "stdout.log"), stdout.Bytes(), 0600); e != nil {
		return m, e
	}
	if e := os.WriteFile(filepath.Join(dir, "stderr.log"), stderr.Bytes(), 0600); e != nil {
		return m, e
	}
	if err != nil {
		return m, fmt.Errorf("sandbox failed (%v): %.1200s %.1200s", err, stderr.String(), stdout.String())
	}
	if !strings.Contains(stdout.String(), "VEDA_TESTS_PASSED") {
		return m, errors.New("tests did not finish successfully")
	}
	m, e := ParseBench(stdout.String())
	m.Duration = time.Since(start).Seconds()
	m.TestsPassed = e == nil
	return m, e
}
func Diff(ctx context.Context, baseline, candidate string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", "diff", "--no-index", "--no-ext-diff", "--no-prefix", baseline, candidate)
	b, e := cmd.CombinedOutput()
	if exit, ok := e.(*exec.ExitError); ok && exit.ExitCode() == 1 {
		e = nil
	}
	if e != nil {
		return "", fmt.Errorf("diff failed: %w", e)
	}
	out := string(b)
	out = strings.ReplaceAll(out, strings.TrimPrefix(baseline, "/")+"/", "a/")
	out = strings.ReplaceAll(out, strings.TrimPrefix(candidate, "/")+"/", "b/")
	return out, nil
}
