package core

import (
	"testing"
	"time"
)

func TestNewNeuronID(t *testing.T) {
	id1 := NewNeuronID()
	id2 := NewNeuronID()

	if id1 == "" {
		t.Error("NewNeuronID should not return empty string")
	}
	if id1 == id2 {
		t.Error("NewNeuronID should return unique IDs")
	}
}

func TestNewSynapseID(t *testing.T) {
	from := NeuronID("neuron-1")
	to := NeuronID("neuron-2")

	id := NewSynapseID(from, to)

	if id == "" {
		t.Error("NewSynapseID should not return empty string")
	}
	if string(id) != "neuron-1:neuron-2" {
		t.Errorf("Expected 'neuron-1:neuron-2', got '%s'", id)
	}
}

func TestNewNeuron(t *testing.T) {
	content := "Test neuron content"
	dim := 5

	n := NewNeuron(content, dim)

	if n.ID == "" {
		t.Error("Neuron should have an ID")
	}
	if n.Content != content {
		t.Errorf("Expected content '%s', got '%s'", content, n.Content)
	}
	if len(n.Position) != dim {
		t.Errorf("Expected position dimension %d, got %d", dim, len(n.Position))
	}
	if n.Energy != 1.0 {
		t.Errorf("New neuron should have energy 1.0, got %f", n.Energy)
	}
	if n.Depth != 0 {
		t.Errorf("New neuron should have depth 0, got %d", n.Depth)
	}
	if n.AccessCount != 1 {
		t.Errorf("New neuron should have access count 1, got %d", n.AccessCount)
	}
}

func TestNeuronFire(t *testing.T) {
	n := NewNeuron("Test", 3)
	n.Energy = 0.5
	initialAccessCount := n.AccessCount

	n.Fire()

	if n.Energy != 0.8 { // 0.5 + 0.3
		t.Errorf("Expected energy 0.8 after fire, got %f", n.Energy)
	}
	if n.AccessCount != initialAccessCount+1 {
		t.Error("Fire should increment access count")
	}
}

func TestNeuronFireCapped(t *testing.T) {
	n := NewNeuron("Test", 3)
	n.Energy = 0.9

	n.Fire()

	if n.Energy != 1.0 { // Should be capped at 1.0
		t.Errorf("Energy should be capped at 1.0, got %f", n.Energy)
	}
}

func TestNeuronDecay(t *testing.T) {
	n := NewNeuron("Test", 3)
	n.Energy = 1.0
	n.LastFiredAt = time.Now().Add(-1 * time.Hour) // 1 hour ago

	n.Decay(0.1) // 0.1 rate per hour

	if n.Energy >= 1.0 {
		t.Error("Energy should decrease after decay")
	}
	if n.Energy < n.BaseEnergy {
		t.Error("Energy should not go below base energy")
	}
}

func TestNeuronIsAlive(t *testing.T) {
	n := NewNeuron("Test", 3)

	if !n.IsAlive() {
		t.Error("New neuron should be alive")
	}

	n.Energy = 0.001

	if n.IsAlive() {
		t.Error("Neuron with energy < 0.01 should not be alive")
	}
}

func TestNeuronShouldConsolidate(t *testing.T) {
	n := NewNeuron("Test", 3)
	n.AccessCount = 5
	n.CreatedAt = time.Now().Add(-10 * time.Minute)

	// Not enough access count
	if n.ShouldConsolidate(10, 30*time.Minute) {
		t.Error("Should not consolidate with access count < threshold")
	}

	n.AccessCount = 15
	// Not old enough
	if n.ShouldConsolidate(10, 30*time.Minute) {
		t.Error("Should not consolidate if not old enough")
	}

	n.CreatedAt = time.Now().Add(-1 * time.Hour)
	// Energy still high — should not consolidate yet
	if n.ShouldConsolidate(10, 30*time.Minute) {
		t.Error("Should not consolidate when energy is still high (>= 0.5)")
	}

	n.Energy = 0.3 // Simulate decayed energy
	// Should consolidate now: access count OK, age OK, energy low
	if !n.ShouldConsolidate(10, 30*time.Minute) {
		t.Error("Should consolidate when access count, age, and low energy conditions are all met")
	}
}

func TestNewSynapse(t *testing.T) {
	from := NeuronID("n1")
	to := NeuronID("n2")
	weight := 0.5

	s := NewSynapse(from, to, weight)

	if s.FromID != from {
		t.Error("FromID mismatch")
	}
	if s.ToID != to {
		t.Error("ToID mismatch")
	}
	if s.Weight != weight {
		t.Errorf("Expected weight %f, got %f", weight, s.Weight)
	}
	if s.CoFireCount != 1 {
		t.Error("New synapse should have co-fire count 1")
	}
}

