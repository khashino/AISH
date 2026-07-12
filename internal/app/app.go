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
	"os/signal"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/khashino/AISH/internal/agent"
	"github.com/khashino/AISH/internal/config"
	"github.com/khashino/AISH/internal/documents"
	"github.com/khashino/AISH/internal/executor"
	"github.com/khashino/AISH/internal/history"
	"github.com/khashino/AISH/internal/knowledge"
	"github.com/khashino/AISH/internal/project"
	"github.com/khashino/AISH/internal/provider"
	"github.com/khashino/AISH/internal/provider/anthropic"
	"github.com/khashino/AISH/internal/provider/gemini"
	"github.com/khashino/AISH/internal/provider/ollama"
	"github.com/khashino/AISH/internal/provider/openai"
	"github.com/khashino/AISH/internal/provider/openaicompat"
	usagepkg "github.com/khashino/AISH/internal/usage"
)

const version = "1.0.0-dev"

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
	case "agent":
		return agentCommand(cfg, args[1:])
	case "knowledge", "kb":
		return knowledgeCommand(cfg, args[1:])
	case "privacy":
		return privacyCommand(cfg)
	case "usage":
		return usageCommand(cfg, args[1:])
	case "pricing":
		return pricingCommand(cfg, args[1:])
	case "project":
		if len(args) > 1 && args[1] == "context" {
			fmt.Print(project.Context("."))
			return nil
		}
		return fmt.Errorf("usage: aish project context")
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
func usageFromClient(c provider.Client, messages []provider.Message, answer string) provider.Usage {
	if r, ok := c.(provider.UsageReporter); ok {
		u := r.LastUsage()
		if u.TotalTokens > 0 || u.InputTokens > 0 || u.OutputTokens > 0 {
			if u.TotalTokens == 0 {
				u.TotalTokens = u.InputTokens + u.OutputTokens
			}
			return u
		}
	}
	return usagepkg.Estimate(messages, answer)
}

func recordUsage(cfg config.Config, c provider.Client, messages []provider.Message, answer, operation, taskID, session string, started time.Time, display bool) provider.Usage {
	u := usageFromClient(c, messages, answer)
	pc := cfg.Providers[cfg.ActiveProvider]
	cost := float64(u.InputTokens)*pc.InputCostPerMillion/1_000_000 + float64(u.OutputTokens)*pc.OutputCostPerMillion/1_000_000
	r := usagepkg.Record{Time: time.Now(), Provider: c.Name(), Model: pc.Model, Operation: operation, TaskID: taskID, Session: session, InputTokens: u.InputTokens, OutputTokens: u.OutputTokens, TotalTokens: u.TotalTokens, Estimated: u.Estimated, DurationMS: time.Since(started).Milliseconds(), CostUSD: cost}
	_ = usagepkg.Append(r)
	if display && cfg.ShowUsage != "off" {
		mark := ""
		if u.Estimated {
			mark = "~"
		}
		if cfg.ShowUsage == "always" {
			fmt.Printf("Usage:\n  Input tokens:  %s%d\n  Output tokens: %s%d\n  Total tokens:  %s%d\n  Duration:      %.2fs\n", mark, u.InputTokens, mark, u.OutputTokens, mark, u.TotalTokens, float64(r.DurationMS)/1000)
			if cost > 0 {
				fmt.Printf("  Estimated cost: $%.6f\n", cost)
			}
		} else {
			fmt.Printf("[%s%d tokens · %.2fs · %s/%s]\n", mark, u.TotalTokens, float64(r.DurationMS)/1000, c.Name(), pc.Model)
		}
	}
	return u
}

func measuredChat(cfg config.Config, c provider.Client, m []provider.Message, operation, taskID, session string, display bool) (string, error) {
	started := time.Now()
	ans, err := c.Chat(context.Background(), m)
	if err == nil {
		recordUsage(cfg, c, m, ans, operation, taskID, session, started, display)
	}
	return ans, err
}

