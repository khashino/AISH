package executor

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"runtime"
	"strings"
	"time"
)

var blocked = []string{"rm -rf /", "mkfs", "format c:", "shutdown", "reboot", ":(){:|:&};:"}

func Validate(command string) error {
	command = strings.TrimSpace(command)
	if command == "" {
		return fmt.Errorf("empty command")
	}
	if strings.Contains(command, "```") {
		return fmt.Errorf("command contains Markdown fencing")
	}
	if err := validateBalancedQuotes(command); err != nil {
		return err
	}
	n := strings.ToLower(command)
	for _, x := range blocked {
		if strings.Contains(n, x) {
			return fmt.Errorf("blocked potentially destructive command")
		}
	}
	if err := validateKnownCommandArguments(command); err != nil {
		return err
	}
	if runtime.GOOS != "windows" {
		cmd := exec.Command("/bin/sh", "-n", "-c", command)
		if out, err := cmd.CombinedOutput(); err != nil {
			msg := strings.TrimSpace(string(out))
			if msg == "" {
				msg = err.Error()
			}
			return fmt.Errorf("invalid shell syntax: %s", msg)
		}
	}
	return nil
}

var sortKeyPattern = regexp.MustCompile(`(?:^|[[:space:]])-k(?:[[:space:]]+)?([^[:space:]|;&]+)`)
var validSortKeyPattern = regexp.MustCompile(`^[0-9]+(?:\.[0-9]+)?[A-Za-z]*?(?:,[0-9]+(?:\.[0-9]+)?[A-Za-z]*?)?$`)

func validateKnownCommandArguments(command string) error {
	// Shell syntax validation cannot detect malformed arguments such as
	// `sort -k 1, -n`. Validate common generated sort keys explicitly.
	for _, match := range sortKeyPattern.FindAllStringSubmatch(command, -1) {
		if len(match) < 2 {
			continue
		}
		key := strings.TrimSpace(match[1])
		if !validSortKeyPattern.MatchString(key) {
			return fmt.Errorf("invalid sort key %q; use forms such as -k 5,5n or prefer sort -n/-nr for numeric input", key)
		}
	}
	return nil
}

func validateBalancedQuotes(command string) error {
	var single, double, backtick bool
	escaped := false
	for _, r := range command {
		if escaped {
			escaped = false
			continue
		}
		if r == '\\' && !single {
			escaped = true
			continue
		}
		switch r {
		case '\'':
			if !double && !backtick {
				single = !single
			}
		case '"':
			if !single && !backtick {
				double = !double
			}
		case '`':
			if !single {
				backtick = !backtick
			}
		}
	}
	if escaped {
		return fmt.Errorf("command ends with an incomplete escape")
	}
	if single || double || backtick {
		return fmt.Errorf("command contains an unclosed quote or backtick")
	}
	return nil
}
func Confirm(command, reason string, strong bool) bool {
	fmt.Printf("\nProposed command:\n  %s\n", command)
	if reason != "" {
		fmt.Printf("Reason: %s\n", reason)
	}
	if strong {
		fmt.Print("Type EXECUTE to continue: ")
		var a string
		fmt.Scanln(&a)
		return a == "EXECUTE"
	}
	fmt.Print("Run this command? [y/N]: ")
	var a string
	fmt.Scanln(&a)
	return strings.EqualFold(strings.TrimSpace(a), "y")
}
func Capture(command string, timeout time.Duration) (string, error) {
	if e := Validate(command); e != nil {
		return "", e
	}
	ctx, c := context.WithTimeout(context.Background(), timeout)
	defer c()
	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		cmd = exec.CommandContext(ctx, "cmd.exe", "/C", command)
	} else {
		cmd = exec.CommandContext(ctx, "/bin/sh", "-c", command)
	}
	var b bytes.Buffer
	cmd.Stdout = &b
	cmd.Stderr = &b
	cmd.Stdin = os.Stdin
	e := cmd.Run()
	if ctx.Err() == context.DeadlineExceeded {
		return b.String(), fmt.Errorf("command timed out")
	}
	return b.String(), e
}
func Run(command string, confirm bool) error {
	if e := Validate(command); e != nil {
		return e
	}
	if confirm && !Confirm(command, "", false) {
		fmt.Println("Cancelled.")
		return nil
	}
	out, e := Capture(command, 60*time.Second)
	fmt.Print(out)
	return e
}
