package veda

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"go/format"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

var excluded = map[string]bool{".git": true, ".veda": true, "node_modules": true, ".venv": true, "vendor": false, "models": true, "bin": true}

func Snapshot(src, dst string) error {
	var size int64
	count := 0
	return filepath.WalkDir(src, func(p string, d fs.DirEntry, e error) error {
		if e != nil {
			return e
		}
		rel, e := filepath.Rel(src, p)
		if e != nil {
			return e
		}
		if rel == "." {
			return os.MkdirAll(dst, 0755)
		}
		if d.IsDir() {
			if excluded[d.Name()] || strings.HasPrefix(d.Name(), ".") {
				return filepath.SkipDir
			}
			return os.MkdirAll(filepath.Join(dst, rel), 0755)
		}
		if d.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("symlink is not allowed in research snapshot: %s", rel)
		}
		if strings.HasPrefix(d.Name(), ".") || strings.HasSuffix(d.Name(), ".pem") || strings.HasSuffix(d.Name(), ".key") {
			return nil
		}
		info, e := d.Info()
		if e != nil {
			return e
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("not a regular file: %s", rel)
		}
		size += info.Size()
		count++
		if size > 32<<20 || count > 3000 {
			return errors.New("prototype snapshot limit: 32 MiB / 3000 files; use a smaller standalone Go module")
		}
		b, e := os.ReadFile(p)
		if e != nil {
			return e
		}
		return os.WriteFile(filepath.Join(dst, rel), b, 0644)
	})
}
func SourceContext(root string) (string, error) {
	var b strings.Builder
	count := 0
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, e error) error {
		if e != nil {
			return e
		}
		if d.IsDir() {
			if d.Name() == "vendor" {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(p, ".go") && d.Name() != "go.mod" {
			return nil
		}
		rel, _ := filepath.Rel(root, p)
		raw, e := os.ReadFile(p)
		if e != nil {
			return e
		}
		count++
		if count > 30 || b.Len()+len(raw) > 18000 {
			return errors.New("source exceeds prototype context budget (30 files / 18,000 bytes); select a smaller Go module")
		}
		fmt.Fprintf(&b, "\n--- FILE %s ---\n%s\n", rel, raw)
		return nil
	})
	return b.String(), err
}
func ApplyEdits(root string, edits []Edit) error {
	resolvedRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		return err
	}
	root = resolvedRoot
	if len(edits) == 0 || len(edits) > 4 {
		return errors.New("candidate must change 1–4 existing Go source files")
	}
	seen := map[string]bool{}
	formatted := map[string][]byte{}
	for _, edit := range edits {
		p := filepath.Clean(edit.Path)
		if p != edit.Path || filepath.IsAbs(p) || p == ".." || strings.HasPrefix(p, ".."+string(os.PathSeparator)) || strings.Contains(p, "\\") || strings.HasPrefix(p, ".") || strings.HasPrefix(p, "vendor/") || !strings.HasSuffix(p, ".go") || strings.HasSuffix(p, "_test.go") {
			return fmt.Errorf("disallowed edit: %s", edit.Path)
		}
		if seen[p] {
			return errors.New("duplicate edit path")
		}
		seen[p] = true
		full := filepath.Join(root, p)
		info, e := os.Lstat(full)
		if e != nil || !info.Mode().IsRegular() {
			return fmt.Errorf("only existing regular files may be edited: %s", p)
		}
		resolved, e := filepath.EvalSymlinks(full)
		if e != nil || resolved != full {
			return fmt.Errorf("symlink edit refused: %s", p)
		}
		if len(edit.Content) > 64000 {
			return errors.New("candidate file exceeds 64 KB")
		}
		content, e := format.Source([]byte(edit.Content))
		if e != nil {
			return fmt.Errorf("invalid Go source in %s: %w", p, e)
		}
		formatted[full] = content
	}
	for p, b := range formatted {
		if e := os.WriteFile(p, b, 0644); e != nil {
			return e
		}
	}
	return nil
}
func HashFiles(root string) (map[string]string, error) {
	out := map[string]string{}
	e := filepath.WalkDir(root, func(p string, d fs.DirEntry, e error) error {
		if e != nil {
			return e
		}
		if d.IsDir() {
			return nil
		}
		if d.Type()&os.ModeSymlink != 0 {
			return errors.New("evidence contains symlink")
		}
		rel, _ := filepath.Rel(root, p)
		if rel == "manifest.json" {
			return nil
		}
		b, e := os.ReadFile(p)
		if e != nil {
			return e
		}
		h := sha256.Sum256(b)
		out[rel] = hex.EncodeToString(h[:])
		return nil
	})
	return out, e
}
func Seal(root string) error {
	hashes, e := HashFiles(root)
	if e != nil {
		return e
	}
	return WriteJSON(filepath.Join(root, "manifest.json"), hashes)
}
func Verify(root string) error {
	b, e := os.ReadFile(filepath.Join(root, "manifest.json"))
	if e != nil {
		return e
	}
	var expected map[string]string
	if e = json.Unmarshal(b, &expected); e != nil {
		return e
	}
	actual, e := HashFiles(root)
	if e != nil {
		return e
	}
	if len(expected) != len(actual) {
		return errors.New("evidence file count changed")
	}
	for p, h := range expected {
		if actual[p] != h {
			return fmt.Errorf("evidence hash mismatch: %s", p)
		}
	}
	return nil
}
func SortedKeys[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
