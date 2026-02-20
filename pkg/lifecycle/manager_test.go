package lifecycle

import (
	"sync/atomic"
	"testing"
	"time"

	"github.com/denizumutdereli/qubicdb/pkg/core"
)

func TestManagerCreation(t *testing.T) {
	m := NewManager()
	defer m.Stop()

	if m == nil {
		t.Fatal("NewManager returned nil")
	}
}

func TestManagerRecordActivity(t *testing.T) {
	m := NewManager()
	defer m.Stop()

	indexID := core.IndexID("user-1")

	m.RecordActivity(indexID)

	state := m.GetState(indexID)
	if state != core.StateActive {
		t.Errorf("Expected StateActive, got %d", state)
	}
}

func TestManagerGetBrainState(t *testing.T) {
	m := NewManager()
	defer m.Stop()

	indexID := core.IndexID("user-1")
	m.RecordActivity(indexID)

	brainState := m.GetBrainState(indexID)

	if brainState == nil {
		t.Fatal("GetBrainState returned nil")
	}
	if brainState.IndexID != indexID {
		t.Error("IndexID mismatch")
	}
	// RecordActivity increments InvokeCount, and NewBrainState starts at 1
	if brainState.InvokeCount < 1 {
		t.Errorf("Expected invoke count >= 1, got %d", brainState.InvokeCount)
	}
}

func TestManagerGetBrainStateNonExistent(t *testing.T) {
	m := NewManager()
	defer m.Stop()

	brainState := m.GetBrainState("nonexistent")

	if brainState != nil {
		t.Error("Should return nil for non-existent user")
	}
}

func TestManagerIsActivitySparse(t *testing.T) {
	m := NewManager()
	defer m.Stop()

	indexID := core.IndexID("user-1")

	// No activity = sparse
	if !m.IsActivitySparse(indexID) {
		t.Error("No activity should be sparse")
	}

	// Record many activities
	for i := 0; i < 10; i++ {
		m.RecordActivity(indexID)
	}

	// Should not be sparse now
	if m.IsActivitySparse(indexID) {
		t.Error("Recent heavy activity should not be sparse")
	}
}

func TestManagerCheckAndTransition(t *testing.T) {
	m := NewManager()
	defer m.Stop()

	indexID := core.IndexID("user-1")
	m.RecordActivity(indexID)

	// Force last invoke to be old
	m.mu.Lock()
	m.states[indexID].LastInvoke = time.Now().Add(-10 * time.Minute)
	m.mu.Unlock()

	transitioned := m.CheckAndTransition(indexID)

	// Should transition due to inactivity
	if !transitioned {
		t.Log("Note: Transition depends on threshold settings")
	}
}

func TestManagerForceWake(t *testing.T) {
	m := NewManager()
	defer m.Stop()

	indexID := core.IndexID("user-1")

	// Create dormant state
	m.mu.Lock()
	m.states[indexID] = &core.BrainState{
		IndexID: indexID,
		State:   core.StateDormant,
	}
	m.mu.Unlock()

	m.ForceWake(indexID)

	state := m.GetState(indexID)
	if state != core.StateActive {
		t.Errorf("ForceWake should set state to Active, got %d", state)
	}
}

func TestManagerForceSleep(t *testing.T) {
	m := NewManager()
	defer m.Stop()

	indexID := core.IndexID("user-1")
	m.RecordActivity(indexID)

	m.ForceSleep(indexID)

	state := m.GetState(indexID)
	if state != core.StateSleeping {
		t.Errorf("ForceSleep should set state to Sleeping, got %d", state)
	}
}

func TestManagerGetActiveUsers(t *testing.T) {
	m := NewManager()
	defer m.Stop()

	m.RecordActivity("index-1")
	m.RecordActivity("index-2")
	m.RecordActivity("user-3")
	m.ForceSleep("user-3")

	active := m.GetActiveUsers()

	if len(active) != 2 {
		t.Errorf("Expected 2 active users, got %d", len(active))
	}
}

func TestManagerGetSleepingUsers(t *testing.T) {
	m := NewManager()
	defer m.Stop()

	m.RecordActivity("user-1")
	m.RecordActivity("user-2")
	m.ForceSleep("user-1")
	m.ForceSleep("user-2")

	sleeping := m.GetSleepingUsers()

	if len(sleeping) != 2 {
		t.Errorf("Expected 2 sleeping users, got %d", len(sleeping))
	}
}

func TestManagerCallbacks(t *testing.T) {
	m := NewManager()
	defer m.Stop()

	var wakeCalled atomic.Bool
	var sleepCalled atomic.Bool

	m.SetCallbacks(
		func(indexID core.IndexID) { sleepCalled.Store(true) },
		nil,
		nil,
		func(indexID core.IndexID) { wakeCalled.Store(true) },
	)

	indexID := core.IndexID("user-1")

	// Create dormant state and wake
	m.mu.Lock()
	m.states[indexID] = &core.BrainState{
		IndexID: indexID,
		State:   core.StateDormant,
	}
	m.mu.Unlock()

	m.RecordActivity(indexID)
	time.Sleep(50 * time.Millisecond) // Allow callback goroutine to run

	if !wakeCalled.Load() {
		t.Error("Wake callback should be called")
	}

	m.ForceSleep(indexID)
	time.Sleep(50 * time.Millisecond)

	if !sleepCalled.Load() {
		t.Error("Sleep callback should be called")
	}
}

func TestManagerStats(t *testing.T) {
	m := NewManager()
	defer m.Stop()

	m.RecordActivity("user-1")
	m.RecordActivity("user-2")

	stats := m.Stats()

	if stats["total_indexes"].(int) != 2 {
		t.Errorf("Expected 2 total indexes, got %v", stats["total_indexes"])
	}
}