func streamAnswer(cfg config.Config, c provider.Client, m []provider.Message, operation, taskID, session string, display bool) (string, error) {
	started := time.Now()
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
	if first {
		fmt.Print("\r\033[2K")
	}
	if ans != "" {
		fmt.Println()
	}
	mu.Unlock()
	if e == nil {
		recordUsage(cfg, c, m, ans, operation, taskID, session, started, display)
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
	a, e := streamAnswer(cfg, c, m, "ask", "", "", true)
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
				fmt.Println("/status /usage /clear /history /save NAME /exit")
			case text == "/status":
				pc := cfg.Providers[cfg.ActiveProvider]
				fmt.Printf("provider=%s model=%s endpoint=%s turns=%d session=%s\n", cfg.ActiveProvider, pc.Model, pc.BaseURL, len(messages)/2, session)
			case text == "/history":
				_ = historyCommand()
			case text == "/usage":
				_ = printUsageSummary("Current session", usagepkg.Filter(mustUsage(), "", session))
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
		a, e := streamAnswer(cfg, c, messages, "chat", "", session, true)
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

func parseProposal(raw, task string) (proposal, error) {
	text := strings.TrimSpace(raw)
	if text == "" {
		return proposal{}, fmt.Errorf("model returned an empty command proposal")
	}

	// Preferred format: strict JSON.
	var p proposal
	if json.Unmarshal([]byte(text), &p) == nil && strings.TrimSpace(p.Command) != "" {
		p.Command = strings.TrimSpace(p.Command)
		return p, nil
	}

	// Some small/local models wrap JSON in a Markdown fence.
	if start := strings.Index(text, "{"); start >= 0 {
		if end := strings.LastIndex(text, "}"); end > start {
			if json.Unmarshal([]byte(text[start:end+1]), &p) == nil && strings.TrimSpace(p.Command) != "" {
				p.Command = strings.TrimSpace(p.Command)
				return p, nil
			}
		}
	}

	// Fallback for models that return only a command or a fenced shell block.
	lines := strings.Split(text, "\n")
	var commandLines []string
	inFence := false
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "```") {
			inFence = !inFence
			continue
		}
		if inFence {
			if trimmed != "" {
				commandLines = append(commandLines, trimmed)
			}
		}
	}
	command := strings.Join(commandLines, "\n")
	if command == "" {
		command = strings.Trim(text, "` \t\r\n")
		if i := strings.IndexByte(command, '\n'); i >= 0 {
			command = strings.TrimSpace(command[:i])
		}
	}
	if command == "" {
		return proposal{}, fmt.Errorf("model returned invalid command proposal: %s", raw)
	}
	return proposal{Command: command, Reason: "Generated for: " + task}, nil
}

func deterministicProposal(task string) (proposal, bool) {
	n := strings.ToLower(strings.TrimSpace(task))
	mentionsLargest := strings.Contains(n, "largest") || strings.Contains(n, "biggest")
	mentionsFiles := strings.Contains(n, "file")
	mentionsCurrentFolder := strings.Contains(n, "this folder") || strings.Contains(n, "current folder") || strings.Contains(n, "current directory") || strings.Contains(n, "here")
	if !mentionsLargest || !mentionsFiles || !mentionsCurrentFolder {
		return proposal{}, false
	}
	if runtime.GOOS == "windows" {
		return proposal{
			Command: `powershell -NoProfile -Command "Get-ChildItem -File | Sort-Object Length -Descending | Select-Object -First 10 Name,Length"`,
			Reason:  "List the ten largest files in the current folder by byte size.",
		}, true
	}
	return proposal{
		Command: `find . -maxdepth 1 -type f -printf '%s\t%p\n' | sort -nr | head -n 10`,
		Reason:  "List the ten largest files in the current folder by byte size.",
	}, true
}

