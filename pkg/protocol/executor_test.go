package protocol

import (
	"sort"
	"testing"

	"github.com/denizumutdereli/qubicdb/pkg/concurrency"
)

// ---------------------------------------------------------------------------
// Command registry pattern tests
// ---------------------------------------------------------------------------

func TestExecutorBuiltinCommands(t *testing.T) {
	e := NewExecutor()
	cmds := e.ListCommands()

	expected := []CommandType{
		CmdInsert, CmdFind, CmdFindOne,
		CmdUpdate, CmdUpdateOne,
		CmdDelete, CmdDeleteOne,
		CmdCount, CmdActivate, CmdSearch, CmdStats,
	}

	if len(cmds) != len(expected) {
		t.Fatalf("expected %d built-in commands, got %d", len(expected), len(cmds))
	}

	// Sort both for stable comparison
	sortCmds := func(s []CommandType) {
		sort.Slice(s, func(i, j int) bool { return s[i] < s[j] })
	}
	sortCmds(cmds)
	sortCmds(expected)

	for i, ct := range expected {
		if cmds[i] != ct {
			t.Errorf("index %d: expected %s, got %s", i, ct, cmds[i])
		}
	}
}

func TestExecutorRegisterCustomCommand(t *testing.T) {
	e := NewExecutor()

	custom := CommandType("ping")
	e.Register(custom, func(_ *concurrency.BrainWorker, _ *Command) *Result {
		return &Result{Success: true, Data: "pong"}
	})

	// Verify it appears in the list
	found := false
	for _, ct := range e.ListCommands() {
		if ct == custom {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("custom command 'ping' not found in ListCommands")
	}

	// Execute the custom command
	result := e.Execute(nil, &Command{Type: custom})
	if !result.Success {
		t.Fatal("expected success for custom command")
	}
	if result.Data != "pong" {
		t.Errorf("expected 'pong', got %v", result.Data)
	}
}

func TestExecutorUnregister(t *testing.T) {
	e := NewExecutor()

	before := len(e.ListCommands())
	e.Unregister(CmdStats)
	after := len(e.ListCommands())

	if after != before-1 {
		t.Fatalf("expected %d commands after unregister, got %d", before-1, after)
	}

	// Executing the removed command should fail gracefully
	result := e.Execute(nil, &Command{Type: CmdStats})
	if result.Success {
		t.Fatal("expected failure for unregistered command")
	}
	if result.Error == "" {
		t.Fatal("expected error message for unregistered command")
	}
}

func TestExecutorUnknownCommand(t *testing.T) {
	e := NewExecutor()
	result := e.Execute(nil, &Command{Type: "nonexistent"})

	if result.Success {
		t.Fatal("expected failure for unknown command")
	}
	if result.Error != "unknown command type: nonexistent" {
		t.Errorf("unexpected error: %s", result.Error)
	}
}

func TestExecutorReplaceBuiltin(t *testing.T) {
	e := NewExecutor()

	// Override the built-in stats handler
	e.Register(CmdStats, func(_ *concurrency.BrainWorker, _ *Command) *Result {
		return &Result{Success: true, Data: "custom-stats"}
	})

	result := e.Execute(nil, &Command{Type: CmdStats})
	if !result.Success {
		t.Fatal("expected success for overridden command")
	}
	if result.Data != "custom-stats" {
		t.Errorf("expected 'custom-stats', got %v", result.Data)
	}
}
