package lifecycle

import (
	"context"
	"sync"
	"time"

	"github.com/qubicDB/qubicdb/pkg/core"
)

// Manager tracks activity and controls brain lifecycle states
type Manager struct {
	states map[core.IndexID]*core.BrainState

	// Callbacks
	onSleepStart func(indexID core.IndexID)
	onSleepEnd   func(indexID core.IndexID)
	onDormant    func(indexID core.IndexID)
	onWake       func(indexID core.IndexID)

	// Activity tracking
	activityBuffer map[core.IndexID][]time.Time
	bufferWindow   time.Duration

	// Thresholds
	idleThreshold    time.Duration
	sleepThreshold   time.Duration
	dormantThreshold time.Duration

	// Activity sparseness detection
	sparsenessWindow time.Duration
	sparsenessMinOps int // Minimum ops in window to be "active"

	mu sync.RWMutex

	ctx    context.Context
	cancel context.CancelFunc
}

// RemoveIndex drops lifecycle state for an index.
func (m *Manager) RemoveIndex(indexID core.IndexID) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.states, indexID)
	delete(m.activityBuffer, indexID)
}

// SetThresholds applies lifecycle thresholds at runtime.
func (m *Manager) SetThresholds(idle, sleep, dormant time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.idleThreshold = idle
	m.sleepThreshold = sleep
	m.dormantThreshold = dormant
	for _, state := range m.states {
		state.IdleThreshold = idle
		state.SleepThreshold = sleep
	}
}

// Thresholds returns currently active lifecycle thresholds.
func (m *Manager) Thresholds() (time.Duration, time.Duration, time.Duration) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.idleThreshold, m.sleepThreshold, m.dormantThreshold
}

// NewManager creates a new lifecycle manager
func NewManager() *Manager {
	ctx, cancel := context.WithCancel(context.Background())
	return &Manager{
		states:           make(map[core.IndexID]*core.BrainState),
		activityBuffer:   make(map[core.IndexID][]time.Time),
		bufferWindow:     5 * time.Minute,
		idleThreshold:    30 * time.Second,
		sleepThreshold:   5 * time.Minute,
		dormantThreshold: 30 * time.Minute,
		sparsenessWindow: 30 * time.Second,
		sparsenessMinOps: 3,
		ctx:              ctx,
		cancel:           cancel,
	}
}

// SetCallbacks configures lifecycle transition callbacks
func (m *Manager) SetCallbacks(
	onSleepStart func(core.IndexID),
	onSleepEnd func(core.IndexID),
	onDormant func(core.IndexID),
	onWake func(core.IndexID),
) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.onSleepStart = onSleepStart
	m.onSleepEnd = onSleepEnd
	m.onDormant = onDormant
	m.onWake = onWake
}

// RecordActivity records an index activity event
func (m *Manager) RecordActivity(indexID core.IndexID) {
	m.mu.Lock()
	defer m.mu.Unlock()

	now := time.Now()

	// Get or create state
	state, ok := m.states[indexID]
	if !ok {
		state = core.NewBrainState(indexID)
		m.states[indexID] = state
	}

	// Handle state transition on activity
	oldState := state.State
	if state.State == core.StateDormant || state.State == core.StateSleeping {
		state.State = core.StateActive
		state.SessionStart = now
		if m.onWake != nil {
			go m.onWake(indexID)
		}
	} else if state.State == core.StateIdle {
		state.State = core.StateActive
	}

	state.LastInvoke = now
	state.InvokeCount++

	// Record in activity buffer
	m.activityBuffer[indexID] = append(m.activityBuffer[indexID], now)

	// Clean old entries from buffer
	m.cleanBuffer(indexID)

	_ = oldState // May use for metrics later
}

// cleanBuffer removes old activity entries
func (m *Manager) cleanBuffer(indexID core.IndexID) {
	cutoff := time.Now().Add(-m.bufferWindow)
	buffer := m.activityBuffer[indexID]

	newBuffer := make([]time.Time, 0, len(buffer))
	for _, t := range buffer {
		if t.After(cutoff) {
			newBuffer = append(newBuffer, t)
		}
	}
	m.activityBuffer[indexID] = newBuffer
}

// GetState returns the current state for an index
func (m *Manager) GetState(indexID core.IndexID) core.ActivityState {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if state, ok := m.states[indexID]; ok {
		return state.State
	}
	return core.StateDormant
}

// GetBrainState returns the full brain state
func (m *Manager) GetBrainState(indexID core.IndexID) *core.BrainState {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if state, ok := m.states[indexID]; ok {
		return state
	}
	return nil
}

// IsActivitySparse checks if activity has become sparse (trigger for sleep)
func (m *Manager) IsActivitySparse(indexID core.IndexID) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()

	buffer := m.activityBuffer[indexID]
	if len(buffer) == 0 {
		return true
	}

	// Count activities in the sparseness window
	cutoff := time.Now().Add(-m.sparsenessWindow)
	count := 0
	for _, t := range buffer {
		if t.After(cutoff) {
			count++
		}
	}

	return count < m.sparsenessMinOps
}

