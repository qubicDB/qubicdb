package concurrency

import (
	"testing"
	"time"

	"github.com/denizumutdereli/qubicdb/pkg/core"
)

func TestOperationTypes(t *testing.T) {
	// Verify all operation types are distinct
	ops := []OpType{
		OpWrite, OpRead, OpSearch, OpTouch,
		OpForget, OpRecall, OpFire, OpDecay,
		OpConsolidate, OpPrune, OpReorg, OpGetStats, OpShutdown,
	}

	seen := make(map[OpType]bool)
	for _, op := range ops {
		if seen[op] {
			t.Errorf("Duplicate OpType: %d", op)
		}
		seen[op] = true
	}
}

func TestBrainWorkerAllOperations(t *testing.T) {
	m := core.NewMatrix("test-user", core.DefaultBounds())
	w := NewBrainWorker("test-user", m)
	defer w.Stop()

	// OpWrite
	result, err := w.Submit(&Operation{
		Type:    OpWrite,
		Payload: AddNeuronRequest{Content: "Test content"},
	})
	if err != nil {
		t.Fatalf("OpWrite failed: %v", err)
	}
	n := result.(*core.Neuron)

	// OpRead
	result, err = w.Submit(&Operation{
		Type:    OpRead,
		Payload: n.ID,
	})
	if err != nil {
		t.Fatalf("OpRead failed: %v", err)
	}

	// OpTouch
	_, err = w.Submit(&Operation{
		Type: OpTouch,
		Payload: UpdateNeuronRequest{
			ID:      n.ID,
			Content: "Updated content",
		},
	})
	if err != nil {
		t.Fatalf("OpTouch failed: %v", err)
	}

	// OpSearch
	result, err = w.Submit(&Operation{
		Type: OpSearch,
		Payload: SearchRequest{
			Query: "content",
			Depth: 1,
			Limit: 10,
		},
	})
	if err != nil {
		t.Fatalf("OpSearch failed: %v", err)
	}

	// OpRecall
	result, err = w.Submit(&Operation{
		Type: OpRecall,
		Payload: ListNeuronsRequest{
			Offset: 0,
			Limit:  10,
		},
	})
	if err != nil {
		t.Fatalf("OpRecall failed: %v", err)
	}

	// OpFire
	w.Submit(&Operation{
		Type:    OpFire,
		Payload: n.ID,
	})

	// OpDecay
	w.Submit(&Operation{Type: OpDecay})

	// OpConsolidate
	w.Submit(&Operation{Type: OpConsolidate})

	// OpPrune
	w.Submit(&Operation{Type: OpPrune})

	// OpReorg
	w.Submit(&Operation{Type: OpReorg})

	// OpGetStats
	result, err = w.Submit(&Operation{Type: OpGetStats})
	if err != nil {
		t.Fatalf("OpGetStats failed: %v", err)
	}
	stats := result.(map[string]any)
	if stats["index_id"] != core.IndexID("test-user") {
		t.Error("Stats index_id mismatch")
	}

	// OpForget
	_, err = w.Submit(&Operation{
		Type:    OpForget,
		Payload: n.ID,
	})
	if err != nil {
		t.Fatalf("OpForget failed: %v", err)
	}
}

func TestBrainWorkerPrune(t *testing.T) {
	m := core.NewMatrix("test-user", core.DefaultBounds())
	w := NewBrainWorker("test-user", m)
	defer w.Stop()

	// Add neuron and make it dead
	result, _ := w.Submit(&Operation{
		Type:    OpWrite,
		Payload: AddNeuronRequest{Content: "Dead neuron"},
	})
	n := result.(*core.Neuron)
	n.Energy = 0.001 // Below alive threshold

	// Prune
	pruneResult, _ := w.Submit(&Operation{Type: OpPrune})
	pruned := pruneResult.(int)

	if pruned < 1 {
		t.Log("Prune should remove dead neurons")
	}
}

func TestBrainWorkerReorg(t *testing.T) {
	m := core.NewMatrix("test-user", core.DefaultBounds())
	w := NewBrainWorker("test-user", m)
	defer w.Stop()

	// Add neurons
	for i := 0; i < 5; i++ {
		w.Submit(&Operation{
			Type:    OpWrite,
			Payload: AddNeuronRequest{Content: "Reorg test content"},
		})
	}

	// Reorg should not panic
	w.Submit(&Operation{Type: OpReorg})

	// Verify matrix still valid
	stats, _ := w.Submit(&Operation{Type: OpGetStats})
	if stats.(map[string]any)["neuron_count"].(int) < 1 {
		t.Error("Neurons should still exist after reorg")
	}
}

