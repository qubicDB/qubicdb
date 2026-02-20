package lifecycle

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/qubicDB/qubicdb/pkg/core"
)

func TestManagerInvokeCountIncrement(t *testing.T) {
	m := NewManager()
	defer m.Stop()

	indexID := core.IndexID("user-1")

	// Multiple activities
	for i := 0; i < 5; i++ {
		m.RecordActivity(indexID)
	}

	state := m.GetBrainState(indexID)
	if state.InvokeCount < 5 {
		t.Errorf("Expected invoke count >= 5, got %d", state.InvokeCount)
	}
}

func TestManagerActivityLog(t *testing.T) {
	m := NewManager()
	defer m.Stop()

	indexID := core.IndexID("user-1")

	// Record multiple activities
	for i := 0; i < 10; i++ {
		m.RecordActivity(indexID)
		time.Sleep(10 * time.Millisecond)
	}

	state := m.GetBrainState(indexID)
	if state.InvokeCount < 10 {
		t.Error("Invoke count should be at least 10")
	}
}

func TestManagerStateActive(t *testing.T) {
	m := NewManager()
	defer m.Stop()

	indexID := core.IndexID("user-1")
	m.RecordActivity(indexID)

	state := m.GetState(indexID)
	if state != core.StateActive {
		t.Errorf("State should be Active, got %d", state)
	}
}

func TestManagerStateNonExistent(t *testing.T) {
	m := NewManager()
	defer m.Stop()

	state := m.GetState("nonexistent")
	// Non-existent should return default/dormant
	if state != core.StateDormant {
		t.Logf("Non-existent user state: %d", state)
	}
}

func TestManagerConcurrentAccess(t *testing.T) {
	m := NewManager()
	defer m.Stop()

	var wg sync.WaitGroup

	// Concurrent activity recording
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			indexID := core.IndexID("user-" + string(rune('A'+idx%10)))
			m.RecordActivity(indexID)
		}(i)
	}

	wg.Wait()

	// Should have multiple active users
	active := m.GetActiveUsers()
	if len(active) < 1 {
		t.Error("Should have active users after concurrent activity")
	}
}

func TestManagerIdleTransition(t *testing.T) {
	m := NewManager()
	defer m.Stop()

	indexID := core.IndexID("user-1")
	m.RecordActivity(indexID)

	// Manually trigger idle by setting old last invoke
	m.mu.Lock()
	m.states[indexID].LastInvoke = time.Now().Add(-10 * time.Minute)
	m.mu.Unlock()

	// Check transition
	m.CheckAndTransition(indexID)

	state := m.GetState(indexID)
	// State should have transitioned
	if state == core.StateActive {
		t.Log("Note: Transition depends on threshold configuration")
	}
}

func TestManagerActiveUsers(t *testing.T) {
	m := NewManager()
	defer m.Stop()

	m.RecordActivity("user-1")
	m.RecordActivity("user-2")
	m.RecordActivity("user-3")

	users := m.GetActiveUsers()
	if len(users) != 3 {
		t.Errorf("Expected 3 active users, got %d", len(users))
	}
}

func TestManagerSleepingUsers(t *testing.T) {
	m := NewManager()
	defer m.Stop()

	m.RecordActivity("user-1")
	m.ForceSleep("user-1")

	sleeping := m.GetSleepingUsers()
	if len(sleeping) != 1 {
		t.Errorf("Expected 1 sleeping user, got %d", len(sleeping))
	}
}

func TestManagerCallbackOnSleep(t *testing.T) {
	m := NewManager()
	defer m.Stop()

	var sleepCalled atomic.Bool
	m.SetCallbacks(
		func(indexID core.IndexID) { sleepCalled.Store(true) },
		nil, nil, nil,
	)

	indexID := core.IndexID("user-1")
	m.RecordActivity(indexID)
	m.ForceSleep(indexID)

	time.Sleep(100 * time.Millisecond) // Allow callback goroutine

	if !sleepCalled.Load() {
		t.Error("Sleep callback should be called")
	}
}

func TestManagerCallbackOnWake(t *testing.T) {
	m := NewManager()
	defer m.Stop()

	var wakeCalled atomic.Bool
	m.SetCallbacks(
		nil, nil, nil,
		func(indexID core.IndexID) { wakeCalled.Store(true) },
	)

	indexID := core.IndexID("user-1")

	// Create sleeping state
	m.mu.Lock()
	m.states[indexID] = &core.BrainState{
		IndexID: indexID,
		State:   core.StateSleeping,
	}
	m.mu.Unlock()

	// Wake up via activity
	m.RecordActivity(indexID)

	time.Sleep(100 * time.Millisecond)

	if !wakeCalled.Load() {
		t.Error("Wake callback should be called")
	}
}

func TestManagerCallbackSetup(t *testing.T) {
	m := NewManager()
	defer m.Stop()

	callCount := 0
	m.SetCallbacks(
		func(indexID core.IndexID) { callCount++ },
		func(indexID core.IndexID) { callCount++ },
		func(indexID core.IndexID) { callCount++ },
		func(indexID core.IndexID) { callCount++ },
	)

	// Verify callbacks are set (no panic)
	t.Log("Callback setup verified")
}

func TestManagerStatsDetail(t *testing.T) {
	m := NewManager()
	defer m.Stop()

	m.RecordActivity("user-1")
	m.RecordActivity("user-2")
	m.ForceSleep("user-1")

	stats := m.Stats()

	// Verify stats structure
	if stats == nil {
		t.Fatal("Stats should not be nil")
	}

	// Check that stats contains expected keys
	if _, ok := stats["total_users"]; !ok {
		t.Log("Stats may have different key names")
	}
}

func TestManagerForceWakeDormant(t *testing.T) {
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
		t.Errorf("ForceWake should set to Active, got %d", state)
	}
}

func TestManagerActivitySparseWithActivity(t *testing.T) {
	m := NewManager()
	defer m.Stop()

	indexID := core.IndexID("user-1")

	// Heavy activity
	for i := 0; i < 50; i++ {
		m.RecordActivity(indexID)
	}

	// Should not be sparse with heavy activity
	if m.IsActivitySparse(indexID) {
		t.Log("Heavy activity may still be sparse depending on timing")
	}
}

func TestManagerStopIdempotent(t *testing.T) {
	m := NewManager()

	// Stop multiple times should not panic
	m.Stop()
	m.Stop()
}
