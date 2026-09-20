package veda

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
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

const AnalyzerVersion = "repository-v2.3"
const maxRepoFiles = 1200
const maxRepoBytes = 24 << 20

type RepoFile struct {
	Path     string `json:"path"`
	Size     int64  `json:"size"`
	Language string `json:"language"`
	SHA256   string `json:"sha256"`
}
type Repository struct {
	URL      string            `json:"url"`
	Name     string            `json:"name"`
	Commit   string            `json:"commit"`
	Ref      string            `json:"ref"`
	Files    []RepoFile        `json:"files"`
	Skipped  int               `json:"skipped"`
	Bytes    int64             `json:"bytes"`
	Warnings []string          `json:"warnings"`
	Root     string            `json:"-"`
	Content  map[string]string `json:"-"`
}
type RepoLoader interface {
	Load(context.Context, string, string, string) (Repository, error)
}
type GitLoader struct{}

var githubRepo = regexp.MustCompile(`^https://github\.com/([A-Za-z0-9][A-Za-z0-9-]{0,38})/([A-Za-z0-9_.-]{1,100})/?$`)
var githubRef = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._/-]{0,160}$`)

func CanonicalRepository(raw string) (string, string, error) {
	raw = strings.TrimSpace(raw)
	if strings.HasPrefix(raw, "git@github.com:") {
		raw = "https://github.com/" + strings.TrimPrefix(raw, "git@github.com:")
	}
	raw = strings.TrimSuffix(strings.TrimSuffix(raw, "/"), ".git")
	m := githubRepo.FindStringSubmatch(raw)
	if m == nil || m[2] == "." || m[2] == ".." {
		return "", "", errors.New("enter a GitHub repository URL such as https://github.com/owner/repository")
	}
	name := strings.ToLower(m[1] + "/" + m[2])
	return "https://github.com/" + name, name, nil
}
func ValidateRef(ref string) error {
	if ref != "" && (!githubRef.MatchString(ref) || strings.Contains(ref, "..") || strings.HasSuffix(ref, "/") || strings.HasSuffix(ref, ".lock")) {
		return errors.New("invalid branch or tag")
	}
	return nil
}
func digest(value string) string { b := sha256.Sum256([]byte(value)); return hex.EncodeToString(b[:]) }
func gitEnv() []string {
	return append(os.Environ(), "GIT_TERMINAL_PROMPT=0", "GIT_LFS_SKIP_SMUDGE=1", "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL=/dev/null")
}
func gitCommand(ctx context.Context, dir string, args ...string) (string, error) {
	fixed := []string{"-c", "core.hooksPath=/dev/null", "-c", "core.fsmonitor=false", "-c", "protocol.file.allow=never", "-c", "protocol.ext.allow=never", "-c", "credential.helper=", "-c", "credential.helper=!gh auth git-credential"}
	cmd := exec.CommandContext(ctx, "git", append(fixed, args...)...)
	cmd.Dir = dir
	cmd.Env = gitEnv()
	out := &boundedBuffer{limit: 8 << 20}
	errout := &boundedBuffer{limit: 4000}
	cmd.Stdout = out
	cmd.Stderr = errout
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("git operation failed: %w: %s", err, strings.TrimSpace(errout.String()))
	}
	if out.Len() == out.limit {
		return "", errors.New("Git response exceeded repository size limit")
	}
	return out.String(), nil
}
func fileLanguage(p string) string {
	name := strings.ToLower(filepath.Base(p))
	ext := strings.ToLower(filepath.Ext(p))
	if name == "dockerfile" || strings.HasPrefix(name, "dockerfile.") {
		return "Docker"
	}
	switch ext {
	case ".go":
		return "Go"
	case ".py":
		return "Python"
	case ".js", ".cjs", ".mjs":
		return "JavaScript"
	case ".ts", ".tsx", ".jsx":
		return "TypeScript / JSX"
	case ".tf":
		return "Terraform"
	case ".yml", ".yaml":
		return "YAML"
	case ".json":
		return "JSON"
	case ".md":
		return "Documentation"
	case ".sh":
		return "Shell"
	case ".html", ".css":
		return "Web"
	}
	if name == "go.mod" || name == "go.sum" || name == "pyproject.toml" || name == "requirements.txt" || name == "makefile" {
		return "Manifest"
	}
	return ""
}
func allowedRepoFile(p string) bool {
	if !filepath.IsLocal(p) || strings.ContainsAny(p, "\\\x00\n\r") {
		return false
	}
	for _, part := range strings.Split(p, "/") {
		if strings.HasPrefix(part, ".") && part != ".github" {
			return false
		}
		switch part {
		case "node_modules", "vendor", "dist", "build", "models", "__pycache__":
			return false
		}
	}
	n := strings.ToLower(filepath.Base(p))
	if strings.Contains(n, "tfstate") || strings.Contains(n, "tfvars") || strings.Contains(n, "credentials") || strings.HasSuffix(n, ".min.js") || strings.HasSuffix(n, ".lock") {
		return false
	}
	return fileLanguage(p) != ""
}
func (GitLoader) Load(ctx context.Context, url, ref, dest string) (Repository, error) {
	var repo Repository
	canonical, name, err := CanonicalRepository(url)
	if err != nil {
		return repo, err
	}
	if err = ValidateRef(ref); err != nil {
		return repo, err
	}
	ctx, cancel := context.WithTimeout(ctx, 3*time.Minute)
	defer cancel()
	gitdir := filepath.Join(dest, "checkout")
	root := filepath.Join(dest, "source")
	if err = os.MkdirAll(dest, 0700); err != nil {
		return repo, err
	}
	args := []string{"clone", "--depth=1", "--no-checkout", "--no-tags", "--filter=blob:limit=128k"}
	if ref != "" {
		args = append(args, "--branch", ref)
	}
	args = append(args, "--", canonical+".git", gitdir)
	if _, err = gitCommand(ctx, "", args...); err != nil {
		return repo, err
	}
	sha, err := gitCommand(ctx, gitdir, "rev-parse", "HEAD")
	if err != nil {
		return repo, err
	}
	branch, _ := gitCommand(ctx, gitdir, "symbolic-ref", "--short", "HEAD")
	listing, err := gitCommand(ctx, gitdir, "ls-tree", "-r", "-l", "-z", "HEAD")
	if err != nil {
		return repo, err
	}
	repo = Repository{URL: canonical, Name: name, Commit: strings.TrimSpace(sha), Ref: strings.TrimSpace(branch), Root: root, Files: []RepoFile{}, Warnings: []string{}, Content: map[string]string{}}
	entries := strings.Split(listing, "\x00")
	if len(entries) > 25000 {
		return repo, errors.New("repository exceeds 25,000 tree entries; investigate a smaller repository")
	}
	// The private investigation parent remains 0700. The mounted source must be
	// readable by the unprivileged container user.
	if err = os.MkdirAll(root, 0755); err != nil {
		return repo, err
	}
	// Only materialize bounded regular source blobs. Git never checks out hooks, links or submodules.
	sort.Slice(entries, func(i, j int) bool { return entries[i] < entries[j] })
	for _, entry := range entries {
		if err = ctx.Err(); err != nil {
			return repo, err
		}
		if entry == "" {
			continue
		}
		head, path, ok := strings.Cut(entry, "\t")
		parts := strings.Fields(head)
		if !ok || len(parts) != 4 {
			repo.Skipped++
			continue
		}
		size, e := strconv.ParseInt(parts[3], 10, 64)
		if e != nil || parts[1] != "blob" || (parts[0] != "100644" && parts[0] != "100755") || !allowedRepoFile(path) || size > 128<<10 || len(repo.Files) >= maxRepoFiles || repo.Bytes+size > maxRepoBytes {
			repo.Skipped++
			continue
		}
		content, e := gitCommand(ctx, gitdir, "cat-file", "blob", parts[2])
		if e != nil {
			return repo, e
		}
		if strings.ContainsRune(content, 0) {
			repo.Skipped++
			continue
		}
		target := filepath.Join(root, path)
		if e = os.MkdirAll(filepath.Dir(target), 0755); e != nil {
			return repo, e
		}
		if e = os.WriteFile(target, []byte(content), 0644); e != nil {
			return repo, e
		}
		repo.Files = append(repo.Files, RepoFile{path, size, fileLanguage(path), digest(content)})
		repo.Bytes += size
		repo.Content[path] = content
	}
	if repo.Skipped > 0 {
		repo.Warnings = append(repo.Warnings, fmt.Sprintf("%d entries excluded (common secret filenames, binaries, links, dependencies, generated files or size limits). Runtime checks apply only to the captured source.", repo.Skipped))
	}
	sort.Slice(repo.Files, func(i, j int) bool { return repo.Files[i].Path < repo.Files[j].Path })
	return repo, nil
}
