package executor

import "testing"

func TestValidateRejectsUnclosedBacktick(t *testing.T) {
	if err := Validate("ls -l` | head -n 10"); err == nil {
		t.Fatal("expected malformed backtick to be rejected")
	}
}

func TestValidateAcceptsPipeline(t *testing.T) {
	if err := Validate("du -ah . | sort -hr | head -n 10"); err != nil {
		t.Fatalf("expected valid pipeline, got %v", err)
	}
}

func TestValidateRejectsMalformedSortKey(t *testing.T) {
	if err := Validate("ls -l | sort -k 1, -n | head -n 10"); err == nil {
		t.Fatal("expected malformed sort key to be rejected")
	}
}
