package veda

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const PreviewImage = "veda-ui-preview:1"

type ChangePreview struct {
	Status         string            `json:"status"`
	Reason         string            `json:"reason"`
	RunID          string            `json:"run_id,omitempty"`
	Page           string            `json:"page,omitempty"`
	Width          int               `json:"width,omitempty"`
	Height         int               `json:"height,omitempty"`
	Captured       string            `json:"captured,omitempty"`
	ImageID        string            `json:"image_id,omitempty"`
	Hashes         map[string]string `json:"hashes,omitempty"`
	ChangedPercent float64           `json:"changed_percent"`
	Warnings       []string          `json:"warnings,omitempty"`
}
type browserCapture struct {
	PNG     string   `json:"png"`
	Missing []string `json:"missing"`
}

func previewPage(repo Repository, c SuggestedChange, requested string) (string, error) {
	if requested != "" {
		if _, ok := repo.Content[requested]; ok && strings.ToLower(filepath.Ext(requested)) == ".html" {
			return requested, nil
		}
		return "", errors.New("selected HTML page was not captured")
	}
	for _, f := range c.Files {
		if strings.ToLower(filepath.Ext(f.Path)) == ".html" {
			return f.Path, nil
		}
	}
	// Prefer an index page beside the changed file or its parent directories.
	for _, f := range c.Files {
		for dir := filepath.Dir(f.Path); ; dir = filepath.Dir(dir) {
			p := filepath.ToSlash(filepath.Join(dir, "index.html"))
			if _, ok := repo.Content[p]; ok {
				return p, nil
			}
			if dir == "." {
				break
			}
		}
	}
	pages := []string{}
	for p := range repo.Content {
		if strings.HasSuffix(strings.ToLower(p), ".html") {
			pages = append(pages, p)
		}
	}
	sort.Strings(pages)
	if len(pages) == 1 {
		return pages[0], nil
	}
	return "", errors.New("a static HTML entry page could not be selected; supply Preview page for a captured HTML page. Framework builds and backend startup are not supported yet")
}

func captureStaticPage(ctx context.Context, source, page, imageID string) ([]byte, []string, error) {
	if strings.Contains(source, ",") {
		return nil, nil, errors.New("preview source path cannot contain a comma")
	}
	ctx, cancel := context.WithTimeout(ctx, 40*time.Second)
	defer cancel()
	name := "veda-preview-" + strings.ToLower(strings.ReplaceAll(NewID(), ".", "-"))
	args := []string{"run", "--rm", "--pull=never", "--name", name, "--network=none", "--read-only", "--cap-drop=ALL", "--security-opt=no-new-privileges", "--pids-limit=256", "--memory=2g", "--memory-swap=2g", "--cpus=2", "--user=65534:65534", "--tmpfs=/tmp:rw,exec,nosuid,size=512m,mode=1777", "--mount", "type=bind,src=" + source + ",dst=/source,readonly", "-e", "HOME=/tmp", imageID, page}
	out, errout := &boundedBuffer{limit: 12 << 20}, &boundedBuffer{limit: 6000}
	cmd := exec.CommandContext(ctx, "docker", args...)
	cmd.Stdout = out
	cmd.Stderr = errout
	err := cmd.Run()
	if ctx.Err() != nil {
		clean, stop := context.WithTimeout(context.Background(), 8*time.Second)
		_ = exec.CommandContext(clean, "docker", "rm", "-f", name).Run()
		stop()
		err = ctx.Err()
	}
	if err != nil {
		return nil, nil, fmt.Errorf("isolated browser capture failed: %w: %.1000s", err, errout.String())
	}
	var captured browserCapture
	if err = json.Unmarshal(out.Bytes(), &captured); err != nil {
		return nil, nil, errors.New("browser did not return a complete capture")
	}
	pngBytes, err := base64.StdEncoding.DecodeString(captured.PNG)
	if err != nil {
		return nil, nil, err
	}
	cfg, err := png.DecodeConfig(bytes.NewReader(pngBytes))
	if err != nil || cfg.Width != 1280 || cfg.Height != 900 {
		return nil, nil, errors.New("browser capture dimensions do not match the fixed viewport")
	}
	return pngBytes, captured.Missing, nil
}

