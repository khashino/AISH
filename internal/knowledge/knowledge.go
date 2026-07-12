package knowledge

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
	"time"

	"github.com/khashino/AISH/internal/documents"
	"github.com/khashino/AISH/internal/securestore"
)

type Collection struct {
	Name    string    `json:"name"`
	Paths   []string  `json:"paths"`
	Updated time.Time `json:"updated"`
}
type Registry struct {
	Active      string                `json:"active"`
	Collections map[string]Collection `json:"collections"`
}

func regPath() (string, error) {
	b, e := os.UserConfigDir()
	if e != nil {
		return "", e
	}
	return filepath.Join(b, "aish", "knowledge.json"), nil
}
func Load() (Registry, error) {
	p, e := regPath()
	if e != nil {
		return Registry{}, e
	}
	b, e := os.ReadFile(p)
	if errors.Is(e, os.ErrNotExist) {
		return Registry{Active: "personal", Collections: map[string]Collection{"personal": {Name: "personal"}}}, nil
	}
	if e != nil {
		return Registry{}, e
	}
	b, e = securestore.Decrypt(b)
	if e != nil {
		return Registry{}, e
	}
	var r Registry
	e = json.Unmarshal(b, &r)
	if r.Collections == nil {
		r.Collections = map[string]Collection{}
	}
	return r, e
}
func Save(r Registry) error {
	p, e := regPath()
	if e != nil {
		return e
	}
	if e = os.MkdirAll(filepath.Dir(p), 0700); e != nil {
		return e
	}
	b, e := json.MarshalIndent(r, "", "  ")
	if e != nil {
		return e
	}
	b, e = securestore.Encrypt(b)
	if e != nil {
		return e
	}
	return os.WriteFile(p, b, 0600)
}
func Create(name string) error {
	r, e := Load()
	if e != nil {
		return e
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return fmt.Errorf("collection name required")
	}
	if _, ok := r.Collections[name]; ok {
		return fmt.Errorf("collection %s already exists", name)
	}
	r.Collections[name] = Collection{Name: name, Updated: time.Now()}
	if r.Active == "" {
		r.Active = name
	}
	return Save(r)
}
func Use(name string) error {
	r, e := Load()
	if e != nil {
		return e
	}
	if _, ok := r.Collections[name]; !ok {
		return fmt.Errorf("unknown collection %s", name)
	}
	r.Active = name
	return Save(r)
}
func Delete(name string) error {
	r, e := Load()
	if e != nil {
		return e
	}
	if _, ok := r.Collections[name]; !ok {
		return nil
	}
	_, _, _ = documents.ClearCollection(name)
	delete(r.Collections, name)
	if r.Active == name {
		r.Active = ""
		for n := range r.Collections {
			r.Active = n
			break
		}
	}
	return Save(r)
}
func List() ([]Collection, string, error) {
	r, e := Load()
	if e != nil {
		return nil, "", e
	}
	out := make([]Collection, 0, len(r.Collections))
	for _, c := range r.Collections {
		out = append(out, c)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, r.Active, nil
}
func AddPath(name, path, baseURL, model string) (int, error) {
	r, e := Load()
	if e != nil {
		return 0, e
	}
	c, ok := r.Collections[name]
	if !ok {
		return 0, fmt.Errorf("unknown collection %s", name)
	}
	abs, _ := filepath.Abs(path)
	found := false
	for _, p := range c.Paths {
		if p == abs {
			found = true
		}
	}
	if !found {
		c.Paths = append(c.Paths, abs)
	}
	c.Updated = time.Now()
	r.Collections[name] = c
	if e = Save(r); e != nil {
		return 0, e
	}
	return documents.AddTo(context.Background(), name, path, baseURL, model)
}
func Watch(name, path, baseURL, model string, interval time.Duration) error {
	if interval <= 0 {
		interval = 5 * time.Second
	}
	fmt.Printf("watching %s for collection %s; Ctrl+C to stop\n", path, name)
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	last := map[string]time.Time{}
	scan := func() error {
		return filepath.WalkDir(path, func(p string, d os.DirEntry, e error) error {
			if e != nil {
				return e
			}
			if d.IsDir() {
				return nil
			}
			st, e := d.Info()
			if e != nil {
				return e
			}
			if t, ok := last[p]; !ok || st.ModTime().After(t) {
				if _, e := documents.AddTo(context.Background(), name, p, baseURL, model); e != nil {
					return e
				}
				last[p] = st.ModTime()
				fmt.Println("indexed", p)
			}
			return nil
		})
	}
	if e := scan(); e != nil {
		return e
	}
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-t.C:
			if e := scan(); e != nil {
				fmt.Println("watch error:", e)
			}
		}
	}
}