func doCommand(cfg config.Config, task string) error {
	c, e := client(cfg)
	if e != nil {
		return e
	}
	var p proposal
	if builtIn, ok := deterministicProposal(task); ok {
		p = builtIn
	} else {
		sys := `You translate a user task into exactly one terminal command for the current OS. Return only JSON: {"command":"...","reason":"...","dangerous":false}. Never use markdown. Prefer simple portable commands. For numeric sorting, prefer sort -n or sort -nr instead of complex -k expressions. OS=` + runtime.GOOS
		messages := []provider.Message{{Role: "system", Content: sys}, {Role: "user", Content: task}}
		for attempt := 1; attempt <= 3; attempt++ {
			raw, chatErr := measuredChat(cfg, c, messages, "do-plan", "", "", false)
			if chatErr != nil {
				return chatErr
			}
			p, e = parseProposal(raw, task)
			if e == nil {
				e = executor.Validate(p.Command)
			}
			if e == nil {
				break
			}
			if attempt == 3 {
				return fmt.Errorf("model could not produce a valid command after 3 attempts: %w", e)
			}
			fmt.Printf("AISH rejected an invalid command and is asking the model to correct it (%d/2).\n", attempt)
			messages = append(messages,
				provider.Message{Role: "assistant", Content: raw},
				provider.Message{Role: "user", Content: "The proposed command was rejected: " + e.Error() + ". Return corrected JSON only. Use one syntactically valid command for " + runtime.GOOS + ". Do not use Markdown or backticks."},
			)
		}
	}
	if !executor.Confirm(p.Command, p.Reason, p.Dangerous) {
		fmt.Println("Cancelled.")
		return nil
	}
	out, runErr := executor.Capture(p.Command, 60*time.Second)
	fmt.Print(out)
	summaryPrompt := fmt.Sprintf("Task: %s\nCommand: %s\nOutput:\n%s\nExplain the result briefly and mention errors.", task, p.Command, out)
	_, sumErr := streamAnswer(cfg, c, []provider.Message{{Role: "user", Content: summaryPrompt}}, "do-summary", "", "", true)
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
		case "show-usage":
			if v != "off" && v != "summary" && v != "always" {
				return fmt.Errorf("show-usage must be off, summary, or always")
			}
			cfg.ShowUsage = v
		case "pricing-input":
			f, err := strconv.ParseFloat(v, 64)
			if err != nil {
				return err
			}
			pc.InputCostPerMillion = f
		case "pricing-output":
			f, err := strconv.ParseFloat(v, 64)
			if err != nil {
				return err
			}
			pc.OutputCostPerMillion = f
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
		return fmt.Errorf("usage: aish docs [add|list|remove|clear|search|ask]")
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
		if len(m) == 0 {
			fmt.Println("no indexed documents")
			return nil
		}
		paths := make([]string, 0, len(m))
		for p := range m {
			paths = append(paths, p)
		}
		sort.Strings(paths)
		for _, p := range paths {
			fmt.Printf("%4d %s\n", m[p], p)
		}
		return nil
	case "remove", "delete", "rm":
		if len(a) < 2 {
			return fmt.Errorf("usage: aish docs remove PATH")
		}
		path := strings.Join(a[1:], " ")
		files, chunks, e := documents.Remove(path)
		if e != nil {
			return e
		}
		if chunks == 0 {
			fmt.Printf("no indexed documents matched %s\n", path)
			return nil
		}
		fmt.Printf("removed %d file(s) and %d chunk(s)\n", files, chunks)
		return nil
	case "clear":
		files, chunks, e := documents.Clear()
		if e != nil {
			return e
		}
		fmt.Printf("cleared %d file(s) and %d chunk(s)\n", files, chunks)
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
  aish do "TASK"               Propose, approve, run, and explain one command
  aish agent "TASK"            Plan and run a resumable multi-step task
  aish agent list              List saved agent tasks
  aish agent resume ID         Resume a paused task
  aish project context         Show detected project and Git context
  aish knowledge create NAME   Create a personal knowledge collection
  aish knowledge add PATH      Index files in the active collection
  aish knowledge watch PATH    Watch and re-index changed files
  aish knowledge ask QUESTION  Answer with source path citations
  aish privacy                 Show local/cloud and encryption status
  aish usage [today|session|task|export|reset]  Token, duration, and cost usage
  aish pricing [show|set]      Configure optional cloud token prices
  aish setup                   Guided setup with WSL detection
  aish doctor                  Diagnose connectivity and model
  aish session list            List persistent sessions
  aish session open NAME       Continue a session
  aish docs add PATH           Index text, code, DOCX, and PDF
  aish docs list               List indexed documents
  aish docs remove PATH        Remove one file or folder from the index
  aish docs clear              Remove the complete document index
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

type planResponse struct {
	Steps []struct {
		Title   string `json:"title"`
		Command string `json:"command"`
	} `json:"steps"`
}

func parsePlan(raw string) ([]agent.Step, error) {
	text := strings.TrimSpace(raw)
	if i := strings.Index(text, "{"); i >= 0 {
		if j := strings.LastIndex(text, "}"); j > i {
			text = text[i : j+1]
		}
	}
	var p planResponse
	if err := json.Unmarshal([]byte(text), &p); err != nil {
		return nil, fmt.Errorf("invalid agent plan: %w", err)
	}
	if len(p.Steps) == 0 {
		return nil, fmt.Errorf("agent returned an empty plan")
	}
	if len(p.Steps) > 12 {
		return nil, fmt.Errorf("agent plan has too many steps (%d; max 12)", len(p.Steps))
	}
	out := make([]agent.Step, 0, len(p.Steps))
	for _, s := range p.Steps {
		if strings.TrimSpace(s.Command) == "" {
			return nil, fmt.Errorf("plan step has empty command")
		}
		if err := executor.Validate(s.Command); err != nil {
			return nil, fmt.Errorf("invalid step %q: %w", s.Title, err)
		}
		out = append(out, agent.Step{Title: s.Title, Command: s.Command, Status: "pending"})
	}
	return out, nil
}

func printAgentPlan(t agent.Task) {
	fmt.Printf("Agent task: %s\nID: %s\nDirectory: %s\nPlan:\n", t.Goal, t.ID, t.Workdir)
	for i, s := range t.Steps {
		fmt.Printf("  %d. [%s] %s\n     %s\n", i+1, s.Status, s.Title, s.Command)
	}
}

func resolveAgentID(value string) (string, error) {
	if n, err := strconv.Atoi(value); err == nil {
		tasks, listErr := agent.List()
		if listErr != nil {
			return "", listErr
		}
		if n < 1 || n > len(tasks) {
			return "", fmt.Errorf("agent task number %d is out of range", n)
		}
		return tasks[n-1].ID, nil
	}
	return value, nil
}

func deterministicAgentPlan(goal, cwd string) ([]agent.Step, bool) {
	g := strings.ToLower(goal)
	_, goModErr := os.Stat(filepath.Join(cwd, "go.mod"))
	isGo := goModErr == nil
	if isGo && strings.Contains(g, "test") && (strings.Contains(g, "inspect") || strings.Contains(g, "go project") || strings.Contains(g, "summar")) {
		steps := []agent.Step{
			{Title: "Inspect Go environment", Command: "go version", Status: "pending"},
			{Title: "Inspect module", Command: "go env GOMOD", Status: "pending"},
			{Title: "Check Git status", Command: "git status --short", Status: "pending"},
			{Title: "Run tests", Command: "go test ./...", Status: "pending"},
			{Title: "Run static checks", Command: "go vet ./...", Status: "pending"},
		}
		return steps, true
	}
	return nil, false
}

func validateAgentPlanStep(command, cwd string) error {
	if err := executor.Validate(command); err != nil {
		return err
	}
	lower := strings.ToLower(strings.TrimSpace(command))
	for _, bad := range []string{"aish summary", "aish run tests", "aish knowledge create", "aish docs add", "go mod init -v"} {
		if strings.HasPrefix(lower, bad) {
			return fmt.Errorf("unsupported or invalid agent command %q", command)
		}
	}
	if strings.HasPrefix(lower, "go mod init") {
		if _, err := os.Stat(filepath.Join(cwd, "go.mod")); err == nil {
			return fmt.Errorf("go.mod already exists; do not run go mod init")
		}
	}
	return nil
}

func summarizeAgentTask(cfg config.Config, c provider.Client, t *agent.Task) {
	var b strings.Builder
	fmt.Fprintf(&b, "Summarize this completed terminal task concisely. Goal: %s\n", t.Goal)
	for i, s := range t.Steps {
		fmt.Fprintf(&b, "Step %d: %s\nCommand: %s\nStatus: %s\nOutput:\n%s\nError: %s\n", i+1, s.Title, s.Command, s.Status, s.Output, s.Error)
	}
	raw, err := measuredChat(cfg, c, []provider.Message{{Role: "user", Content: b.String()}}, "agent-summary", t.ID, "", false)
	if err == nil && strings.TrimSpace(raw) != "" {
		fmt.Println("\nSummary:")
		fmt.Println(strings.TrimSpace(raw))
	}
}

func agentCommand(cfg config.Config, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: aish agent TASK | list | show ID | resume ID | delete ID")
	}
	switch args[0] {
	case "list":
		ts, e := agent.List()
		if e != nil {
			return e
		}
		for i, t := range ts {
			fmt.Printf("%2d. %-40s %-9s %d/%d %s\n", i+1, t.ID, t.Status, t.Current, len(t.Steps), t.Goal)
		}
		return nil
	case "show":
		if len(args) != 2 {
			return fmt.Errorf("usage: aish agent show ID")
		}
		id, e := resolveAgentID(args[1])
		if e != nil {
			return e
		}
		t, e := agent.Load(id)
		if e != nil {
			return e
		}
		printAgentPlan(t)
		return nil
	case "delete":
		if len(args) != 2 {
			return fmt.Errorf("usage: aish agent delete ID")
		}
		id, e := resolveAgentID(args[1])
		if e != nil {
			return e
		}
		return agent.Delete(id)
	case "resume":
		if len(args) != 2 {
			return fmt.Errorf("usage: aish agent resume ID")
		}
		id, e := resolveAgentID(args[1])
		if e != nil {
			return e
		}
		t, e := agent.Load(id)
		if e != nil {
			return e
		}
		return runAgentTask(cfg, &t)
	}
	goal := strings.Join(args, " ")
	c, e := client(cfg)
	if e != nil {
		return e
	}
	cwd, _ := os.Getwd()
	taskID := agent.New(goal, cwd, c.Name(), nil).ID
	steps, deterministic := deterministicAgentPlan(goal, cwd)
	if !deterministic {
		prompt := `Create a safe, concise multi-step terminal plan. Return JSON only: {"steps":[{"title":"...","command":"..."}]}. Maximum 8 steps. Each step must be one valid, non-interactive command for OS ` + runtime.GOOS + `. Never invent AISH subcommands. Never initialize a project when project files already exist. Prefer inspection and test commands for inspection goals. Do not use sudo, destructive deletion, package installation, interactive editors, or markdown. Use the project context below.\n\n` + project.Context(cwd) + "\nGoal: " + goal
		raw, chatErr := measuredChat(cfg, c, []provider.Message{{Role: "system", Content: "You are a careful terminal planning agent. Only propose commands that actually exist."}, {Role: "user", Content: prompt}}, "agent-plan", taskID, "", false)
		if chatErr != nil {
			return chatErr
		}
		steps, e = parsePlan(raw)
		if e != nil {
			return e
		}
	}
	for _, step := range steps {
		if e := validateAgentPlanStep(step.Command, cwd); e != nil {
			return fmt.Errorf("invalid plan step %q: %w", step.Title, e)
		}
	}
	t := agent.New(goal, cwd, c.Name(), steps)
	if e = agent.Save(t); e != nil {
		return e
	}
	printAgentPlan(t)
	return runAgentTask(cfg, &t)
}

