package history

import (
	"bufio"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/khashino/AISH/internal/securestore"
)

type Entry struct {
	Time                             time.Time `json:"time"`
	Provider, Role, Content, Session string
}
type Session struct {
	Name     string    `json:"name"`
	Updated  time.Time `json:"updated"`
	Messages []Entry   `json:"messages"`
}

func baseDir() (string, error) {
	b, e := os.UserCacheDir()
	if e != nil {
		return "", e
	}
	return filepath.Join(b, "aish"), nil
}
func path() (string, error)         { b, e := baseDir(); return filepath.Join(b, "history.jsonl"), e }
func sessionsPath() (string, error) { b, e := baseDir(); return filepath.Join(b, "sessions.json"), e }
func Append(e Entry) error {
	p, err := path()
	if err != nil {
		return err
	}
	os.MkdirAll(filepath.Dir(p), 0700)
	f, err := os.OpenFile(p, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	defer f.Close()
	b, err := json.Marshal(e)
	if err != nil {
		return err
	}
	b, err = securestore.Encrypt(b)
	if err != nil {
		return err
	}
	_, err = f.Write(append(b, '\n'))
	return err
}
func ReadAll() ([]Entry, error) {
	p, e := path()
	if e != nil {
		return nil, e
	}
	f, e := os.Open(p)
	if errors.Is(e, os.ErrNotExist) {
		return nil, nil
	}
	if e != nil {
		return nil, e
	}
	defer f.Close()
	var out []Entry
	s := bufio.NewScanner(f)
	for s.Scan() {
		b, decErr := securestore.Decrypt(append([]byte(nil), s.Bytes()...))
		if decErr != nil {
			return nil, decErr
		}
		var x Entry
		if json.Unmarshal(b, &x) == nil {
			out = append(out, x)
		}
	}
	return out, s.Err()
}
func loadSessions() (map[string]Session, error) {
	p, e := sessionsPath()
	if e != nil {
		return nil, e
	}
	b, e := os.ReadFile(p)
	if errors.Is(e, os.ErrNotExist) {
		return map[string]Session{}, nil
	}
	if e != nil {
		return nil, e
	}
	b, e = securestore.Decrypt(b)
	if e != nil {
		return nil, e
	}
	m := map[string]Session{}
	e = json.Unmarshal(b, &m)
	return m, e
}
func saveSessions(m map[string]Session) error {
	p, e := sessionsPath()
	if e != nil {
		return e
	}
	os.MkdirAll(filepath.Dir(p), 0700)
	b, e := json.MarshalIndent(m, "", "  ")
	if e != nil {
		return e
	}
	b, e = securestore.Encrypt(b)
	if e != nil {
		return e
	}
	return os.WriteFile(p, b, 0600)
}
func SaveSession(name string, entries []Entry) error {
	m, e := loadSessions()
	if e != nil {
		return e
	}
	for i := range entries {
		entries[i].Session = name
	}
	m[name] = Session{Name: name, Updated: time.Now(), Messages: entries}
	return saveSessions(m)
}
func LoadSession(name string) (Session, error) {
	m, e := loadSessions()
	if e != nil {
		return Session{}, e
	}
	s, ok := m[name]
	if !ok {
		return Session{}, os.ErrNotExist
	}
	return s, nil
}
func ListSessions() ([]Session, error) {
	m, e := loadSessions()
	if e != nil {
		return nil, e
	}
	out := make([]Session, 0, len(m))
	for _, s := range m {
		out = append(out, s)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Updated.After(out[j].Updated) })
	return out, nil
}
func DeleteSession(name string) error {
	m, e := loadSessions()
	if e != nil {
		return e
	}
	delete(m, name)
	return saveSessions(m)
}