func TestSynapseStrengthen(t *testing.T) {
	s := NewSynapse("n1", "n2", 0.3)
	initialCoFire := s.CoFireCount

	s.Strengthen(0.2)

	if s.Weight != 0.5 {
		t.Errorf("Expected weight 0.5, got %f", s.Weight)
	}
	if s.CoFireCount != initialCoFire+1 {
		t.Error("Strengthen should increment co-fire count")
	}
}

func TestSynapseStrengthenCapped(t *testing.T) {
	s := NewSynapse("n1", "n2", 0.9)

	s.Strengthen(0.5)

	if s.Weight != 1.0 {
		t.Errorf("Weight should be capped at 1.0, got %f", s.Weight)
	}
}

func TestSynapseWeaken(t *testing.T) {
	s := NewSynapse("n1", "n2", 0.5)

	s.Weaken(0.2)

	if s.Weight != 0.3 {
		t.Errorf("Expected weight 0.3, got %f", s.Weight)
	}
}

func TestSynapseWeakenFloor(t *testing.T) {
	s := NewSynapse("n1", "n2", 0.1)

	s.Weaken(0.5)

	if s.Weight != 0.0 {
		t.Errorf("Weight should not go below 0, got %f", s.Weight)
	}
}

func TestSynapseDecay(t *testing.T) {
	s := NewSynapse("n1", "n2", 0.5)
	s.LastCoFire = time.Now().Add(-1 * time.Hour)

	s.Decay(0.1)

	if s.Weight >= 0.5 {
		t.Error("Weight should decrease after decay")
	}
}

func TestSynapseIsAlive(t *testing.T) {
	s := NewSynapse("n1", "n2", 0.5)

	if !s.IsAlive() {
		t.Error("Synapse with weight 0.5 should be alive")
	}

	s.Weight = 0.01

	if s.IsAlive() {
		t.Error("Synapse with weight < 0.05 should not be alive")
	}
}

func TestNewMatrix(t *testing.T) {
	indexID := IndexID("user-1")
	bounds := DefaultBounds()

	m := NewMatrix(indexID, bounds)

	if m.IndexID != indexID {
		t.Error("IndexID mismatch")
	}
	if m.CurrentDim != bounds.MinDimension {
		t.Errorf("Expected dimension %d, got %d", bounds.MinDimension, m.CurrentDim)
	}
	if len(m.Neurons) != 0 {
		t.Error("New matrix should have no neurons")
	}
	if len(m.Synapses) != 0 {
		t.Error("New matrix should have no synapses")
	}
	if m.Version != 1 {
		t.Error("New matrix should have version 1")
	}
}

func TestDefaultBounds(t *testing.T) {
	bounds := DefaultBounds()

	if bounds.MinDimension < 1 {
		t.Error("MinDimension should be >= 1")
	}
	if bounds.MaxDimension < bounds.MinDimension {
		t.Error("MaxDimension should be >= MinDimension")
	}
	if bounds.MaxNeurons < 1 {
		t.Error("MaxNeurons should be >= 1")
	}
}

func TestNewBrainState(t *testing.T) {
	indexID := IndexID("user-1")

	state := NewBrainState(indexID)

	if state.IndexID != indexID {
		t.Error("IndexID mismatch")
	}
	if state.State != StateActive {
		t.Error("New brain state should be Active")
	}
	if state.InvokeCount != 1 {
		t.Error("New brain state should have invoke count 1")
	}
}

func TestMatrixLocking(t *testing.T) {
	m := NewMatrix("user-1", DefaultBounds())

	// Test that write-locking allows mutation without panic
	m.Lock()
	m.CurrentDim = 5
	m.Unlock()

	// Test that read-locking allows reads without panic
	m.RLock()
	_ = m.CurrentDim
	m.RUnlock()
}

func TestNeuronLocking(t *testing.T) {
	n := NewNeuron("Test", 3)

	// Test that write-locking allows mutation without panic
	n.Lock()
	n.Energy = 0.5
	n.Unlock()

	// Test that read-locking allows reads without panic
	n.RLock()
	_ = n.Energy
	n.RUnlock()
}

func TestHashContent(t *testing.T) {
	// Same content should produce same hash
	hash1 := HashContent("test content")
	hash2 := HashContent("test content")
	if hash1 != hash2 {
		t.Error("Same content should produce same hash")
	}

	// Different content should produce different hash
	hash3 := HashContent("different content")
	if hash1 == hash3 {
		t.Error("Different content should produce different hash")
	}

	// Empty content should still produce hash
	hash4 := HashContent("")
	if hash4 == "" {
		t.Error("Empty content should still produce a hash")
	}
}