// CheckAndTransition evaluates an index state and transitions if needed
// Returns true if a transition occurred
func (m *Manager) CheckAndTransition(indexID core.IndexID) bool {
	m.mu.Lock()
	defer m.mu.Unlock()

	state, ok := m.states[indexID]
	if !ok {
		return false
	}

	now := time.Now()
	elapsed := now.Sub(state.LastInvoke)
	oldState := state.State

	switch state.State {
	case core.StateActive:
		// Check for sparseness-based sleep (organic trigger)
		if m.isActivitySparseUnsafe(indexID) && elapsed > m.idleThreshold {
			state.State = core.StateIdle
		}

	case core.StateIdle:
		if elapsed > m.sleepThreshold {
			state.State = core.StateSleeping
			if m.onSleepStart != nil {
				go m.onSleepStart(indexID)
			}
		}

	case core.StateSleeping:
		if elapsed > m.dormantThreshold {
			state.State = core.StateDormant
			if m.onSleepEnd != nil {
				go m.onSleepEnd(indexID)
			}
			if m.onDormant != nil {
				go m.onDormant(indexID)
			}
		}
	}

	return state.State != oldState
}

func (m *Manager) isActivitySparseUnsafe(indexID core.IndexID) bool {
	buffer := m.activityBuffer[indexID]
	if len(buffer) == 0 {
		return true
	}

	cutoff := time.Now().Add(-m.sparsenessWindow)
	count := 0
	for _, t := range buffer {
		if t.After(cutoff) {
			count++
		}
	}

	return count < m.sparsenessMinOps
}

// StartMonitor starts the background lifecycle monitoring
func (m *Manager) StartMonitor(checkInterval time.Duration) {
	go func() {
		ticker := time.NewTicker(checkInterval)
		defer ticker.Stop()

		for {
			select {
			case <-m.ctx.Done():
				return
			case <-ticker.C:
				m.checkAllUsers()
			}
		}
	}()
}

// checkAllUsers checks all indexes for state transitions
func (m *Manager) checkAllUsers() {
	m.mu.RLock()
	indexIDs := make([]core.IndexID, 0, len(m.states))
	for id := range m.states {
		indexIDs = append(indexIDs, id)
	}
	m.mu.RUnlock()

	for _, id := range indexIDs {
		m.CheckAndTransition(id)
	}
}

// Stop stops the lifecycle manager
func (m *Manager) Stop() {
	m.cancel()
}

// GetActiveUsers returns all indexes in Active or Idle state
func (m *Manager) GetActiveUsers() []core.IndexID {
	m.mu.RLock()
	defer m.mu.RUnlock()

	active := make([]core.IndexID, 0)
	for id, state := range m.states {
		if state.State == core.StateActive || state.State == core.StateIdle {
			active = append(active, id)
		}
	}
	return active
}

// GetSleepingUsers returns all indexes in Sleeping state
func (m *Manager) GetSleepingUsers() []core.IndexID {
	m.mu.RLock()
	defer m.mu.RUnlock()

	sleeping := make([]core.IndexID, 0)
	for id, state := range m.states {
		if state.State == core.StateSleeping {
			sleeping = append(sleeping, id)
		}
	}
	return sleeping
}

// ForceWake forces an index to wake up
func (m *Manager) ForceWake(indexID core.IndexID) {
	m.mu.Lock()
	defer m.mu.Unlock()

	state, ok := m.states[indexID]
	if !ok {
		state = core.NewBrainState(indexID)
		m.states[indexID] = state
	}

	if state.State != core.StateActive {
		state.State = core.StateActive
		state.LastInvoke = time.Now()
		if m.onWake != nil {
			go m.onWake(indexID)
		}
	}
}

// ForceSleep forces an index into sleep state
func (m *Manager) ForceSleep(indexID core.IndexID) {
	m.mu.Lock()
	defer m.mu.Unlock()

	state, ok := m.states[indexID]
	if !ok {
		return
	}

	if state.State != core.StateSleeping {
		state.State = core.StateSleeping
		if m.onSleepStart != nil {
			go m.onSleepStart(indexID)
		}
	}
}

// Stats returns manager statistics
func (m *Manager) Stats() map[string]any {
	m.mu.RLock()
	defer m.mu.RUnlock()

	stateCounts := map[string]int{
		"active":   0,
		"idle":     0,
		"sleeping": 0,
		"dormant":  0,
	}

	for _, state := range m.states {
		switch state.State {
		case core.StateActive:
			stateCounts["active"]++
		case core.StateIdle:
			stateCounts["idle"]++
		case core.StateSleeping:
			stateCounts["sleeping"]++
		case core.StateDormant:
			stateCounts["dormant"]++
		}
	}

	return map[string]any{
		"total_indexes":      len(m.states),
		"state_distribution": stateCounts,
		"idle_threshold":     m.idleThreshold.String(),
		"sleep_threshold":    m.sleepThreshold.String(),
		"dormant_threshold":  m.dormantThreshold.String(),
	}
}
