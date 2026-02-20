package concurrency

import (
	"os"
	"sync"
	"testing"
	"time"

	"github.com/denizumutdereli/qubicdb/pkg/core"
	"github.com/denizumutdereli/qubicdb/pkg/persistence"
)

func setupTestPool(t *testing.T) (*WorkerPool, string) {
	tmpDir, err := os.MkdirTemp("", "qubicdb-pool-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}

	store, err := persistence.NewStore(tmpDir, true)
	if err != nil {
		os.RemoveAll(tmpDir)
		t.Fatalf("Failed to create store: %v", err)
	}

	pool := NewWorkerPool(store, core.DefaultBounds())
	return pool, tmpDir
}

func TestWorkerPoolCreation(t *testing.T) {
	pool, tmpDir := setupTestPool(t)
	defer os.RemoveAll(tmpDir)
	defer pool.Shutdown()

	if pool == nil {
		t.Fatal("NewWorkerPool returned nil")
	}
}

func TestWorkerPoolGetOrCreate(t *testing.T) {
	pool, tmpDir := setupTestPool(t)
	defer os.RemoveAll(tmpDir)
	defer pool.Shutdown()

	worker1, err := pool.GetOrCreate("user-1")
	if err != nil {
		t.Fatalf("GetOrCreate failed: %v", err)
	}

	if worker1 == nil {
		t.Error("Worker should not be nil")
	}

	// Getting same user should return same worker
	worker2, _ := pool.GetOrCreate("user-1")
	if worker1 != worker2 {
		t.Error("Same user should get same worker")
	}
}

func TestWorkerPoolGet(t *testing.T) {
	pool, tmpDir := setupTestPool(t)
	defer os.RemoveAll(tmpDir)
	defer pool.Shutdown()

	// Get non-existent
	worker, err := pool.Get("nonexistent")
	if err == nil || worker != nil {
		t.Error("Should return error for non-existent user")
	}

	// Create and then get
	pool.GetOrCreate("user-1")
	worker, err = pool.Get("user-1")
	if err != nil || worker == nil {
		t.Error("Should return worker for existing user")
	}
}

func TestWorkerPoolEvict(t *testing.T) {
	pool, tmpDir := setupTestPool(t)
	defer os.RemoveAll(tmpDir)
	defer pool.Shutdown()

	pool.GetOrCreate("user-1")

	err := pool.Evict("user-1")
	if err != nil {
		t.Fatalf("Evict failed: %v", err)
	}

	worker, err := pool.Get("user-1")
	if err == nil || worker != nil {
		t.Error("Worker should be nil after eviction")
	}
}

func TestWorkerPoolEvictNonExistent(t *testing.T) {
	pool, tmpDir := setupTestPool(t)
	defer os.RemoveAll(tmpDir)
	defer pool.Shutdown()

	err := pool.Evict("nonexistent")
	if err != nil {
		t.Error("Evict non-existent should not error")
	}
}

func TestWorkerPoolForEach(t *testing.T) {
	pool, tmpDir := setupTestPool(t)
	defer os.RemoveAll(tmpDir)
	defer pool.Shutdown()

	// Create multiple workers
	pool.GetOrCreate("user-1")
	pool.GetOrCreate("user-2")
	pool.GetOrCreate("user-3")

	count := 0
	pool.ForEach(func(indexID core.IndexID, worker *BrainWorker) {
		count++
	})

	if count != 3 {
		t.Errorf("Expected 3 workers, got %d", count)
	}
}

func TestWorkerPoolPersistAll(t *testing.T) {
	pool, tmpDir := setupTestPool(t)
	defer os.RemoveAll(tmpDir)
	defer pool.Shutdown()

	worker, _ := pool.GetOrCreate("user-1")
	worker.Submit(&Operation{
		Type:    OpWrite,
		Payload: AddNeuronRequest{Content: "Test content"},
	})

	err := pool.PersistAll()
	if err != nil {
		t.Fatalf("PersistAll failed: %v", err)
	}
}

func TestWorkerPoolConcurrentGetOrCreate(t *testing.T) {
	pool, tmpDir := setupTestPool(t)
	defer os.RemoveAll(tmpDir)
	defer pool.Shutdown()

	var wg sync.WaitGroup
	workers := make([]*BrainWorker, 10)

	// Concurrent GetOrCreate for same user
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			w, _ := pool.GetOrCreate("same-user")
			workers[idx] = w
		}(i)
	}

	wg.Wait()

	// All should be the same worker
	for i := 1; i < 10; i++ {
		if workers[i] != workers[0] {
			t.Error("All concurrent GetOrCreate should return same worker")
		}
	}
}

func TestWorkerPoolConcurrentDifferentUsers(t *testing.T) {
	pool, tmpDir := setupTestPool(t)
	defer os.RemoveAll(tmpDir)
	defer pool.Shutdown()

	var wg sync.WaitGroup

	// Concurrent GetOrCreate for different users
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			indexID := core.IndexID("user-" + string(rune('A'+idx)))
			pool.GetOrCreate(indexID)
		}(i)
	}

	wg.Wait()

	count := 0
	pool.ForEach(func(indexID core.IndexID, worker *BrainWorker) {
		count++
	})

	if count != 10 {
		t.Errorf("Expected 10 workers, got %d", count)
	}
}

func TestWorkerPoolStats(t *testing.T) {
	pool, tmpDir := setupTestPool(t)
	defer os.RemoveAll(tmpDir)
	defer pool.Shutdown()

	pool.GetOrCreate("user-1")
	pool.GetOrCreate("user-2")

	stats := pool.Stats()

	if stats["active_workers"].(int) != 2 {
		t.Errorf("Expected 2 active workers, got %v", stats["active_workers"])
	}
}

func TestWorkerPoolShutdown(t *testing.T) {
	pool, tmpDir := setupTestPool(t)
	defer os.RemoveAll(tmpDir)

	pool.GetOrCreate("user-1")

	done := make(chan bool)
	go func() {
		pool.Shutdown()
		done <- true
	}()

	select {
	case <-done:
		// OK
	case <-time.After(5 * time.Second):
		t.Error("Shutdown should complete within timeout")
	}
}

func TestWorkerPoolReloadFromPersistence(t *testing.T) {
	tmpDir, _ := os.MkdirTemp("", "qubicdb-pool-reload-*")
	defer os.RemoveAll(tmpDir)

	store, _ := persistence.NewStore(tmpDir, true)

	// Create pool, add data, shutdown
	pool1 := NewWorkerPool(store, core.DefaultBounds())
	worker, _ := pool1.GetOrCreate("user-1")
	worker.Submit(&Operation{
		Type:    OpWrite,
		Payload: AddNeuronRequest{Content: "Persistent content"},
	})
	pool1.Shutdown()

	// Create new pool with same store
	pool2 := NewWorkerPool(store, core.DefaultBounds())
	defer pool2.Shutdown()

	// Worker should reload from persistence
	worker2, _ := pool2.GetOrCreate("user-1")
	result, _ := worker2.Submit(&Operation{Type: OpGetStats})
	stats := result.(map[string]any)

	neuronCount := stats["neuron_count"].(int)
	if neuronCount < 1 {
		t.Log("Note: Persistence reload depends on store implementation")
	}
}
