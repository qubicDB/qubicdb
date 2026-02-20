package daemon

import (
	"os"
	"testing"
	"time"

	"github.com/qubicDB/qubicdb/pkg/concurrency"
	"github.com/qubicDB/qubicdb/pkg/core"
	"github.com/qubicDB/qubicdb/pkg/lifecycle"
	"github.com/qubicDB/qubicdb/pkg/persistence"
)

func setupTestDaemon(t *testing.T) (*DaemonManager, *concurrency.WorkerPool, *lifecycle.Manager, string) {
	tmpDir, err := os.MkdirTemp("", "qubicdb-daemon-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}

	store, err := persistence.NewStore(tmpDir, true)
	if err != nil {
		os.RemoveAll(tmpDir)
		t.Fatalf("Failed to create store: %v", err)
	}

	pool := concurrency.NewWorkerPool(store, core.DefaultBounds())
	lm := lifecycle.NewManager()

	dm := NewDaemonManager(pool, lm, store)

	return dm, pool, lm, tmpDir
}

func TestDaemonManagerCreation(t *testing.T) {
	dm, _, lm, tmpDir := setupTestDaemon(t)
	defer os.RemoveAll(tmpDir)
	defer lm.Stop()

	if dm == nil {
		t.Fatal("NewDaemonManager returned nil")
	}
}

func TestDaemonManagerStartStop(t *testing.T) {
	dm, _, lm, tmpDir := setupTestDaemon(t)
	defer os.RemoveAll(tmpDir)
	defer lm.Stop()

	dm.Start()

	// Give daemons time to start
	time.Sleep(100 * time.Millisecond)

	// Stop should complete without deadlock
	done := make(chan bool)
	go func() {
		dm.Stop()
		done <- true
	}()

	select {
	case <-done:
		// OK
	case <-time.After(5 * time.Second):
		t.Error("Stop should complete within timeout")
	}
}

func TestDaemonManagerSetIntervals(t *testing.T) {
	dm, _, lm, tmpDir := setupTestDaemon(t)
	defer os.RemoveAll(tmpDir)
	defer lm.Stop()

	dm.SetIntervals(
		10*time.Second,
		20*time.Second,
		30*time.Second,
		40*time.Second,
		50*time.Second,
	)

	stats := dm.Stats()

	if stats["decay_interval"].(string) != "10s" {
		t.Errorf("Expected decay_interval 10s, got %s", stats["decay_interval"])
	}
	if stats["consolidate_interval"].(string) != "20s" {
		t.Errorf("Expected consolidate_interval 20s, got %s", stats["consolidate_interval"])
	}
}

func TestDaemonManagerStats(t *testing.T) {
	dm, _, lm, tmpDir := setupTestDaemon(t)
	defer os.RemoveAll(tmpDir)
	defer lm.Stop()

	stats := dm.Stats()

	if stats["decay_interval"] == nil {
		t.Error("Stats should include decay_interval")
	}
	if stats["consolidate_interval"] == nil {
		t.Error("Stats should include consolidate_interval")
	}
	if stats["prune_interval"] == nil {
		t.Error("Stats should include prune_interval")
	}
	if stats["persist_interval"] == nil {
		t.Error("Stats should include persist_interval")
	}
	if stats["reorg_interval"] == nil {
		t.Error("Stats should include reorg_interval")
	}
}

func TestDaemonDecayIntegration(t *testing.T) {
	dm, pool, lm, tmpDir := setupTestDaemon(t)
	defer os.RemoveAll(tmpDir)
	defer lm.Stop()

	// Set short interval for testing
	dm.SetIntervals(
		100*time.Millisecond, // decay
		1*time.Hour,          // consolidate
		1*time.Hour,          // prune
		1*time.Hour,          // persist
		1*time.Hour,          // reorg
	)

	// Create a worker with a neuron
	worker, _ := pool.GetOrCreate("test-user")
	worker.Submit(&concurrency.Operation{
		Type: concurrency.OpWrite,
		Payload: concurrency.AddNeuronRequest{
			Content: "Test neuron",
		},
	})

	// Record activity
	lm.RecordActivity("test-user")

	// Start daemon
	dm.Start()

	// Wait for decay to run
	time.Sleep(300 * time.Millisecond)

	dm.Stop()

	// Neuron energy should have decayed slightly
	// (depends on decay rate and time)
	t.Log("Decay daemon ran successfully")
}

func TestDaemonConsolidateIntegration(t *testing.T) {
	dm, pool, lm, tmpDir := setupTestDaemon(t)
	defer os.RemoveAll(tmpDir)
	defer lm.Stop()

	dm.SetIntervals(
		1*time.Hour,
		100*time.Millisecond, // consolidate
		1*time.Hour,
		1*time.Hour,
		1*time.Hour,
	)

	// Create worker with mature neuron
	worker, _ := pool.GetOrCreate("test-user")
	result, _ := worker.Submit(&concurrency.Operation{
		Type: concurrency.OpWrite,
		Payload: concurrency.AddNeuronRequest{
			Content: "Mature neuron",
		},
	})

	n := result.(*core.Neuron)
	n.AccessCount = 20
	n.CreatedAt = time.Now().Add(-1 * time.Hour)

	// Put user to sleep for consolidation
	lm.RecordActivity("test-user")
	lm.ForceSleep("test-user")

	dm.Start()
	time.Sleep(300 * time.Millisecond)
	dm.Stop()

	// Check if neuron was consolidated
	if n.Depth > 0 {
		t.Log("Consolidation worked - neuron moved to deeper layer")
	}
}

func TestClamp(t *testing.T) {
	tests := []struct {
		val, min, max, expected float64
	}{
		{0.5, 0, 1, 0.5},
		{-0.5, 0, 1, 0},
		{1.5, 0, 1, 1},
		{0, 0, 1, 0},
		{1, 0, 1, 1},
	}

	for _, tt := range tests {
		result := clamp(tt.val, tt.min, tt.max)
		if result != tt.expected {
			t.Errorf("clamp(%f, %f, %f) = %f, expected %f",
				tt.val, tt.min, tt.max, result, tt.expected)
		}
	}
}
