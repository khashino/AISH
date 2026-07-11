package app

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/khashino/AISH/internal/config"
	"github.com/khashino/AISH/internal/documents"
	"github.com/khashino/AISH/internal/executor"
	"github.com/khashino/AISH/internal/history"
	"github.com/khashino/AISH/internal/provider"
	"github.com/khashino/AISH/internal/provider/anthropic"
	"github.com/khashino/AISH/internal/provider/gemini"
	"github.com/khashino/AISH/internal/provider/ollama"
	"github.com/khashino/AISH/internal/provider/openai"
	"github.com/khashino/AISH/internal/provider/openaicompat"
)

const version = "0.6.0-dev"

func Run(args []string) error {
	cfg, e := config.Load()
	if e != nil {
		return e
	}
	if len(args) == 0 || args[0] == "chat" {
		return chat(cfg, argValue(args, "--session"))
	}
	switch args[0] {
	case "-config", "config":
		return configCommand(cfg, args[1:])
	case "setup":
		return setup(cfg)
	case "doctor":
		return doctor(cfg)
	case "ask":
		if len(args) < 2 {
			return fmt.Errorf("usage: aish ask QUESTION")
		}
		return ask(cfg, strings.Join(args[1:], " "), nil)
	case "do":
		if len(args) < 2 {
			return fmt.Errorf("usage: aish do TASK")
		}
		return doCommand(cfg, strings.Join(args[1:], " "))
	case "provider":
		return providerCommand(cfg, args[1:])
	case "history":
		return historyCommand()
	case "session", "sessions":
		return sessionCommand(args[1:])
	case "docs":
		return docsCommand(cfg, args[1:])
	case "run":
		if len(args) < 2 {
			return fmt.Errorf("usage: aish run COMMAND")
		}
		return executor.Run(strings.Join(args[1:], " "), cfg.RequireConfirm)
	case "update":
		return updateCommand()
	case "version", "--version", "-v":
		fmt.Println("aish", version)
		return nil
	case "help", "--help", "-h":
		printHelp()
		return nil
	default:
		return fmt.Errorf("unknown command %q; run 'aish help'", args[0])
	}
}
func argValue(a []string, key string) string {
	for i := range a {
		if a[i] == key && i+1 < len(a) {
			return a[i+1]
		}
	}
	return ""
}
func client(cfg config.Config) (provider.Client, error) {
	pc, ok := cfg.Providers[cfg.ActiveProvider]
	if !ok {
		return nil, fmt.Errorf("provider %q not configured", cfg.ActiveProvider)
	}
	key := ""
	if pc.APIKeyEnv != "" {
		key = os.Getenv(pc.APIKeyEnv)
		if key == "" && cfg.ActiveProvider != "llamacpp" {
			return nil, fmt.Errorf("%s is not set", pc.APIKeyEnv)
		}
	}
	switch cfg.ActiveProvider {
	case "ollama":
		return ollama.New(pc.BaseURL, pc.Model), nil
	case "llamacpp":
		return openaicompat.New("llamacpp", pc.BaseURL, pc.Model, key), nil
	case "openai":
		return openai.New(pc.BaseURL, pc.Model, key), nil
	case "claude":
		return anthropic.New(pc.BaseURL, pc.Model, key), nil
	case "gemini":
		return gemini.New(pc.BaseURL, pc.Model, key), nil
	}
	return nil, fmt.Errorf("unknown provider")
}
func streamAnswer(c provider.Client, m []provider.Message) (string, error) {
	var mu sync.Mutex
	first := true
	done := make(chan struct{})
	go func() {
		frames := []string{".", "..", "..."}
		t := time.NewTicker(350 * time.Millisecond)
		defer t.Stop()
		i := 0
		for {
			select {
			case <-done:
				return
			case <-t.C:
				mu.Lock()
				if first {
					fmt.Printf("\raish> thinking%-3s", frames[i%3])
					i++
				}
				mu.Unlock()
			}
		}
	}()
	fmt.Print("aish> thinking...")
	ans, e := c.Stream(context.Background(), m, func(s string) error {
		mu.Lock()
		defer mu.Unlock()
		if first {
			first = false
			fmt.Print("\r\033[2K", "aish> ")
		}
		fmt.Print(s)
		return nil
	})
	close(done)
	mu.Lock()
	defer mu.Unlock()
	if first {
		fmt.Print("\r\033[2K")
	}
	if ans != "" {
		fmt.Println()
	}
	return ans, e
}
func saveTurn(c provider.Client, p, a, s string) {
	now := time.Now()
	_ = history.Append(history.Entry{Time: now, Provider: c.Name(), Role: "user", Content: p, Session: s})
	_ = history.Append(history.Entry{Time: now, Provider: c.Name(), Role: "assistant", Content: a, Session: s})
}
func ask(cfg config.Config, p string, extra []provider.Message) error {
	c, e := client(cfg)
	if e != nil {
		return e
	}
	m := append(extra, provider.Message{Role: "user", Content: p})
	a, e := streamAnswer(c, m)
	if e == nil {
		saveTurn(c, p, a, "")
	}
	return e
}
func chat(cfg config.Config, session string) error {
	c, e := client(cfg)
	if e != nil {
		return e
	}
	var messages []provider.Message
	var sessionEntries []history.Entry
	if session != "" {
		s, e := history.LoadSession(session)
		if e == nil {
			for _, x := range s.Messages {
				messages = append(messages, provider.Message{Role: x.Role, Content: x.Content})
				sessionEntries = s.Messages
			}
		}
	}
	fmt.Printf("AISH %s | provider=%s | /help for commands\n", version, c.Name())
	sc := bufio.NewScanner(os.Stdin)
	sc.Buffer(make([]byte, 65536), 1048576)
	for {
		fmt.Print("you> ")
		if !sc.Scan() {
			if session != "" {
				_ = history.SaveSession(session, sessionEntries)
			}
			return sc.Err()
		}
		text := strings.TrimSpace(sc.Text())
		if text == "" {
			continue
		}
		if strings.HasPrefix(text, "/") {
			switch {
			case text == "/exit" || text == "/quit":
				if session != "" {
					_ = history.SaveSession(session, sessionEntries)
				}
				return nil
			case text == "/clear":
				messages = nil
				sessionEntries = nil
				fmt.Println("conversation cleared")
			case text == "/help":
				fmt.Println("/status /clear /history /save NAME /exit")
			case text == "/status":
				pc := cfg.Providers[cfg.ActiveProvider]
				fmt.Printf("provider=%s model=%s endpoint=%s turns=%d session=%s\n", cfg.ActiveProvider, pc.Model, pc.BaseURL, len(messages)/2, session)
			case text == "/history":
				_ = historyCommand()
			case strings.HasPrefix(text, "/save "):
				session = strings.TrimSpace(strings.TrimPrefix(text, "/save "))
				_ = history.SaveSession(session, sessionEntries)
				fmt.Println("saved session", session)
			default:
				fmt.Println("unknown chat command")
			}
			continue
		}
		messages = append(messages, provider.Message{Role: "user", Content: text})
		a, e := streamAnswer(c, messages)
		if e != nil {
			fmt.Println("error:", e)
			messages = messages[:len(messages)-1]
			continue
		}
		messages = append(messages, provider.Message{Role: "assistant", Content: a})
		now := time.Now()
		sessionEntries = append(sessionEntries, history.Entry{Time: now, Provider: c.Name(), Role: "user", Content: text}, history.Entry{Time: now, Provider: c.Name(), Role: "assistant", Content: a})
		saveTurn(c, text, a, session)
		if session != "" {
			_ = history.SaveSession(session, sessionEntries)
		}
	}
}

