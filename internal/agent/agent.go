package agent

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/khashino/AISH/internal/securestore"
)

type Step struct {
	Title    string `json:"title"`
	Command  string `json:"command"`
	Status   string `json:"status"`
	Output   string `json:"output,omitempty"`
	Error    string `json:"error,omitempty"`
	Attempts int    `json:"attempts,omitempty"`
}

type Task struct {
	ID       string    `json:"id"`
	Goal     string    `json:"goal"`
	Created  time.Time `json:"created"`
	Updated  time.Time `json:"updated"`
	Workdir  string    `json:"workdir"`
	Provider string    `json:"provider"`
	Current  int       `json:"current"`
	Status   string    `json:"status"`
	Steps    []Step    `json:"steps"`
}

func dir() (string, error) {
	b, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(b, "aish", "agents"), nil
}
func cleanID(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	var b strings.Builder
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' {
			b.WriteRune(r)
		} else if r == ' ' || r == '_' {
			b.WriteByte('-')
		}
	}
	out := strings.Trim(b.String(), "-")
	if out == "" {
		out = time.Now().Format("20060102-150405")
	}
	return out
}
func New(goal, workdir, provider string, steps []Step) Task {
	id := cleanID(goal)
	if len(id) > 40 {
		id = id[:40]
	}
	return Task{ID: id, Goal: goal, Created: time.Now(), Updated: time.Now(), Workdir: workdir, Provider: provider, Status: "paused", Steps: steps}
}
func Save(t Task) error {
	d, err := dir()
	if err != nil {
		return err
	}
	if err = os.MkdirAll(d, 0700); err != nil {
		return err
	}
	t.Updated = time.Now()
	b, err := json.MarshalIndent(t, "", "  ")
	if err != nil {
		return err
	}
	b, err = securestore.Encrypt(b)
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(d, t.ID+".json"), b, 0600)
}
func Load(id string) (Task, error) {
	d, err := dir()
	if err != nil {
		return Task{}, err
	}
	b, err := os.ReadFile(filepath.Join(d, cleanID(id)+".json"))
	if err != nil {
		return Task{}, err
	}
	b, err = securestore.Decrypt(b)
	if err != nil {
		return Task{}, err
	}
	var t Task
	err = json.Unmarshal(b, &t)
	return t, err
}
func List() ([]Task, error) {
	d, err := dir()
	if err != nil {
		return nil, err
	}
	es, err := os.ReadDir(d)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	out := []Task{}
	for _, e := range es {
		if e.IsDir() || filepath.Ext(e.Name()) != ".json" {
			continue
		}
		t, er := Load(strings.TrimSuffix(e.Name(), ".json"))
		if er == nil {
			out = append(out, t)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Updated.After(out[j].Updated) })
	return out, nil
}
func Delete(id string) error {
	d, err := dir()
	if err != nil {
		return err
	}
	err = os.Remove(filepath.Join(d, cleanID(id)+".json"))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}