func imageDifference(before, after []byte) ([]byte, float64, error) {
	a, err := png.Decode(bytes.NewReader(before))
	if err != nil {
		return nil, 0, err
	}
	b, err := png.Decode(bytes.NewReader(after))
	if err != nil {
		return nil, 0, err
	}
	if a.Bounds() != b.Bounds() {
		return nil, 0, errors.New("capture dimensions differ")
	}
	bounds := a.Bounds()
	if bounds.Dx()*bounds.Dy() > 1280*900 {
		return nil, 0, errors.New("capture exceeds viewport limit")
	}
	diff := image.NewNRGBA(bounds)
	changed := 0
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			aa, bb := color.NRGBAModel.Convert(a.At(x, y)).(color.NRGBA), color.NRGBAModel.Convert(b.At(x, y)).(color.NRGBA)
			if aa != bb {
				changed++
				diff.SetNRGBA(x, y, color.NRGBA{R: 211, G: 45, B: 121, A: 255})
			} else {
				gray := uint8((int(bb.R)+int(bb.G)+int(bb.B))/6 + 128)
				diff.SetNRGBA(x, y, color.NRGBA{R: gray, G: gray, B: gray, A: 255})
			}
		}
	}
	var out bytes.Buffer
	err = png.Encode(&out, diff)
	return out.Bytes(), 100 * float64(changed) / float64(bounds.Dx()*bounds.Dy()), err
}

func (h *Hub) captureChange(ctx context.Context, r Investigation, c SuggestedChange) *ChangePreview {
	p := &ChangePreview{Status: "unavailable", RunID: r.ID, Width: 1280, Height: 900, ImageID: r.Images["UI preview"]}
	page, err := previewPage(r.Repository, c, r.Request.PreviewPage)
	if err != nil {
		p.Reason = err.Error()
		return p
	}
	p.Page = page
	if p.ImageID == "" || p.ImageID == "unavailable" {
		p.Reason = "Screenshot runtime unavailable. Run scripts/setup-preview.sh in the Veda project, then refresh this investigation."
		return p
	}
	tmp, err := os.MkdirTemp("", "veda-preview-source-")
	if err != nil {
		p.Reason = err.Error()
		return p
	}
	defer os.RemoveAll(tmp)
	// Copy only bounded, captured files. The container has no host-writable mounts.
	for path, body := range r.Repository.Content {
		if !allowedRepoFile(path) {
			continue
		}
		target := filepath.Join(tmp, path)
		if err = os.MkdirAll(filepath.Dir(target), 0755); err == nil {
			err = os.WriteFile(target, []byte(body), 0644)
		}
		if err != nil {
			p.Reason = err.Error()
			return p
		}
	}
	// Docker's unprivileged browser must be able to traverse the temporary root.
	if err = os.Chmod(tmp, 0755); err != nil {
		p.Reason = err.Error()
		return p
	}
	before, missing, err := captureStaticPage(ctx, tmp, page, p.ImageID)
	if err != nil {
		p.Reason = err.Error()
		return p
	}
	p.Warnings = append(p.Warnings, missing...)
	for _, f := range c.Files {
		if err = os.WriteFile(filepath.Join(tmp, f.Path), []byte(f.After), 0644); err != nil {
			p.Reason = err.Error()
			return p
		}
	}
	after, missing, err := captureStaticPage(ctx, tmp, page, p.ImageID)
	if err != nil {
		p.Reason = err.Error()
		return p
	}
	p.Warnings = append(p.Warnings, missing...)
	diff, percent, err := imageDifference(before, after)
	if err != nil {
		p.Reason = err.Error()
		return p
	}
	dir := filepath.Join(h.Root, "investigations", r.ID, "changes", c.ID)
	if err = os.MkdirAll(dir, 0700); err != nil {
		p.Reason = err.Error()
		return p
	}
	p.Hashes = map[string]string{}
	for name, data := range map[string][]byte{"before": before, "after": after, "difference": diff} {
		if err = os.WriteFile(filepath.Join(dir, name+".png"), data, 0600); err != nil {
			p.Reason = err.Error()
			return p
		}
		p.Hashes[name] = digest(string(data))
	}
	p.Status = "captured"
	p.Captured = now()
	p.ChangedPercent = percent
	p.Reason = "Captured the original and this independent suggestion at the same page and 1280 × 900 viewport. Static preview only; screenshots do not prove functional correctness."
	warnings := map[string]bool{}
	for _, warning := range p.Warnings {
		warnings[warning] = true
	}
	p.Warnings = SortedKeys(warnings)
	if len(p.Warnings) > 0 {
		p.Status = "partial"
		p.Reason = "Before/after captures are available, but some assets or services were unavailable. This is a partial static preview."
	}
	return p
}
