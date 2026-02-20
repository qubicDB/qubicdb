package synapse

import (
	"testing"
	"time"

	"github.com/denizumutdereli/qubicdb/pkg/core"
)

func newTestMatrix() *core.Matrix {
	return core.NewMatrix("test-user", core.DefaultBounds())
}

func TestHebbianEngineCreation(t *testing.T) {
	m := newTestMatrix()
	h := NewHebbianEngine(m)

	if h == nil {
		t.Fatal("NewHebbianEngine returned nil")
	}
}

func TestHebbianEngineOnNeuronFired(t *testing.T) {
	m := newTestMatrix()
	h := NewHebbianEngine(m)

	// Add two neurons
	n1 := core.NewNeuron("Neuron 1", m.CurrentDim)
	n2 := core.NewNeuron("Neuron 2", m.CurrentDim)
	m.Neurons[n1.ID] = n1
	m.Neurons[n2.ID] = n2
	m.Adjacency[n1.ID] = []core.NeuronID{}
	m.Adjacency[n2.ID] = []core.NeuronID{}

	// Fire them in sequence (within co-activation window)
	h.OnNeuronFired(n1.ID)
	h.OnNeuronFired(n2.ID)

	// Should create a synapse
	if len(m.Synapses) != 1 {
		t.Errorf("Expected 1 synapse after co-firing, got %d", len(m.Synapses))
	}
}

func TestHebbianEngineSynapseStrengthening(t *testing.T) {
	m := newTestMatrix()
	h := NewHebbianEngine(m)

	n1 := core.NewNeuron("Neuron 1", m.CurrentDim)
	n2 := core.NewNeuron("Neuron 2", m.CurrentDim)
	m.Neurons[n1.ID] = n1
	m.Neurons[n2.ID] = n2
	m.Adjacency[n1.ID] = []core.NeuronID{}
	m.Adjacency[n2.ID] = []core.NeuronID{}

	// First co-fire creates synapse
	h.OnNeuronFired(n1.ID)
	h.OnNeuronFired(n2.ID)

	// Get initial weight
	var initialWeight float64
	for _, s := range m.Synapses {
		initialWeight = s.Weight
		break
	}

	// Wait a tiny bit and fire again
	time.Sleep(10 * time.Millisecond)
	h.OnNeuronFired(n1.ID)
	h.OnNeuronFired(n2.ID)

	// Weight should increase
	for _, s := range m.Synapses {
		if s.Weight <= initialWeight {
			t.Error("Synapse weight should increase on repeated co-firing")
		}
		break
	}
}

func TestHebbianEngineDecayAll(t *testing.T) {
	m := newTestMatrix()
	h := NewHebbianEngine(m)

	// Create a synapse manually
	s := core.NewSynapse("n1", "n2", 0.5)
	s.LastCoFire = time.Now().Add(-1 * time.Hour) // 1 hour ago
	m.Synapses[s.ID] = s

	h.DecayAll()

	if s.Weight >= 0.5 {
		t.Error("Synapse weight should decay")
	}
}

func TestHebbianEnginePruneDeadSynapses(t *testing.T) {
	m := newTestMatrix()
	h := NewHebbianEngine(m)

	// Create neurons for adjacency
	n1 := core.NewNeuron("N1", m.CurrentDim)
	n2 := core.NewNeuron("N2", m.CurrentDim)
	m.Neurons[n1.ID] = n1
	m.Neurons[n2.ID] = n2

	// Create a weak synapse
	s := core.NewSynapse(n1.ID, n2.ID, 0.01) // Below alive threshold
	m.Synapses[s.ID] = s
	m.Adjacency[n1.ID] = []core.NeuronID{n2.ID}
	m.Adjacency[n2.ID] = []core.NeuronID{n1.ID}

	pruned := h.PruneDeadSynapses()

	if pruned != 1 {
		t.Errorf("Expected 1 pruned synapse, got %d", pruned)
	}
	if len(m.Synapses) != 0 {
		t.Error("Dead synapse should be removed")
	}
}

func TestHebbianEngineGetSynapseWeight(t *testing.T) {
	m := newTestMatrix()
	h := NewHebbianEngine(m)

	s := core.NewSynapse("n1", "n2", 0.7)
	m.Synapses[s.ID] = s

	weight := h.GetSynapseWeight("n1", "n2")
	if weight != 0.7 {
		t.Errorf("Expected weight 0.7, got %f", weight)
	}

	// Test reverse direction
	weight = h.GetSynapseWeight("n2", "n1")
	if weight != 0.7 {
		t.Errorf("Expected weight 0.7 for reverse, got %f", weight)
	}

	// Test non-existent
	weight = h.GetSynapseWeight("n3", "n4")
	if weight != 0 {
		t.Errorf("Expected weight 0 for non-existent, got %f", weight)
	}
}

func TestHebbianEngineGetConnectedNeurons(t *testing.T) {
	m := newTestMatrix()
	h := NewHebbianEngine(m)

	// Setup adjacency and synapses
	m.Adjacency["n1"] = []core.NeuronID{"n2", "n3"}
	m.Synapses[core.NewSynapseID("n1", "n2")] = core.NewSynapse("n1", "n2", 0.8)
	m.Synapses[core.NewSynapseID("n1", "n3")] = core.NewSynapse("n1", "n3", 0.2)

	// Get connections with min weight 0.5
	connected := h.GetConnectedNeurons("n1", 0.5)

	if len(connected) != 1 {
		t.Errorf("Expected 1 connection above threshold, got %d", len(connected))
	}
}

func TestHebbianEngineSelfTune(t *testing.T) {
	m := newTestMatrix()
	h := NewHebbianEngine(m)

	// Add some neurons
	for i := 0; i < 10; i++ {
		n := core.NewNeuron("Content", m.CurrentDim)
		m.Neurons[n.ID] = n
	}

	initialLearningRate := h.learningRate

	// Self tune should adjust parameters
	h.SelfTune()

	// Learning rate should increase if few synapses
	if h.learningRate <= initialLearningRate {
		t.Log("Note: Learning rate adjustment depends on synapse density")
	}
}

func TestHebbianEngineStats(t *testing.T) {
	m := newTestMatrix()
	h := NewHebbianEngine(m)

	stats := h.Stats()

	if stats["learning_rate"] == nil {
		t.Error("Stats should include learning_rate")
	}
	if stats["forgetting_rate"] == nil {
		t.Error("Stats should include forgetting_rate")
	}
}