func runAgentTask(cfg config.Config, t *agent.Task) error {
	c, e := client(cfg)
	if e != nil {
		return e
	}
	in := bufio.NewReader(os.Stdin)
	t.Status = "running"
	_ = agent.Save(*t)
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(sig)
	go func() {
		<-sig
		t.Status = "paused"
		_ = agent.Save(*t)
		fmt.Printf("\nAgent task paused. Resume with: aish agent resume %s\n", t.ID)
		os.Exit(130)
	}()
	for t.Current < len(t.Steps) {
		s := &t.Steps[t.Current]
		fmt.Printf("\nStep %d/%d: %s\nCommand: %s\nApprove? [y]es/[s]kip/[p]ause/[c]ancel: ", t.Current+1, len(t.Steps), s.Title, s.Command)
		ans, _ := in.ReadString('\n')
		ans = strings.ToLower(strings.TrimSpace(ans))
		switch ans {
		case "c", "cancel":
			t.Status = "cancelled"
			_ = agent.Save(*t)
			fmt.Println("Agent task cancelled.")
			return nil
		case "p", "pause", "":
			t.Status = "paused"
			_ = agent.Save(*t)
			fmt.Println("Agent task paused. Resume with: aish agent resume " + t.ID)
			return nil
		case "s", "skip":
			s.Status = "skipped"
			t.Current++
			_ = agent.Save(*t)
			continue
		case "y", "yes":
		default:
			fmt.Println("Please choose y, s, p, or c.")
			continue
		}
		s.Status = "running"
		_ = agent.Save(*t)
		out, runErr := executor.Capture(s.Command, 2*time.Minute)
		fmt.Print(out)
		if runErr != nil {
			s.Attempts++
			s.Error = runErr.Error()
			s.Output = out
			s.Status = "failed"
			_ = agent.Save(*t)
			fixPrompt := fmt.Sprintf("A command failed in an agent task. Return JSON only with corrected command and reason: {\"command\":\"...\",\"reason\":\"...\",\"dangerous\":false}. Goal: %s\nStep: %s\nFailed command: %s\nError/output: %s\nOS: %s", t.Goal, s.Title, s.Command, out+" "+runErr.Error(), runtime.GOOS)
			raw, ce := measuredChat(cfg, c, []provider.Message{{Role: "user", Content: fixPrompt}}, "agent-correction", t.ID, "", false)
			if ce == nil && s.Attempts < 2 {
				if p, pe := parseProposal(raw, t.Goal); pe == nil && validateAgentPlanStep(p.Command, t.Workdir) == nil && strings.TrimSpace(p.Command) != strings.TrimSpace(s.Command) {
					fmt.Printf("Suggested correction: %s\nRetry corrected command? [y/N]: ", p.Command)
					a, _ := in.ReadString('\n')
					if strings.EqualFold(strings.TrimSpace(a), "y") {
						s.Command = p.Command
						s.Status = "pending"
						_ = agent.Save(*t)
						continue
					}
				}
			}
			t.Status = "paused"
			_ = agent.Save(*t)
			return fmt.Errorf("step failed; task saved for resume: %s", t.ID)
		}
		s.Output = out
		s.Status = "done"
		t.Current++
		_ = agent.Save(*t)
	}
	t.Status = "completed"
	_ = agent.Save(*t)
	fmt.Println("\nAgent task completed.")
	summarizeAgentTask(cfg, c, t)
	printTaskUsage(t.ID)
	return nil
}

