package concurrency

import (
	"testing"
	"time"

	"github.com/denizumutdereli/qubicdb/pkg/core"
)

func newTestMatrix() *core.Matrix {
	return core.NewMatrix("test-user", core.DefaultBounds())
}

func TestBrainWorkerCreation(t *testing.T) {
	m := newTestMatrix()
	w := NewBrainWorker("test-user", m)
	defer w.Stop()

	if w == nil {
		t.Fatal("NewBrainWorker returned nil")
	}
}

func TestBrainWorkerAddNeuron(t *testing.T) {
	m := newTestMatrix()
	w := NewBrainWorker("test-user", m)
	defer w.Stop()

	result, err := w.Submit(&Operation{
		Type: OpWrite,
		Payload: AddNeuronRequest{
			Content: "Test neuron content",
		},
	})

	if err != nil {
		t.Fatalf("AddNeuron failed: %v", err)
	}

	n := result.(*core.Neuron)
	if n.Content != "Test neuron content" {
		t.Error("Content mismatch")
	}
}

func TestBrainWorkerGetNeuron(t *testing.T) {
	m := newTestMatrix()
	w := NewBrainWorker("test-user", m)
	defer w.Stop()

	// Add neuron first
	addResult, _ := w.Submit(&Operation{
		Type: OpWrite,
		Payload: AddNeuronRequest{
			Content: "Test content",
		},
	})
	n := addResult.(*core.Neuron)

	// Get neuron
	result, err := w.Submit(&Operation{
		Type:    OpRead,
		Payload: n.ID,
	})

	if err != nil {
		t.Fatalf("GetNeuron failed: %v", err)
	}

	retrieved := result.(*core.Neuron)
	if retrieved.ID != n.ID {
		t.Error("Retrieved wrong neuron")
	}
}

func TestBrainWorkerSearch(t *testing.T) {
	m := newTestMatrix()
	w := NewBrainWorker("test-user", m)
	defer w.Stop()

	// Add neurons
	w.Submit(&Operation{
		Type:    OpWrite,
		Payload: AddNeuronRequest{Content: "TypeScript programming"},
	})
	w.Submit(&Operation{
		Type:    OpWrite,
		Payload: AddNeuronRequest{Content: "Go programming"},
	})

	// Search
	result, err := w.Submit(&Operation{
		Type: OpSearch,
		Payload: SearchRequest{
			Query: "programming",
			Depth: 1,
			Limit: 10,
		},
	})

	if err != nil {
		t.Fatalf("Search failed: %v", err)
	}

	neurons := result.([]*core.Neuron)
	if len(neurons) != 2 {
		t.Errorf("Expected 2 results, got %d", len(neurons))
	}
}

func TestBrainWorkerUpdateNeuron(t *testing.T) {
	m := newTestMatrix()
	w := NewBrainWorker("test-user", m)
	defer w.Stop()

	// Add neuron
	addResult, _ := w.Submit(&Operation{
		Type:    OpWrite,
		Payload: AddNeuronRequest{Content: "Original"},
	})
	n := addResult.(*core.Neuron)

	// Update
	_, err := w.Submit(&Operation{
		Type: OpTouch,
		Payload: UpdateNeuronRequest{
			ID:      n.ID,
			Content: "Updated",
		},
	})

	if err != nil {
		t.Fatalf("UpdateNeuron failed: %v", err)
	}

	if n.Content != "Updated" {
		t.Error("Content should be updated")
	}
}

func TestBrainWorkerDeleteNeuron(t *testing.T) {
	m := newTestMatrix()
	w := NewBrainWorker("test-user", m)
	defer w.Stop()

	// Add neuron
	addResult, _ := w.Submit(&Operation{
		Type:    OpWrite,
		Payload: AddNeuronRequest{Content: "To delete"},
	})
	n := addResult.(*core.Neuron)

	// Delete
	_, err := w.Submit(&Operation{
		Type:    OpForget,
		Payload: n.ID,
	})

	if err != nil {
		t.Fatalf("DeleteNeuron failed: %v", err)
	}

	if len(m.Neurons) != 0 {
		t.Error("Neuron should be deleted")
	}
}

