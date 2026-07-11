package executor

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"
)

var blocked = []string{"rm -rf /", "mkfs", "format c:", "shutdown", "reboot", ":(){:|:&};:"}

func Validate(command string) error {
	n := strings.ToLower(strings.TrimSpace(command))
	for _, x := range blocked {
		if strings.Contains(n, x) {
			return fmt.Errorf("blocked potentially destructive command")
		}
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
