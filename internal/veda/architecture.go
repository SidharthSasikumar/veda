package veda

import (
	"encoding/json"
	"fmt"
	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"gopkg.in/yaml.v3"
	"path/filepath"
	"sort"
	"strings"
)

type GraphNode struct {
	ID     string `json:"id"`
	Label  string `json:"label"`
	Kind   string `json:"kind"`
	Path   string `json:"path,omitempty"`
	Line   int    `json:"line,omitempty"`
	Detail string `json:"detail,omitempty"`
}
type GraphEdge struct {
	ID     string `json:"id"`
	Source string `json:"source"`
	Target string `json:"target"`
	Label  string `json:"label"`
	Path   string `json:"path,omitempty"`
	Line   int    `json:"line,omitempty"`
	Basis  string `json:"basis"`
}
type Graph struct {
	Nodes    []GraphNode `json:"nodes"`
	Edges    []GraphEdge `json:"edges"`
	Warnings []string    `json:"warnings"`
}
type graphBuilder struct {
	graph Graph
	nodes map[string]bool
	edges map[string]bool
}

func newGraph() *graphBuilder {
	return &graphBuilder{Graph{[]GraphNode{}, []GraphEdge{}, []string{}}, map[string]bool{}, map[string]bool{}}
}
func (b *graphBuilder) node(n GraphNode) {
	if !b.nodes[n.ID] && len(b.graph.Nodes) < 1800 {
		b.nodes[n.ID] = true
		b.graph.Nodes = append(b.graph.Nodes, n)
	}
}
func (b *graphBuilder) edge(from, to, label, path string, line int) {
	key := digest(from + "|" + to + "|" + label)[:20]
	if !b.edges[key] {
		b.edges[key] = true
		b.graph.Edges = append(b.graph.Edges, GraphEdge{key, from, to, label, path, line, "source declaration"})
	}
}
func (b *graphBuilder) result() Graph {
	edges := []GraphEdge{}
	for _, e := range b.graph.Edges {
		if b.nodes[e.Source] && b.nodes[e.Target] {
			edges = append(edges, e)
		}
	}
	b.graph.Edges = edges
	return b.graph
}
func sourceLine(text, needle string) int {
	for i, s := range strings.Split(text, "\n") {
		if strings.Contains(s, needle) {
			return i + 1
		}
	}
	return 1
}
func Architecture(repo Repository) Graph {
	b := newGraph()
	b.node(GraphNode{ID: "repository", Label: repo.Name, Kind: "repository", Detail: "Declared source architecture · commit " + repo.Commit})
	for _, file := range repo.Files {
		path, text := file.Path, repo.Content[file.Path]
		name := strings.ToLower(filepath.Base(path))
		dir := filepath.ToSlash(filepath.Dir(path))
		switch {
		case strings.HasSuffix(name, ".tf"):
			b.terraform(path, text, dir)
		case name == "compose.yml" || name == "compose.yaml" || strings.HasPrefix(name, "docker-compose") && (strings.HasSuffix(name, ".yml") || strings.HasSuffix(name, ".yaml")):
			b.compose(path, text, dir)
		case name == "dockerfile" || strings.HasPrefix(name, "dockerfile."):
			id := "dockerfile:" + path
			b.node(GraphNode{id, filepath.Base(path), "dockerfile", path, 1, "Image build definition; not a running container"})
			b.edge("repository", id, "builds", path, 1)
			for i, line := range strings.Split(text, "\n") {
				f := strings.Fields(line)
				if len(f) < 2 {
					continue
				}
				if strings.EqualFold(f[0], "FROM") {
					j := 1
					for j < len(f) && strings.HasPrefix(f[j], "--") {
						j++
					}
					if j < len(f) {
						img := "image:" + f[j]
						b.node(GraphNode{ID: img, Label: f[j], Kind: "image", Path: path, Line: i + 1})
						b.edge(id, img, "base image", path, i+1)
					}
				}
			}
		case name == "go.mod" || name == "package.json" || name == "pyproject.toml" || name == "requirements.txt":
			id := "component:" + dir
			label := dir
			if dir == "." {
				label = repo.Name
			}
			detail := name
			if name == "package.json" {
				var v struct {
					Name         string            `json:"name"`
					Dependencies map[string]string `json:"dependencies"`
				}
				if json.Unmarshal([]byte(text), &v) == nil {
					if v.Name != "" {
						label = v.Name
					}
					detail = fmt.Sprintf("Node package · %d dependencies", len(v.Dependencies))
				}
			}
			if name == "go.mod" {
				for _, line := range strings.Split(text, "\n") {
					if strings.HasPrefix(strings.TrimSpace(line), "module ") {
						label = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(line), "module "))
						break
					}
				}
				detail = "Go module"
			}
			b.node(GraphNode{id, label, "component", path, 1, detail})
			b.edge("repository", id, "contains", path, 1)
		case strings.HasSuffix(name, ".yml") || strings.HasSuffix(name, ".yaml"):
			// Kubernetes manifests: show resource identity, never secret payloads.
			dec := yaml.NewDecoder(strings.NewReader(text))
			for i := 0; i < 100; i++ {
				var v struct {
					Kind       string `yaml:"kind"`
					APIVersion string `yaml:"apiVersion"`
					Metadata   struct {
						Name string `yaml:"name"`
					} `yaml:"metadata"`
				}
				if dec.Decode(&v) != nil {
					break
				}
				if v.Kind != "" && v.APIVersion != "" && v.Metadata.Name != "" {
					id := "kube:" + path + ":" + v.Kind + ":" + v.Metadata.Name
					b.node(GraphNode{id, v.Kind + " / " + v.Metadata.Name, "kubernetes", path, sourceLine(text, "kind:"), "Declared Kubernetes resource"})
					b.edge("repository", id, "declares", path, 1)
				}
			}
		}
	}
	// Connect image definitions to co-located application components when the path establishes that relationship.
	for _, n := range b.graph.Nodes {
		if n.Kind == "dockerfile" {
			id := "component:" + filepath.ToSlash(filepath.Dir(n.Path))
			if b.nodes[id] {
				b.edge(id, n.ID, "build definition", n.Path, 1)
			}
		}
	}
	if len(b.graph.Nodes) == 1 {
		b.graph.Warnings = append(b.graph.Warnings, "No supported infrastructure or package manifests found. No system components have been invented.")
	}
	if len(b.graph.Nodes) >= 1800 {
		b.graph.Warnings = append(b.graph.Warnings, "Graph capped at 1,800 components.")
	}
	return b.result()
}
func (b *graphBuilder) terraform(path, text, dir string) {
	file, diags := hclsyntax.ParseConfig([]byte(text), path, hcl.InitialPos)
	if diags.HasErrors() {
		b.graph.Warnings = append(b.graph.Warnings, path+": invalid HCL; architecture extraction incomplete")
		return
	}
	body := file.Body.(*hclsyntax.Body)
	prefix := "tf:" + dir + ":"
	for _, block := range body.Blocks {
		var key, label, kind string
		switch block.Type {
		case "resource", "data":
			if len(block.Labels) != 2 {
				continue
			}
			key = block.Type + "." + strings.Join(block.Labels, ".")
			label = block.Labels[1]
			kind = "terraform"
		case "module":
			if len(block.Labels) != 1 {
				continue
			}
			key = "module." + block.Labels[0]
			label = block.Labels[0]
			kind = "module"
		case "variable":
			if len(block.Labels) != 1 {
				continue
			}
			key = "var." + block.Labels[0]
			label = block.Labels[0]
			kind = "variable"
		default:
			continue
		}
		id := prefix + key
		b.node(GraphNode{id, label, kind, path, block.TypeRange.Start.Line, key})
		b.edge("repository", id, "declares", path, block.TypeRange.Start.Line)
		var visit func(*hclsyntax.Body)
		visit = func(inner *hclsyntax.Body) {
			keys := make([]string, 0, len(inner.Attributes))
			for name := range inner.Attributes {
				keys = append(keys, name)
			}
			sort.Strings(keys)
			for _, name := range keys {
				attr := inner.Attributes[name]
				for _, tr := range attr.Expr.Variables() {
					parts := []string{}
					for _, step := range tr {
						switch t := step.(type) {
						case hcl.TraverseRoot:
							parts = append(parts, t.Name)
						case hcl.TraverseAttr:
							parts = append(parts, t.Name)
						}
					}
					if len(parts) < 2 {
						continue
					}
					key := "resource." + strings.Join(parts[:2], ".")
					if parts[0] == "data" && len(parts) >= 3 {
						key = strings.Join(parts[:3], ".")
					} else if parts[0] == "module" || parts[0] == "var" {
						key = strings.Join(parts[:2], ".")
					}
					b.edge(id, prefix+key, "references", path, attr.Range().Start.Line)
				}
			}
			for _, nested := range inner.Blocks {
				visit(nested.Body)
			}
		}
		visit(block.Body)
	}
}
func names(v any) []string {
	out := []string{}
	switch x := v.(type) {
	case []any:
		for _, v := range x {
			if s, ok := v.(string); ok {
				out = append(out, s)
			}
		}
	case map[string]any:
		for k := range x {
			out = append(out, k)
		}
	}
	sort.Strings(out)
	return out
}
func (b *graphBuilder) compose(path, text, dir string) {
	var v struct {
		Services map[string]map[string]any `yaml:"services"`
		Networks map[string]any            `yaml:"networks"`
		Volumes  map[string]any            `yaml:"volumes"`
	}
	if yaml.Unmarshal([]byte(text), &v) != nil {
		b.graph.Warnings = append(b.graph.Warnings, path+": invalid Compose YAML")
		return
	}
	prefix := "compose:" + path + ":"
	keys := make([]string, 0, len(v.Services))
	for k := range v.Services {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, name := range keys {
		service := v.Services[name]
		id := prefix + name
		line := sourceLine(text, name+":")
		b.node(GraphNode{id, name, "container", path, line, "Compose service declaration; runtime state is not inspected"})
		b.edge("repository", id, "contains", path, line)
		if img, ok := service["image"].(string); ok {
			imageID := "image:" + img
			b.node(GraphNode{ID: imageID, Label: img, Kind: "image", Path: path, Line: line})
			b.edge(id, imageID, "uses image", path, line)
		}
		for _, dep := range names(service["depends_on"]) {
			b.edge(id, prefix+dep, "depends on", path, line)
		}
		for _, net := range names(service["networks"]) {
			nid := prefix + "network:" + net
			b.node(GraphNode{ID: nid, Label: net, Kind: "network", Path: path, Line: line})
			b.edge(id, nid, "connects", path, line)
		}
		if vol, ok := service["volumes"].([]any); ok {
			for _, mount := range vol {
				source := ""
				switch m := mount.(type) {
				case string:
					source = strings.Split(m, ":")[0]
				case map[string]any:
					source, _ = m["source"].(string)
				}
				if _, exists := v.Volumes[source]; exists {
					vid := prefix + "volume:" + source
					b.node(GraphNode{ID: vid, Label: source, Kind: "volume", Path: path, Line: line})
					b.edge(id, vid, "mounts", path, line)
				}
			}
		}
		dockerfile := ""
		switch build := service["build"].(type) {
		case string:
			dockerfile = filepath.Join(dir, build, "Dockerfile")
		case map[string]any:
			context, _ := build["context"].(string)
			file, _ := build["dockerfile"].(string)
			if file == "" {
				file = "Dockerfile"
			}
			dockerfile = filepath.Join(dir, context, file)
		}
		if dockerfile != "" {
			b.edge(id, "dockerfile:"+filepath.ToSlash(dockerfile), "built from", path, line)
		}
	}
}