func TestBrainWorkerListNeurons(t *testing.T) {
	m := newTestMatrix()
	w := NewBrainWorker("test-user", m)
	defer w.Stop()

	// Add neurons
	for i := 0; i < 5; i++ {
		w.Submit(&Operation{
			Type:    OpWrite,
			Payload: AddNeuronRequest{Content: "Neuron " + string(rune('A'+i))},
		})
	}

	// List
	result, err := w.Submit(&Operation{
		Type: OpRecall,
		Payload: ListNeuronsRequest{
			Offset: 0,
			Limit:  10,
		},
	})

	if err != nil {
		t.Fatalf("ListNeurons failed: %v", err)
	}

	neurons := result.([]*core.Neuron)
	if len(neurons) != 5 {
		t.Errorf("Expected 5 neurons, got %d", len(neurons))
	}
}

func TestBrainWorkerDecay(t *testing.T) {
	m := newTestMatrix()
	w := NewBrainWorker("test-user", m)
	defer w.Stop()

	// Add neuron with old timestamp
	addResult, _ := w.Submit(&Operation{
		Type:    OpWrite,
		Payload: AddNeuronRequest{Content: "Test"},
	})
	n := addResult.(*core.Neuron)
	n.LastFiredAt = time.Now().Add(-1 * time.Hour)

	// Decay
	w.Submit(&Operation{Type: OpDecay})

	// Energy should decrease
	if n.Energy >= 1.0 {
		t.Error("Energy should decrease after decay")
	}
}

func TestBrainWorkerConsolidate(t *testing.T) {
	m := newTestMatrix()
	w := NewBrainWorker("test-user", m)
	defer w.Stop()

	// Add old, frequently accessed neuron
	addResult, _ := w.Submit(&Operation{
		Type:    OpWrite,
		Payload: AddNeuronRequest{Content: "Test"},
	})
	n := addResult.(*core.Neuron)
	n.AccessCount = 15
	n.CreatedAt = time.Now().Add(-1 * time.Hour)
	n.Energy = 0.3 // Simulate decayed energy so consolidation threshold is met

	// Consolidate
	result, err := w.Submit(&Operation{Type: OpConsolidate})

	if err != nil {
		t.Fatalf("Consolidate failed: %v", err)
	}

	count := result.(int)
	if count != 1 {
		t.Errorf("Expected 1 consolidated, got %d", count)
	}
	if n.Depth != 1 {
		t.Errorf("Neuron depth should be 1, got %d", n.Depth)
	}
}

func TestBrainWorkerGetStats(t *testing.T) {
	m := newTestMatrix()
	w := NewBrainWorker("test-user", m)
	defer w.Stop()

	result, err := w.Submit(&Operation{Type: OpGetStats})

	if err != nil {
		t.Fatalf("GetStats failed: %v", err)
	}

	stats := result.(map[string]any)
	if stats["index_id"] != core.IndexID("test-user") {
		t.Error("Stats should include index_id")
	}
}

func TestBrainWorkerAsync(t *testing.T) {
	m := newTestMatrix()
	w := NewBrainWorker("test-user", m)
	defer w.Stop()

	// Submit async operations
	for i := 0; i < 10; i++ {
		w.SubmitAsync(&Operation{
			Type:    OpWrite,
			Payload: AddNeuronRequest{Content: "Async neuron"},
		})
	}

	// Wait for processing
	time.Sleep(100 * time.Millisecond)

	// Neurons may be deduplicated, but at least 1 should exist
	if len(m.Neurons) < 1 {
		t.Error("Async operations should add neurons")
	}
}

func TestBrainWorkerStats(t *testing.T) {
	m := newTestMatrix()
	w := NewBrainWorker("test-user", m)
	defer w.Stop()

	// Process some operations
	w.Submit(&Operation{
		Type:    OpWrite,
		Payload: AddNeuronRequest{Content: "Test"},
	})

	stats := w.Stats()

	if stats["index_id"] != core.IndexID("test-user") {
		t.Error("Stats should include index_id")
	}
	if stats["ops_processed"].(uint64) < 1 {
		t.Error("Should have processed at least 1 operation")
	}
}

func TestBrainWorkerStop(t *testing.T) {
	m := newTestMatrix()
	w := NewBrainWorker("test-user", m)

	// Add some operations
	w.Submit(&Operation{
		Type:    OpWrite,
		Payload: AddNeuronRequest{Content: "Test"},
	})

	// Stop should complete without deadlock
	done := make(chan bool)
	go func() {
		w.Stop()
		done <- true
	}()

	select {
	case <-done:
		// OK
	case <-time.After(2 * time.Second):
		t.Error("Stop should complete within timeout")
	}
}