func knowledgeCommand(cfg config.Config, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: aish knowledge [create|list|use|add|watch|search|ask|remove|clear|delete]")
	}
	ep := cfg.Providers[cfg.Documents.EmbeddingProvider]
	r, e := knowledge.Load()
	if e != nil {
		return e
	}
	active := r.Active
	switch args[0] {
	case "create":
		if len(args) != 2 {
			return fmt.Errorf("usage: aish knowledge create NAME")
		}
		if _, ok := r.Collections[args[1]]; ok {
			_ = knowledge.Use(args[1])
			fmt.Printf("collection %s already exists and is now active\n", args[1])
			return nil
		}
		if e := knowledge.Create(args[1]); e != nil {
			return e
		}
		fmt.Printf("created collection %s\n", args[1])
		return nil
	case "list":
		cs, a, e := knowledge.List()
		if e != nil {
			return e
		}
		for _, c := range cs {
			m := " "
			if c.Name == a {
				m = "*"
			}
			fmt.Printf("%s %-20s %d path(s)\n", m, c.Name, len(c.Paths))
		}
		return nil
	case "use":
		if len(args) != 2 {
			return fmt.Errorf("usage: aish knowledge use NAME")
		}
		if e := knowledge.Use(args[1]); e != nil {
			return e
		}
		fmt.Printf("active knowledge collection: %s\n", args[1])
		return nil
	case "delete":
		if len(args) != 2 {
			return fmt.Errorf("usage: aish knowledge delete NAME")
		}
		return knowledge.Delete(args[1])
	case "add":
		if len(args) < 2 {
			return fmt.Errorf("usage: aish knowledge add [NAME] PATH")
		}
		name, path := active, strings.Join(args[1:], " ")
		if len(args) >= 3 {
			if _, ok := r.Collections[args[1]]; ok {
				name = args[1]
				path = strings.Join(args[2:], " ")
			}
		}
		n, e := knowledge.AddPath(name, path, ep.BaseURL, cfg.Documents.EmbeddingModel)
		if e == nil {
			fmt.Printf("indexed %d chunks into %s\n", n, name)
		}
		return e
	case "watch":
		if len(args) < 2 {
			return fmt.Errorf("usage: aish knowledge watch [NAME] PATH")
		}
		name, path := active, strings.Join(args[1:], " ")
		if len(args) >= 3 {
			if _, ok := r.Collections[args[1]]; ok {
				name = args[1]
				path = strings.Join(args[2:], " ")
			}
		}
		return knowledge.Watch(name, path, ep.BaseURL, cfg.Documents.EmbeddingModel, 5*time.Second)
	case "remove":
		if len(args) < 2 {
			return fmt.Errorf("usage: aish knowledge remove [NAME] PATH")
		}
		name, path := active, strings.Join(args[1:], " ")
		if len(args) >= 3 {
			if _, ok := r.Collections[args[1]]; ok {
				name = args[1]
				path = strings.Join(args[2:], " ")
			}
		}
		f, ch, e := documents.RemoveFrom(name, path)
		if e == nil {
			fmt.Printf("removed %d file(s), %d chunk(s) from %s\n", f, ch, name)
		}
		return e
	case "clear":
		name := active
		if len(args) == 2 {
			name = args[1]
		}
		f, ch, e := documents.ClearCollection(name)
		if e == nil {
			fmt.Printf("cleared %d file(s), %d chunk(s) from %s\n", f, ch, name)
		}
		return e
	case "search", "ask":
		if len(args) < 2 {
			return fmt.Errorf("query required")
		}
		q := strings.Join(args[1:], " ")
		cs, e := documents.SearchIn(context.Background(), active, q, ep.BaseURL, cfg.Documents.EmbeddingModel, cfg.Documents.TopK)
		if e != nil {
			return e
		}
		if args[0] == "search" {
			for i, x := range cs {
				fmt.Printf("\n[%d] %s\n%s\n", i+1, x.Path, x.Text)
			}
			return nil
		}
		var b strings.Builder
		b.WriteString("Answer only from these personal knowledge sources. Cite each claim with [source: PATH]. If context is insufficient, say so.\n")
		for _, x := range cs {
			fmt.Fprintf(&b, "SOURCE %s\n%s\n", x.Path, x.Text)
		}
		return ask(cfg, q, []provider.Message{{Role: "system", Content: b.String()}})
	}
	return fmt.Errorf("unknown knowledge command")
}

