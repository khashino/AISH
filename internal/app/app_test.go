package app

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseProposalJSON(t *testing.T) {
	p, err := parseProposal(`{"command":"ls -la","reason":"list files","dangerous":false}`, "list files")
	if err != nil || p.Command != "ls -la" {
		t.Fatalf("unexpected result: %#v, %v", p, err)
	}
}

func TestParseProposalPlainCommand(t *testing.T) {
	p, err := parseProposal("ls -l | head -10", "show files")
	if err != nil || p.Command != "ls -l | head -10" {
		t.Fatalf("unexpected result: %#v, %v", p, err)
	}
}

func TestParseProposalFencedCommand(t *testing.T) {
	p, err := parseProposal("```sh\ndu -ah . | sort -hr | head -n 10\n```", "largest files")
	if err != nil || p.Command != "du -ah . | sort -hr | head -n 10" {
		t.Fatalf("unexpected result: %#v, %v", p, err)
	}
}

func TestParseProposalMalformedBacktickStillParsesForValidation(t *testing.T) {
	p, err := parseProposal("ls -l` | head -n 10", "show files")
	if err != nil || p.Command == "" {
		t.Fatalf("unexpected parse result: %#v, %v", p, err)
	}
}

func TestDeterministicProposalLargestFiles(t *testing.T) {
	p, ok := deterministicProposal("show the ten largest files in this folder")
	if !ok {
		t.Fatal("expected deterministic proposal")
	}
	if p.Command == "" {
		t.Fatal("expected command")
	}
}

func TestDeterministicGoAgentPlan(t *testing.T) {
	d := t.TempDir()
	if err := os.WriteFile(filepath.Join(d, "go.mod"), []byte("module example.com/test\n\ngo 1.22\n"), 0600); err != nil {
		t.Fatal(err)
	}
	steps, ok := deterministicAgentPlan("inspect this Go project, run the tests, and summarize any failures", d)
	if !ok {
		t.Fatal("expected deterministic Go plan")
	}
	want := []string{"go version", "go env GOMOD", "git status --short", "go test ./...", "go vet ./..."}
	if len(steps) != len(want) {
		t.Fatalf("got %d steps, want %d", len(steps), len(want))
	}
	for i := range want {
		if steps[i].Command != want[i] {
			t.Fatalf("step %d command = %q, want %q", i, steps[i].Command, want[i])
		}
	}
}

func TestValidateAgentPlanRejectsExistingGoModInit(t *testing.T) {
	d := t.TempDir()
	if err := os.WriteFile(filepath.Join(d, "go.mod"), []byte("module example.com/test\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := validateAgentPlanStep("go mod init example.com/other", d); err == nil {
		t.Fatal("expected go mod init to be rejected when go.mod exists")
	}
}