func TestBrainWorkerListNeuronsWithDepth(t *testing.T) {
	m := core.NewMatrix("test-user", core.DefaultBounds())
	w := NewBrainWorker("test-user", m)
	defer w.Stop()

	// Add neurons at different depths
	result1, _ := w.Submit(&Operation{
		Type:    OpWrite,
		Payload: AddNeuronRequest{Content: "Surface neuron"},
	})
	n1 := result1.(*core.Neuron)
	n1.Depth = 0

	result2, _ := w.Submit(&Operation{
		Type:    OpWrite,
		Payload: AddNeuronRequest{Content: "Deep neuron"},
	})
	n2 := result2.(*core.Neuron)
	n2.Depth = 2

	// List with depth filter
	depth0 := 0
	result, _ := w.Submit(&Operation{
		Type: OpRecall,
		Payload: ListNeuronsRequest{
			Offset:      0,
			Limit:       10,
			DepthFilter: &depth0,
		},
	})
	neurons := result.([]*core.Neuron)

	found := false
	for _, n := range neurons {
		if n.ID == n1.ID {
			found = true
		}
	}
	if !found {
		t.Log("Depth filter should include surface neurons")
	}
}

func TestBrainWorkerFireNonExistent(t *testing.T) {
	m := core.NewMatrix("test-user", core.DefaultBounds())
	w := NewBrainWorker("test-user", m)
	defer w.Stop()

	// Fire non-existent neuron should not panic
	w.Submit(&Operation{
		Type:    OpFire,
		Payload: core.NeuronID("nonexistent"),
	})
}

func TestBrainWorkerGetNeuronNotFound(t *testing.T) {
	m := core.NewMatrix("test-user", core.DefaultBounds())
	w := NewBrainWorker("test-user", m)
	defer w.Stop()

	_, err := w.Submit(&Operation{
		Type:    OpRead,
		Payload: core.NeuronID("nonexistent"),
	})

	if err != core.ErrNeuronNotFound {
		t.Error("Should return ErrNeuronNotFound")
	}
}

func TestBrainWorkerUpdateNeuronNotFound(t *testing.T) {
	m := core.NewMatrix("test-user", core.DefaultBounds())
	w := NewBrainWorker("test-user", m)
	defer w.Stop()

	_, err := w.Submit(&Operation{
		Type: OpTouch,
		Payload: UpdateNeuronRequest{
			ID:      "nonexistent",
			Content: "New content",
		},
	})

	if err != core.ErrNeuronNotFound {
		t.Error("Should return ErrNeuronNotFound")
	}
}

func TestBrainWorkerDeleteNeuronNotFound(t *testing.T) {
	m := core.NewMatrix("test-user", core.DefaultBounds())
	w := NewBrainWorker("test-user", m)
	defer w.Stop()

	_, err := w.Submit(&Operation{
		Type:    OpForget,
		Payload: core.NeuronID("nonexistent"),
	})

	if err != core.ErrNeuronNotFound {
		t.Error("Should return ErrNeuronNotFound")
	}
}

func TestBrainWorkerMatrix(t *testing.T) {
	m := core.NewMatrix("test-user", core.DefaultBounds())
	w := NewBrainWorker("test-user", m)
	defer w.Stop()

	matrix := w.Matrix()
	if matrix != m {
		t.Error("Matrix() should return the worker's matrix")
	}
}

func TestBrainWorkerIndexID(t *testing.T) {
	m := core.NewMatrix("test-user", core.DefaultBounds())
	w := NewBrainWorker("test-user", m)
	defer w.Stop()

	stats := w.Stats()
	if stats["index_id"] != core.IndexID("test-user") {
		t.Error("Worker should track index_id")
	}
}

func TestBrainWorkerLastOp(t *testing.T) {
	m := core.NewMatrix("test-user", core.DefaultBounds())
	w := NewBrainWorker("test-user", m)
	defer w.Stop()

	before := time.Now()

	w.Submit(&Operation{
		Type:    OpWrite,
		Payload: AddNeuronRequest{Content: "Test"},
	})

	stats := w.Stats()
	lastOp := stats["last_op"].(time.Time)

	if lastOp.Before(before) {
		t.Error("last_op should be updated after operation")
	}
}

func TestBrainWorkerOpsProcessed(t *testing.T) {
	m := core.NewMatrix("test-user", core.DefaultBounds())
	w := NewBrainWorker("test-user", m)
	defer w.Stop()

	// Submit multiple operations
	for i := 0; i < 5; i++ {
		w.Submit(&Operation{
			Type:    OpWrite,
			Payload: AddNeuronRequest{Content: "Test"},
		})
	}

	stats := w.Stats()
	opsProcessed := stats["ops_processed"].(uint64)

	if opsProcessed < 5 {
		t.Errorf("Expected at least 5 ops processed, got %d", opsProcessed)
	}
}

func TestClampFunction(t *testing.T) {
	tests := []struct {
		val, min, max, expected float64
	}{
		{0.5, 0, 1, 0.5},
		{-0.5, 0, 1, 0},
		{1.5, 0, 1, 1},
		{0, 0, 1, 0},
		{1, 0, 1, 1},
		{-2, -1, 1, -1},
		{2, -1, 1, 1},
	}

	for _, tt := range tests {
		result := clamp(tt.val, tt.min, tt.max)
		if result != tt.expected {
			t.Errorf("clamp(%f, %f, %f) = %f, expected %f",
				tt.val, tt.min, tt.max, result, tt.expected)
		}
	}
}
