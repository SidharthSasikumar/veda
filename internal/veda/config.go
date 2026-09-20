package veda

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"time"
)

const DefaultModel = "Qwen3.5-9B-Q4_K_M"
const DefaultImage = "golang:1.26.4-alpine"

type Config struct {
	Version        int     `json:"version"`
	Name           string  `json:"name"`
	Model          string  `json:"model"`
	Endpoint       string  `json:"endpoint"`
	Benchmark      string  `json:"benchmark"`
	Metric         string  `json:"metric"`
	Target         float64 `json:"target_percent"`
	MaxExperiments int     `json:"max_experiments"`
	Image          string  `json:"image"`
	TimeoutSeconds int     `json:"timeout_seconds"`
}

func Init(root string) (Config, error) {
	c := Config{1, filepath.Base(root), DetectHardware().Model, "http://127.0.0.1:18080/v1", "Benchmark", "B/op", 20, 3, DefaultImage, 240}
	if _, err := os.Stat(filepath.Join(root, "go.mod")); err != nil {
		return c, errors.New("workspace must contain go.mod; try the bundled examples/allocdemo")
	}
	p := filepath.Join(root, ".veda", "project.json")
	if _, err := os.Stat(p); err == nil {
		return LoadConfig(root)
	}
	if err := os.MkdirAll(filepath.Dir(p), 0700); err != nil {
		return c, err
	}
	if err := WriteJSON(p, c); err != nil {
		return c, err
	}
	return c, nil
}
func LoadConfig(root string) (Config, error) {
	var c Config
	b, e := os.ReadFile(filepath.Join(root, ".veda", "project.json"))
	if e != nil {
		return c, fmt.Errorf("run veda init first: %w", e)
	}
	e = json.Unmarshal(b, &c)
	if e != nil {
		return c, e
	}
	return c, c.Validate()
}
func (c Config) Validate() error {
	if e := ValidateEndpoint(c.Endpoint); e != nil {
		return e
	}
	if c.Metric != "B/op" && c.Metric != "allocs/op" && c.Metric != "ns/op" {
		return errors.New("metric must be B/op, allocs/op or ns/op")
	}
	if c.MaxExperiments < 3 || c.MaxExperiments > 8 {
		return errors.New("max_experiments must be between 3 and 8")
	}
	if c.Target <= 0 || c.Target > 100 {
		return errors.New("target_percent must be > 0 and <= 100")
	}
	if c.TimeoutSeconds < 30 || c.TimeoutSeconds > 900 {
		return errors.New("timeout_seconds must be between 30 and 900")
	}
	if _, err := regexp.Compile(c.Benchmark); err != nil {
		return fmt.Errorf("invalid benchmark regex: %w", err)
	}
	if c.Benchmark == "" || len(c.Benchmark) > 200 {
		return errors.New("benchmark selector required")
	}
	if !strings.HasPrefix(c.Image, "golang:") && !strings.HasPrefix(c.Image, "golang@") {
		return errors.New("use a golang image, optionally pinned by digest")
	}
	return nil
}
func ValidateEndpoint(raw string) error {
	u, e := url.Parse(raw)
	if e != nil || u.Scheme != "http" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return errors.New("model endpoint must be a local http URL without credentials or query")
	}
	// Literal loopback prevents DNS rebinding and accidental cloud calls.
	ip := net.ParseIP(u.Hostname())
	if ip == nil || !ip.IsLoopback() {
		return errors.New("model endpoint must use a literal loopback IP, e.g. http://127.0.0.1:18080/v1")
	}
	return nil
}
func WriteJSON(path string, v any) error {
	b, e := json.MarshalIndent(v, "", "  ")
	if e != nil {
		return e
	}
	return os.WriteFile(path, append(b, '\n'), 0600)
}
func NewID() string { return "RUN-" + time.Now().UTC().Format("20060102-150405.000000000") }

type Hardware struct {
	OS       string `json:"os"`
	Arch     string `json:"arch"`
	CPU      string `json:"cpu"`
	Cores    int    `json:"cores"`
	MemoryGB int    `json:"memory_gb"`
	Profile  string `json:"profile"`
	Model    string `json:"model"`
	Context  int    `json:"context"`
}

func DetectHardware() Hardware {
	h := Hardware{OS: runtime.GOOS, Arch: runtime.GOARCH, Cores: runtime.NumCPU(), CPU: runtime.GOARCH, Profile: "light", Model: "Qwen3.5-4B-Q4_K_M", Context: 8192}
	if runtime.GOOS == "darwin" {
		b, _ := exec.Command("sysctl", "-n", "hw.memsize").Output()
		n, _ := strconv.ParseInt(strings.TrimSpace(string(b)), 10, 64)
		h.MemoryGB = int(n / (1 << 30))
		b, _ = exec.Command("sysctl", "-n", "machdep.cpu.brand_string").Output()
		if len(b) > 0 {
			h.CPU = strings.TrimSpace(string(b))
		}
	}
	if h.MemoryGB >= 24 {
		h.Profile = "balanced"
		h.Model = DefaultModel
	}
	return h
}
