package core

import (
	"strings"
	"testing"
)

func TestValidateNeuronContent_TooLarge(t *testing.T) {
	tooLarge := strings.Repeat("a", DefaultMaxNeuronContentBytes+1)
	err := ValidateNeuronContent(tooLarge)
	if err == nil {
		t.Fatal("expected oversized content to fail")
	}
	if !strings.Contains(err.Error(), ErrContentTooLarge.Error()) {
		t.Fatalf("expected ErrContentTooLarge, got: %v", err)
	}
}

func TestValidateNeuronContent_ValidAtBoundary(t *testing.T) {
	boundary := strings.Repeat("a", DefaultMaxNeuronContentBytes)
	if err := ValidateNeuronContent(boundary); err != nil {
		t.Fatalf("expected boundary-sized content to pass, got: %v", err)
	}
}

func TestSetMaxNeuronContentBytes_AppliesRuntimeLimit(t *testing.T) {
	if err := SetMaxNeuronContentBytes(DefaultMaxNeuronContentBytes); err != nil {
		t.Fatalf("failed to reset default limit: %v", err)
	}
	t.Cleanup(func() {
		_ = SetMaxNeuronContentBytes(DefaultMaxNeuronContentBytes)
	})

	if err := SetMaxNeuronContentBytes(8); err != nil {
		t.Fatalf("SetMaxNeuronContentBytes failed: %v", err)
	}

	if got := GetMaxNeuronContentBytes(); got != 8 {
		t.Fatalf("expected runtime limit 8, got %d", got)
	}

	if err := ValidateNeuronContent("12345678"); err != nil {
		t.Fatalf("expected content at runtime boundary to pass: %v", err)
	}
	if err := ValidateNeuronContent("123456789"); err == nil {
		t.Fatal("expected content above runtime boundary to fail")
	}
}

func TestSetMaxNeuronContentBytes_RejectsNonPositive(t *testing.T) {
	if err := SetMaxNeuronContentBytes(0); err == nil {
		t.Fatal("expected zero limit to fail")
	}
	if err := SetMaxNeuronContentBytes(-1); err == nil {
		t.Fatal("expected negative limit to fail")
	}
}
