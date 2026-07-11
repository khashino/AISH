package app

import "testing"

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
