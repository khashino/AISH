package project

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

func Context(dir string) string {
	abs, _ := filepath.Abs(dir)
	var b strings.Builder
	fmt.Fprintf(&b, "Working directory: %s\n", abs)
	entries, _ := os.ReadDir(abs)
	var names []string
	for _, e := range entries {
		names = append(names, e.Name())
	}
	sort.Strings(names)
	if len(names) > 40 {
		names = names[:40]
	}
	fmt.Fprintf(&b, "Top-level files: %s\n", strings.Join(names, ", "))
	if out, err := exec.Command("git", "-C", abs, "rev-parse", "--show-toplevel").Output(); err == nil {
		fmt.Fprintf(&b, "Git repository: %s", string(out))
		if s, err := exec.Command("git", "-C", abs, "status", "--short").Output(); err == nil {
			txt := strings.TrimSpace(string(s))
			if txt == "" {
				txt = "clean"
			}
			fmt.Fprintf(&b, "Git status:\n%s\n", txt)
		}
		if br, err := exec.Command("git", "-C", abs, "branch", "--show-current").Output(); err == nil {
			fmt.Fprintf(&b, "Git branch: %s", string(br))
		}
	}
	for _, f := range []string{"README.md", "go.mod", "package.json", "pyproject.toml", "Cargo.toml", "Dockerfile"} {
		p := filepath.Join(abs, f)
		if data, err := os.ReadFile(p); err == nil {
			if len(data) > 3000 {
				data = data[:3000]
			}
			fmt.Fprintf(&b, "\n--- %s ---\n%s\n", f, string(data))
		}
	}
	return b.String()
}
