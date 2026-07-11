package documents

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

type Chunk struct {
	Path   string    `json:"path"`
	Text   string    `json:"text"`
	Vector []float64 `json:"vector"`
}
type Store struct {
	Chunks []Chunk `json:"chunks"`
}

func indexPath() (string, error) {
	base, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(base, "aish", "documents.json"), nil
}
func load() (Store, error) {
	p, err := indexPath()
	if err != nil {
		return Store{}, err
	}
	b, err := os.ReadFile(p)
	if os.IsNotExist(err) {
		return Store{}, nil
	}
	if err != nil {
		return Store{}, err
	}
	var s Store
	err = json.Unmarshal(b, &s)
	return s, err
}
func save(s Store) error {
	p, err := indexPath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(p), 0700); err != nil {
		return err
	}
	b, err := json.Marshal(s)
	if err != nil {
		return err
	}
	return os.WriteFile(p, b, 0600)
}
func embed(ctx context.Context, baseURL, model, text string) ([]float64, error) {
	payload, _ := json.Marshal(map[string]any{"model": model, "input": text})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(baseURL, "/")+"/api/embed", bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	req.Header.Set("content-type", "application/json")
	resp, err := (&http.Client{Timeout: 5 * time.Minute}).Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return nil, fmt.Errorf("embedding endpoint returned %s: %s", resp.Status, strings.TrimSpace(string(b)))
	}
	var out struct {
		Embeddings [][]float64 `json:"embeddings"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, err
	}
	if len(out.Embeddings) == 0 {
		return nil, fmt.Errorf("embedding endpoint returned no vector")
	}
	return out.Embeddings[0], nil
}
func readText(path string) ([]byte, error) {
	ext := strings.ToLower(filepath.Ext(path))
	if ext == ".docx" {
		r, err := zip.OpenReader(path)
		if err != nil {
			return nil, err
		}
		defer r.Close()
		var b strings.Builder
		for _, f := range r.File {
			if f.Name != "word/document.xml" {
				continue
			}
			rc, err := f.Open()
			if err != nil {
				return nil, err
			}
			data, _ := io.ReadAll(rc)
			rc.Close()
			re := regexp.MustCompile(`<[^>]+>`)
			b.WriteString(re.ReplaceAllString(string(data), " "))
		}
		return []byte(b.String()), nil
	}
	if ext == ".pdf" {
		if _, err := exec.LookPath("pdftotext"); err != nil {
			return nil, fmt.Errorf("PDF support requires pdftotext (poppler-utils) on PATH")
		}
		cmd := exec.Command("pdftotext", "-layout", path, "-")
		return cmd.Output()
	}
	return os.ReadFile(path)
}
func supported(ext string) bool {
	switch ext {
	case ".txt", ".md", ".json", ".csv", ".go", ".py", ".js", ".ts", ".rs", ".java", ".c", ".cpp", ".h", ".yaml", ".yml", ".toml", ".html", ".css", ".xml", ".log", ".docx", ".pdf":
		return true
	}
	return false
}
func split(text string) []string {
	const size = 1200
	const overlap = 200
	var out []string
	for start := 0; start < len(text); {
		end := start + size
		if end > len(text) {
			end = len(text)
		}
		out = append(out, text[start:end])
		if end == len(text) {
			break
		}
		start = end - overlap
	}
	return out
}
func Add(ctx context.Context, path, baseURL, model string) (int, error) {
	info, err := os.Stat(path)
	if err != nil {
		return 0, err
	}
	var files []string
	if info.IsDir() {
		err = filepath.WalkDir(path, func(p string, d os.DirEntry, e error) error {
			if e != nil {
				return e
			}
			if !d.IsDir() {
				ext := strings.ToLower(filepath.Ext(p))
				if supported(ext) {
					files = append(files, p)
				}
			}
			return nil
		})
	} else {
		files = []string{path}
	}
	if err != nil {
		return 0, err
	}
	s, err := load()
	if err != nil {
		return 0, err
	}
	count := 0
	fileSet := map[string]bool{}
	for _, f := range files {
		fileSet[f] = true
	}
	kept := s.Chunks[:0]
	for _, c := range s.Chunks {
		if !fileSet[c.Path] {
			kept = append(kept, c)
		}
	}
	s.Chunks = kept
	for _, f := range files {
		b, e := readText(f)
		if e != nil {
			continue
		}
		for _, text := range split(string(b)) {
			v, e := embed(ctx, baseURL, model, text)
			if e != nil {
				return count, e
			}
			s.Chunks = append(s.Chunks, Chunk{Path: f, Text: text, Vector: v})
			count++
		}
	}
	return count, save(s)
}
func cosine(a, b []float64) float64 {
	if len(a) != len(b) || len(a) == 0 {
		return -1
	}
	var dot, aa, bb float64
	for i := range a {
		dot += a[i] * b[i]
		aa += a[i] * a[i]
		bb += b[i] * b[i]
	}
	if aa == 0 || bb == 0 {
		return -1
	}
	return dot / (math.Sqrt(aa) * math.Sqrt(bb))
}
func Search(ctx context.Context, query, baseURL, model string, k int) ([]Chunk, error) {
	s, err := load()
	if err != nil {
		return nil, err
	}
	v, err := embed(ctx, baseURL, model, query)
	if err != nil {
		return nil, err
	}
	type scored struct {
		c Chunk
		s float64
	}
	items := make([]scored, 0, len(s.Chunks))
	for _, c := range s.Chunks {
		items = append(items, scored{c, cosine(v, c.Vector)})
	}
	sort.Slice(items, func(i, j int) bool { return items[i].s > items[j].s })
	if k <= 0 {
		k = 5
	}
	if k > len(items) {
		k = len(items)
	}
	out := make([]Chunk, k)
	for i := 0; i < k; i++ {
		out[i] = items[i].c
	}
	return out, nil
}
func List() (map[string]int, error) {
	s, err := load()
	if err != nil {
		return nil, err
	}
	m := map[string]int{}
	for _, c := range s.Chunks {
		m[c.Path]++
	}
	return m, nil
}