func TestTimeSince(t *testing.T) {
	past := time.Now().Add(-1 * time.Hour)
	duration := TimeSince(past)

	if duration < 59*time.Minute {
		t.Error("TimeSince should return approximately 1 hour")
	}
}

func TestNeuronPosition(t *testing.T) {
	n := NewNeuron("Test", 5)

	if len(n.Position) != 5 {
		t.Errorf("Expected 5 dimensions, got %d", len(n.Position))
	}

	// All positions should be within [-1, 1]
	for i, p := range n.Position {
		if p < -1 || p > 1 {
			t.Errorf("Position[%d] = %f, should be in [-1, 1]", i, p)
		}
	}
}

func TestNeuronMetadata(t *testing.T) {
	n := NewNeuron("Test", 3)

	if n.Metadata == nil {
		t.Error("Metadata should be initialized")
	}

	n.Metadata["key"] = "value"
	if n.Metadata["key"] != "value" {
		t.Error("Metadata should be writable")
	}
}

func TestNeuronTags(t *testing.T) {
	n := NewNeuron("Test", 3)

	if n.Tags == nil {
		t.Error("Tags should be initialized")
	}

	n.Tags = append(n.Tags, "tag1", "tag2")
	if len(n.Tags) != 2 {
		t.Errorf("Expected 2 tags, got %d", len(n.Tags))
	}
}

func TestMatrixAddNeuron(t *testing.T) {
	m := NewMatrix("user-1", DefaultBounds())
	n := NewNeuron("Test content", m.CurrentDim)

	m.Neurons[n.ID] = n
	m.Adjacency[n.ID] = []NeuronID{}

	if len(m.Neurons) != 1 {
		t.Error("Matrix should have 1 neuron")
	}
}

func TestMatrixAddSynapse(t *testing.T) {
	m := NewMatrix("user-1", DefaultBounds())
	n1 := NewNeuron("Content 1", m.CurrentDim)
	n2 := NewNeuron("Content 2", m.CurrentDim)

	m.Neurons[n1.ID] = n1
	m.Neurons[n2.ID] = n2

	s := NewSynapse(n1.ID, n2.ID, 0.5)
	m.Synapses[s.ID] = s

	if len(m.Synapses) != 1 {
		t.Error("Matrix should have 1 synapse")
	}
}

func TestBrainStateTransitions(t *testing.T) {
	state := NewBrainState("user-1")

	// Initial state should be Active
	if state.State != StateActive {
		t.Error("Initial state should be Active")
	}

	// Test state transitions
	state.State = StateIdle
	if state.State != StateIdle {
		t.Error("Should be able to transition to Idle")
	}

	state.State = StateSleeping
	if state.State != StateSleeping {
		t.Error("Should be able to transition to Sleeping")
	}

	state.State = StateDormant
	if state.State != StateDormant {
		t.Error("Should be able to transition to Dormant")
	}
}

func TestNeuronDecayMultiple(t *testing.T) {
	n := NewNeuron("Test", 3)
	n.Energy = 1.0
	n.LastFiredAt = time.Now().Add(-2 * time.Hour)

	// First decay
	n.Decay(0.1)
	energy1 := n.Energy

	// Second decay should reduce further
	n.LastFiredAt = time.Now().Add(-2 * time.Hour)
	n.Decay(0.1)
	energy2 := n.Energy

	if energy2 >= energy1 {
		t.Error("Second decay should reduce energy further")
	}
}

func TestSynapseDecayMultiple(t *testing.T) {
	s := NewSynapse("n1", "n2", 1.0)
	s.LastCoFire = time.Now().Add(-2 * time.Hour)

	s.Decay(0.1)
	weight1 := s.Weight

	s.LastCoFire = time.Now().Add(-2 * time.Hour)
	s.Decay(0.1)
	weight2 := s.Weight

	if weight2 >= weight1 {
		t.Error("Second decay should reduce weight further")
	}
}

func TestNeuronIDUniqueness(t *testing.T) {
	ids := make(map[NeuronID]bool)
	for i := 0; i < 1000; i++ {
		id := NewNeuronID()
		if ids[id] {
			t.Errorf("Duplicate ID generated: %s", id)
		}
		ids[id] = true
	}
}

func TestMatrixBoundsValidation(t *testing.T) {
	bounds := MatrixBounds{
		MinDimension: 3,
		MaxDimension: 100,
		MinNeurons:   0,
		MaxNeurons:   10000,
	}

	m := NewMatrix("user-1", bounds)

	if m.CurrentDim < bounds.MinDimension {
		t.Error("Matrix should start at MinDimension")
	}
}