type proposal struct {
	Command   string `json:"command"`
	Reason    string `json:"reason"`
	Dangerous bool   `json:"dangerous"`
}

func doCommand(cfg config.Config, task string) error {
	c, e := client(cfg)
	if e != nil {
		return e
	}
	sys := `You translate a user task into exactly one terminal command for the current OS. Return only JSON: {"command":"...","reason":"...","dangerous":false}. Never use markdown. OS=` + runtime.GOOS
	raw, e := c.Chat(context.Background(), []provider.Message{{Role: "system", Content: sys}, {Role: "user", Content: task}})
	if e != nil {
		return e
	}
	var p proposal
	if e = json.Unmarshal([]byte(strings.TrimSpace(raw)), &p); e != nil {
		return fmt.Errorf("model returned invalid command proposal: %s", raw)
	}
	if e = executor.Validate(p.Command); e != nil {
		return e
	}
	if !executor.Confirm(p.Command, p.Reason, p.Dangerous) {
		fmt.Println("Cancelled.")
		return nil
	}
	out, runErr := executor.Capture(p.Command, 60*time.Second)
	fmt.Print(out)
	summaryPrompt := fmt.Sprintf("Task: %s\nCommand: %s\nOutput:\n%s\nExplain the result briefly and mention errors.", task, p.Command, out)
	_, sumErr := streamAnswer(c, []provider.Message{{Role: "user", Content: summaryPrompt}})
	if runErr != nil {
		return runErr
	}
	return sumErr
}
func setup(cfg config.Config) error {
	in := bufio.NewReader(os.Stdin)
	providers := []string{"ollama", "llamacpp", "openai", "claude", "gemini"}

	fmt.Println("AISH setup")
	fmt.Println("Choose an AI provider:")
	for i, name := range providers {
		marker := " "
		if name == cfg.ActiveProvider {
			marker = "*"
		}
		fmt.Printf("  %d) %-9s %s\n", i+1, name, marker)
	}
	fmt.Printf("Provider number [%s]: ", cfg.ActiveProvider)
	choice, err := in.ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return err
	}
	choice = strings.TrimSpace(choice)
	if choice != "" {
		var selected string
		for i, name := range providers {
			if choice == fmt.Sprint(i+1) || strings.EqualFold(choice, name) {
				selected = name
				break
			}
		}
		if selected == "" {
			return fmt.Errorf("invalid provider selection %q; choose 1-%d", choice, len(providers))
		}
		cfg.ActiveProvider = selected
	}

	pc, ok := cfg.Providers[cfg.ActiveProvider]
	if !ok {
		return fmt.Errorf("provider %q is not configured", cfg.ActiveProvider)
	}
	fmt.Printf("Model [%s]: ", pc.Model)
	m, _ := in.ReadString('\n')
	if value := strings.TrimSpace(m); value != "" {
		pc.Model = value
	}

	if cfg.ActiveProvider == "ollama" {
		if x := detectWSLOllama(); x != "" && (pc.BaseURL == "" || strings.Contains(pc.BaseURL, "localhost") || strings.Contains(pc.BaseURL, "127.0.0.1")) {
			pc.BaseURL = x
			fmt.Println("Detected Windows Ollama:", x)
		}
	}
	fmt.Printf("Base URL [%s]: ", pc.BaseURL)
	u, _ := in.ReadString('\n')
	if value := strings.TrimSpace(u); value != "" {
		pc.BaseURL = value
	}

	cfg.Providers[cfg.ActiveProvider] = pc
	if err := config.Save(cfg); err != nil {
		return err
	}
	fmt.Printf("Saved provider=%s model=%s base_url=%s\n", cfg.ActiveProvider, pc.Model, pc.BaseURL)
	return nil
}
func detectWSLOllama() string {
	if runtime.GOOS != "linux" {
		return ""
	}
	b, e := os.ReadFile("/proc/version")
	if e != nil || !strings.Contains(strings.ToLower(string(b)), "microsoft") {
		return ""
	}
	out, e := exec.Command("sh", "-c", "ip route show | awk '/default/ {print $3; exit}'").Output()
	if e != nil {
		return ""
	}
	url := "http://" + strings.TrimSpace(string(out)) + ":11434"
	cl := http.Client{Timeout: 1200 * time.Millisecond}
	r, e := cl.Get(url + "/api/tags")
	if e == nil {
		r.Body.Close()
		if r.StatusCode == 200 {
			return url
		}
	}
	return ""
}
func doctor(cfg config.Config) error {
	pc := cfg.Providers[cfg.ActiveProvider]
	fmt.Printf("AISH doctor\n[OK  ] OS %s/%s\n[OK  ] Provider %s\n[OK  ] Model %s\n", runtime.GOOS, runtime.GOARCH, cfg.ActiveProvider, pc.Model)
	if cfg.ActiveProvider == "ollama" {
		url := pc.BaseURL
		if auto := detectWSLOllama(); auto != "" && strings.Contains(url, "localhost") {
			url = auto
			fmt.Println("[INFO] WSL Ollama detected at", auto)
		}
		r, e := http.Get(strings.TrimRight(url, "/") + "/api/tags")
		if e != nil {
			return fmt.Errorf("[FAIL] connectivity: %w", e)
		}
		defer r.Body.Close()
		b, _ := io.ReadAll(r.Body)
		if !strings.Contains(string(b), pc.Model) {
			return fmt.Errorf("[FAIL] connected, but model %s not found", pc.Model)
		}
		fmt.Println("[OK  ] Connectivity and model")
	}
	fmt.Println("AISH is ready.")
	return nil
}
func sessionCommand(a []string) error {
	if len(a) == 0 || a[0] == "list" {
		s, e := history.ListSessions()
		if e != nil {
			return e
		}
		for _, x := range s {
			fmt.Printf("%-24s %s %d messages\n", x.Name, x.Updated.Format("2006-01-02 15:04"), len(x.Messages))
		}
		return nil
	}
	if a[0] == "open" && len(a) == 2 {
		cfg, e := config.Load()
		if e != nil {
			return e
		}
		return chat(cfg, a[1])
	}
	if a[0] == "delete" && len(a) == 2 {
		return history.DeleteSession(a[1])
	}
	return fmt.Errorf("usage: aish session [list|open NAME|delete NAME]")
}
func updateCommand() error {
	asset := fmt.Sprintf("aish-%s-%s", runtime.GOOS, runtime.GOARCH)
	if runtime.GOOS == "windows" {
		asset += ".exe"
	}
	url := "https://github.com/khashino/AISH/releases/latest/download/" + asset
	fmt.Println("Downloading", url)
	resp, err := http.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("update download returned %s (publish a GitHub release first)", resp.Status)
	}
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	tmp := exe + ".new"
	f, err := os.OpenFile(tmp, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0755)
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(f, resp.Body)
	closeErr := f.Close()
	if copyErr != nil {
		return copyErr
	}
	if closeErr != nil {
		return closeErr
	}
	if runtime.GOOS == "windows" {
		fmt.Printf("Downloaded update to %s. Close AISH, replace %s with that file, and rename it to aish.exe.\n", tmp, exe)
		return nil
	}
	if err := os.Rename(tmp, exe); err != nil {
		return fmt.Errorf("downloaded to %s but could not replace executable: %w", tmp, err)
	}
	fmt.Println("AISH updated successfully. Run 'aish version' to verify.")
	return nil
}
func configCommand(cfg config.Config, a []string) error {
	if len(a) == 0 || a[0] == "show" {
		pc := cfg.Providers[cfg.ActiveProvider]
		fmt.Printf("Provider: %s\nModel: %s\nBase URL: %s\n", cfg.ActiveProvider, pc.Model, pc.BaseURL)
		return nil
	}
	if a[0] == "set" && len(a) >= 3 {
		k, v := a[1], strings.Join(a[2:], " ")
		pc := cfg.Providers[cfg.ActiveProvider]
		switch k {
		case "model":
			pc.Model = v
		case "base-url":
			pc.BaseURL = v
		case "api-key-env":
			pc.APIKeyEnv = v
		case "confirm":
			cfg.RequireConfirm = v != "false"
		case "embedding-model":
			cfg.Documents.EmbeddingModel = v
		default:
			return fmt.Errorf("unknown config key")
		}
		cfg.Providers[cfg.ActiveProvider] = pc
		return config.Save(cfg)
	}
	return fmt.Errorf("usage: aish config [show|set KEY VALUE]")
}
func providerCommand(cfg config.Config, a []string) error {
	if len(a) == 0 || a[0] == "list" {
		var ns []string
		for n := range cfg.Providers {
			ns = append(ns, n)
		}
		sort.Strings(ns)
		for _, n := range ns {
			m := " "
			if n == cfg.ActiveProvider {
				m = "*"
			}
			fmt.Printf("%s %-10s %s\n", m, n, cfg.Providers[n].Model)
		}
		return nil
	}
	if a[0] == "use" && len(a) == 2 {
		if _, ok := cfg.Providers[a[1]]; !ok {
			return fmt.Errorf("unknown provider")
		}
		cfg.ActiveProvider = a[1]
		return config.Save(cfg)
	}
	return fmt.Errorf("usage: aish provider [list|use NAME]")
}
func docsCommand(cfg config.Config, a []string) error {
	if len(a) == 0 {
		return fmt.Errorf("usage: aish docs [add|list|search|ask]")
	}
	ep := cfg.Providers[cfg.Documents.EmbeddingProvider]
	switch a[0] {
	case "add":
		if len(a) < 2 {
			return fmt.Errorf("path required")
		}
		n, e := documents.Add(context.Background(), strings.Join(a[1:], " "), ep.BaseURL, cfg.Documents.EmbeddingModel)
		if e == nil {
			fmt.Printf("indexed %d chunks\n", n)
		}
		return e
	case "list":
		m, e := documents.List()
		if e != nil {
			return e
		}
		for p, n := range m {
			fmt.Printf("%4d %s\n", n, p)
		}
		return nil
	case "search", "ask":
		if len(a) < 2 {
			return fmt.Errorf("query required")
		}
		q := strings.Join(a[1:], " ")
		cs, e := documents.Search(context.Background(), q, ep.BaseURL, cfg.Documents.EmbeddingModel, cfg.Documents.TopK)
		if e != nil {
			return e
		}
		if a[0] == "search" {
			for i, c := range cs {
				fmt.Printf("\n[%d] %s\n%s\n", i+1, c.Path, c.Text)
			}
			return nil
		}
		var b strings.Builder
		b.WriteString("Use this local context and cite paths.\n")
		for _, x := range cs {
			fmt.Fprintf(&b, "SOURCE %s\n%s\n", x.Path, x.Text)
		}
		return ask(cfg, q, []provider.Message{{Role: "system", Content: b.String()}})
	}
	return fmt.Errorf("unknown docs command")
}
func historyCommand() error {
	es, e := history.ReadAll()
	if e != nil {
		return e
	}
	start := 0
	if len(es) > 30 {
		start = len(es) - 30
	}
	for _, x := range es[start:] {
		c := strings.ReplaceAll(x.Content, "\n", " ")
		if len(c) > 100 {
			c = c[:100] + "..."
		}
		fmt.Printf("%s %-9s %s\n", x.Time.Format("01-02 15:04"), x.Role, c)
	}
	return nil
}
func printHelp() {
	fmt.Println(`AISH — one shell for every AI

Commands:
  aish                         Interactive chat
  aish ask "QUESTION"          One question
  aish do "TASK"               Propose, approve, run, and explain a command
  aish setup                   Guided setup with WSL detection
  aish doctor                  Diagnose connectivity and model
  aish session list            List persistent sessions
  aish session open NAME       Continue a session
  aish docs add PATH           Index text, code, DOCX, and PDF
  aish docs search QUERY       Search local documents
  aish docs ask QUESTION       RAG question
  aish provider use NAME       Switch provider
  aish config                  Show configuration
  aish history                 Recent history
  aish run "COMMAND"           Run an exact command
  aish update                  Update information
  aish version                 Version`)
}

var _ = errors.Is