func privacyCommand(cfg config.Config) error {
	pc := cfg.Providers[cfg.ActiveProvider]
	location := "cloud provider"
	if cfg.ActiveProvider == "ollama" || cfg.ActiveProvider == "llamacpp" {
		location = "local endpoint"
	}
	encrypted := "disabled"
	if os.Getenv("AISH_ENCRYPTION_KEY") != "" {
		encrypted = "enabled (AES-GCM)"
	}
	fmt.Printf("AISH privacy\nProvider: %s\nProcessing: %s (%s)\nHistory/index encryption: %s\nAPI key storage: environment variable %s\nTelemetry: none\n", cfg.ActiveProvider, location, pc.BaseURL, encrypted, pc.APIKeyEnv)
	return nil
}

func mustUsage() []usagepkg.Record {
	xs, _ := usagepkg.ReadAll()
	return xs
}

func printUsageSummary(title string, xs []usagepkg.Record) error {
	s := usagepkg.Summarize(xs)
	fmt.Println(title)
	fmt.Printf("Requests:      %d\n", s.Requests)
	fmt.Printf("Input tokens:  %d\n", s.InputTokens)
	fmt.Printf("Output tokens: %d\n", s.OutputTokens)
	fmt.Printf("Total tokens:  %d\n", s.TotalTokens)
	fmt.Printf("Duration:      %.2fs\n", float64(s.DurationMS)/1000)
	if s.CostUSD > 0 {
		fmt.Printf("Estimated cost: $%.6f\n", s.CostUSD)
	}
	if s.EstimatedRecords > 0 {
		fmt.Printf("Estimated records: %d/%d\n", s.EstimatedRecords, s.Requests)
	}
	return nil
}

