package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"veda.local/veda/internal/veda"
)

func main() {
	if e := run(); e != nil {
		fmt.Fprintln(os.Stderr, "Veda:", e)
		os.Exit(1)
	}
}
func run() error {
	if len(os.Args) < 2 {
		help()
		return nil
	}
	command := os.Args[1]
	if command == "help" || command == "--help" {
		help()
		return nil
	}
	if command == "version" {
		fmt.Println("Veda 0.2.0")
		return nil
	}
	args := os.Args[2:]
	if command == "model" && len(args) > 0 && args[0] == "setup" {
		args = args[1:]
	}
	f := flag.NewFlagSet(command, flag.ContinueOnError)
	workspace := f.String("workspace", ".", "Go module to investigate")
	knowledge := f.String("knowledge-dir", veda.DefaultKnowledgeDir(), "shared repository knowledge directory")
	address := f.String("addr", "127.0.0.1:8787", "local dashboard address")
	demo := f.Bool("demo", false, "scripted demonstration (bundled example only)")
	benchmark := f.String("benchmark", "", "single Go benchmark regex")
	metric := f.String("metric", "", "B/op, allocs/op or ns/op")
	target := f.Float64("target", 0, "target reduction percentage")
	max := f.Int("max-experiments", 0, "3–8 experiment budget")
	profile := f.String("profile", "auto", "auto, light or balanced")
	if e := f.Parse(args); e != nil {
		return e
	}
	root, e := filepath.Abs(*workspace)
	if e != nil {
		return e
	}
	root, e = filepath.EvalSymlinks(root)
	if e != nil {
		return e
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	if command == "init" {
		c, e := veda.Init(root)
		if e == nil {
			fmt.Printf("Initialized %s\nModel: %s\n", root, c.Model)
		}
		return e
	}
	c, e := veda.LoadConfig(root)
	if e != nil && command != "doctor" {
		return e
	}
	if command == "doctor" && e != nil {
		c = veda.Config{Endpoint: "http://127.0.0.1:18080/v1", Model: veda.DefaultModel, Image: veda.DefaultImage}
	}
	if *benchmark != "" {
		c.Benchmark = *benchmark
	}
	if *metric != "" {
		c.Metric = *metric
	}
	if *target != 0 {
		c.Target = *target
	}
	if *max != 0 {
		c.MaxExperiments = *max
	}
	if command == "doctor" {
		printJSON(veda.Doctor(ctx, c))
		return nil
	}
	if command == "model" {
		h := veda.DetectHardware()
		if *profile == "light" {
			h.Model = "Qwen3.5-4B-Q4_K_M"
			h.Profile = "light"
		} else if *profile == "balanced" {
			h.Model = veda.DefaultModel
			h.Profile = "balanced"
		} else if *profile != "auto" {
			return fmt.Errorf("supported profiles: auto, light, balanced")
		}
		c.Model = h.Model
		if e = veda.WriteJSON(filepath.Join(root, ".veda", "project.json"), c); e != nil {
			return e
		}
		printJSON(h)
		fmt.Println("Profile saved. Start a matching local model with scripts/start-model.sh.")
		return veda.WriteJSON(filepath.Join(root, ".veda", "model.json"), h)
	}
	s, e := veda.OpenStore(root)
	if e != nil {
		return e
	}
	defer s.DB.Close()
	switch command {
	case "serve":
		hub, err := veda.OpenHub(*knowledge, c)
		if err != nil {
			return err
		}
		defer hub.Close()
		server := veda.NewServer(root, s, c)
		server.Hub = hub
		return server.Serve(ctx, *address)
	case "research":
		objective := strings.Join(f.Args(), " ")
		var m veda.Model = veda.LocalModel{Endpoint: c.Endpoint, Name: c.Model}
		if *demo {
			m = &veda.DemoModel{}
		}
		engine := veda.Engine{Root: root, Store: s, Model: m, Runner: veda.DockerRunner{}, Log: func(msg string) { fmt.Println(msg) }}
		r, e := engine.Run(ctx, veda.Options{Objective: objective, Config: c, Demo: *demo})
		if r.ID != "" {
			fmt.Printf("Report: %s\n", filepath.Join(root, ".veda", "runs", r.ID, "report.md"))
		}
		return e
	case "runs":
		runs, e := s.Runs()
		if e != nil {
			return e
		}
		for _, r := range runs {
			fmt.Printf("%s  %-10s  %6.2f%%  %s\n", r.ID, r.Status, r.Improvement, r.Objective)
		}
		return nil
	case "report":
		if f.NArg() != 1 {
			return fmt.Errorf("usage: veda report --workspace PATH RUN-ID")
		}
		r, e := s.Get(f.Arg(0))
		if e == nil {
			fmt.Print(r.Report)
		}
		return e
	case "verify":
		if f.NArg() != 1 {
			return fmt.Errorf("usage: veda verify --workspace PATH RUN-ID")
		}
		if e = veda.VerifyRun(root, f.Arg(0)); e == nil {
			fmt.Println("All experiment and baseline evidence hashes match.")
		}
		return e
	case "reproduce":
		if f.NArg() != 2 {
			return fmt.Errorf("usage: veda reproduce --workspace PATH RUN-ID EXP-ID (or BASELINE)")
		}
		m, dir, e := veda.Reproduce(ctx, root, s, f.Arg(0), f.Arg(1))
		printJSON(m)
		fmt.Println("Reproduction evidence:", dir)
		return e
	default:
		return fmt.Errorf("unknown command %q; run veda help", command)
	}
}
func printJSON(v any) { b, _ := json.MarshalIndent(v, "", "  "); fmt.Println(string(b)) }
func help() {
	fmt.Print(`Veda · local autonomous experimentation

  veda init --workspace PATH
  veda doctor --workspace PATH
  veda model setup --workspace PATH --profile auto
  veda serve --workspace PATH [--addr 127.0.0.1:8787]
  veda research --workspace PATH --benchmark '^BenchmarkRender$' "Reduce allocation by 20%"
  veda research --workspace examples/allocdemo --demo "Reduce allocation by 20%"
  veda runs --workspace PATH
  veda report --workspace PATH RUN-ID
  veda verify --workspace PATH RUN-ID
  veda reproduce --workspace PATH RUN-ID EXP-ID

Place all flags before the objective or run identifiers. Docker and a local model
are required for research; --demo uses fixed proposals and real measurements.
No cloud AI, account, API key, or network-enabled experiments.
`)
}