func printTaskUsage(id string) {
	xs, err := usagepkg.ReadAll()
	if err != nil {
		return
	}
	filtered := usagepkg.Filter(xs, id, "")
	if len(filtered) == 0 {
		return
	}
	fmt.Println()
	_ = printUsageSummary("Agent usage", filtered)
}

func usageCommand(cfg config.Config, args []string) error {
	xs, err := usagepkg.ReadAll()
	if err != nil {
		return err
	}
	if len(args) == 0 || args[0] == "all" {
		if err := printUsageSummary("AISH usage — all time", xs); err != nil {
			return err
		}
		by := usagepkg.ByProvider(xs)
		if len(by) > 0 {
			fmt.Println("\nBy provider:")
			for _, name := range usagepkg.SortedProviders(by) {
				s := by[name]
				fmt.Printf("  %-12s %8d tokens  %d request(s)\n", name, s.TotalTokens, s.Requests)
			}
		}
		return nil
	}
	switch args[0] {
	case "today":
		return printUsageSummary("AISH usage — today", usagepkg.Today(xs, time.Now()))
	case "session":
		if len(args) != 2 {
			return fmt.Errorf("usage: aish usage session NAME")
		}
		return printUsageSummary("AISH usage — session "+args[1], usagepkg.Filter(xs, "", args[1]))
	case "task":
		if len(args) != 2 {
			return fmt.Errorf("usage: aish usage task ID")
		}
		id, e := resolveAgentID(args[1])
		if e != nil {
			id = args[1]
		}
		return printUsageSummary("AISH usage — task "+id, usagepkg.Filter(xs, id, ""))
	case "reset":
		if !executor.Confirm("clear usage records", "Delete AISH usage metadata only", false) {
			fmt.Println("Cancelled.")
			return nil
		}
		return usagepkg.Reset()
	case "export":
		format, dest := "json", ""
		for i := 1; i < len(args); i++ {
			if args[i] == "--format" && i+1 < len(args) {
				format = args[i+1]
				i++
			} else if args[i] == "--output" && i+1 < len(args) {
				dest = args[i+1]
				i++
			}
		}
		if err := usagepkg.Export(format, dest, xs); err != nil {
			return err
		}
		if dest == "" {
			dest = "aish-usage." + format
		}
		fmt.Println("exported usage to", dest)
		return nil
	default:
		return fmt.Errorf("usage: aish usage [today|session NAME|task ID|export --format json|csv [--output FILE]|reset]")
	}
}

func pricingCommand(cfg config.Config, args []string) error {
	pc := cfg.Providers[cfg.ActiveProvider]
	if len(args) == 0 || args[0] == "show" {
		fmt.Printf("Provider: %s\nModel: %s\nInput: $%.6f per 1M tokens\nOutput: $%.6f per 1M tokens\n", cfg.ActiveProvider, pc.Model, pc.InputCostPerMillion, pc.OutputCostPerMillion)
		return nil
	}
	if args[0] == "set" && len(args) == 3 {
		v, err := strconv.ParseFloat(args[2], 64)
		if err != nil {
			return err
		}
		switch args[1] {
		case "input":
			pc.InputCostPerMillion = v
		case "output":
			pc.OutputCostPerMillion = v
		default:
			return fmt.Errorf("usage: aish pricing set [input|output] COST_PER_MILLION")
		}
		cfg.Providers[cfg.ActiveProvider] = pc
		return config.Save(cfg)
	}
	return fmt.Errorf("usage: aish pricing [show|set input COST|set output COST]")
}
