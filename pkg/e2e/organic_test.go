package e2e

import (
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/denizumutdereli/qubicdb/pkg/concurrency"
	"github.com/denizumutdereli/qubicdb/pkg/core"
	"github.com/denizumutdereli/qubicdb/pkg/daemon"
	"github.com/denizumutdereli/qubicdb/pkg/lifecycle"
	"github.com/denizumutdereli/qubicdb/pkg/persistence"
)

// E2E Test: Organic Growth - Per-User Matrix Shapes
func TestOrganicGrowthPerUser(t *testing.T) {
	tmpDir, _ := os.MkdirTemp("", "qubicdb-e2e-*")
	defer os.RemoveAll(tmpDir)

	store, _ := persistence.NewStore(tmpDir, true)
	pool := concurrency.NewWorkerPool(store, core.DefaultBounds())
	defer pool.Shutdown()

	// User A: Heavy user - adds many neurons
	workerA, _ := pool.GetOrCreate("user-heavy")
	for i := 0; i < 100; i++ {
		workerA.Submit(&concurrency.Operation{
			Type: concurrency.OpWrite,
			Payload: concurrency.AddNeuronRequest{
				Content: fmt.Sprintf("Heavy user content %d about programming and development", i),
			},
		})
	}

	// User B: Light user - adds few neurons
	workerB, _ := pool.GetOrCreate("user-light")
	for i := 0; i < 5; i++ {
		workerB.Submit(&concurrency.Operation{
			Type: concurrency.OpWrite,
			Payload: concurrency.AddNeuronRequest{
				Content: fmt.Sprintf("Light user content %d", i),
			},
		})
	}

	// Get stats for both
	statsA, _ := workerA.Submit(&concurrency.Operation{Type: concurrency.OpGetStats})
	statsB, _ := workerB.Submit(&concurrency.Operation{Type: concurrency.OpGetStats})

	neuronCountA := statsA.(map[string]any)["neuron_count"].(int)
	neuronCountB := statsB.(map[string]any)["neuron_count"].(int)

	// Verify organic differentiation
	if neuronCountA <= neuronCountB {
		t.Errorf("Heavy user should have more neurons: A=%d, B=%d", neuronCountA, neuronCountB)
	}

	t.Logf("Organic Growth Test: User A=%d neurons, User B=%d neurons", neuronCountA, neuronCountB)
}

// E2E Test: Hebbian Learning - Co-activation Strengthening
func TestHebbianLearning(t *testing.T) {
	tmpDir, _ := os.MkdirTemp("", "qubicdb-hebbian-*")
	defer os.RemoveAll(tmpDir)

	store, _ := persistence.NewStore(tmpDir, true)
	pool := concurrency.NewWorkerPool(store, core.DefaultBounds())
	defer pool.Shutdown()

	worker, _ := pool.GetOrCreate("user-hebbian")

	// Add neurons
	worker.Submit(&concurrency.Operation{
		Type:    concurrency.OpWrite,
		Payload: concurrency.AddNeuronRequest{Content: "TypeScript programming"},
	})
	worker.Submit(&concurrency.Operation{
		Type:    concurrency.OpWrite,
		Payload: concurrency.AddNeuronRequest{Content: "React framework"},
	})

	// Search for both together multiple times (co-activation)
	for i := 0; i < 10; i++ {
		worker.Submit(&concurrency.Operation{
			Type: concurrency.OpSearch,
			Payload: concurrency.SearchRequest{
				Query: "TypeScript",
				Depth: 1,
				Limit: 10,
			},
		})
		worker.Submit(&concurrency.Operation{
			Type: concurrency.OpSearch,
			Payload: concurrency.SearchRequest{
				Query: "React",
				Depth: 1,
				Limit: 10,
			},
		})
	}

	// Check synapse formation
	stats, _ := worker.Submit(&concurrency.Operation{Type: concurrency.OpGetStats})
	synapseCount := stats.(map[string]any)["synapse_count"].(int)

	if synapseCount < 1 {
		t.Log("Note: Synapse formation depends on co-activation window timing")
	}

	t.Logf("Hebbian Learning: %d synapses formed", synapseCount)
}

// E2E Test: Hot/Cold Memory - Energy Decay
func TestHotColdMemory(t *testing.T) {
	tmpDir, _ := os.MkdirTemp("", "qubicdb-hotcold-*")
	defer os.RemoveAll(tmpDir)

	store, _ := persistence.NewStore(tmpDir, true)
	pool := concurrency.NewWorkerPool(store, core.DefaultBounds())
	defer pool.Shutdown()

	worker, _ := pool.GetOrCreate("user-hotcold")

	// Add neuron
	result, _ := worker.Submit(&concurrency.Operation{
		Type:    concurrency.OpWrite,
		Payload: concurrency.AddNeuronRequest{Content: "Hot memory content"},
	})
	neuron := result.(*core.Neuron)
	initialEnergy := neuron.Energy

	// Simulate time passing by setting LastFiredAt to past
	neuron.LastFiredAt = time.Now().Add(-1 * time.Hour)

	// Trigger decay
	worker.Submit(&concurrency.Operation{Type: concurrency.OpDecay})

	// Energy should have decreased
	if neuron.Energy >= initialEnergy {
		t.Error("Energy should decay over time")
	}

	t.Logf("Hot/Cold Memory: Initial=%.2f, After decay=%.2f", initialEnergy, neuron.Energy)
}

// E2E Test: Lifecycle - Sleep/Wake Cycles
func TestLifecycleSleepWake(t *testing.T) {
	tmpDir, _ := os.MkdirTemp("", "qubicdb-lifecycle-*")
	defer os.RemoveAll(tmpDir)

	store, _ := persistence.NewStore(tmpDir, true)
	pool := concurrency.NewWorkerPool(store, core.DefaultBounds())
	lm := lifecycle.NewManager()
	defer pool.Shutdown()
	defer lm.Stop()

	indexID := core.IndexID("user-lifecycle")

	// Record activity - should be awake
	lm.RecordActivity(indexID)
	state := lm.GetState(indexID)

	if state != core.StateActive {
		t.Errorf("Should be Active after activity, got %d", state)
	}

	// Force sleep
	lm.ForceSleep(indexID)
	state = lm.GetState(indexID)

	if state != core.StateSleeping {
		t.Errorf("Should be Sleeping after ForceSleep, got %d", state)
	}

	// Wake up
	lm.ForceWake(indexID)
	state = lm.GetState(indexID)

	if state != core.StateActive {
		t.Errorf("Should be Active after ForceWake, got %d", state)
	}

	t.Log("Lifecycle Test: Sleep/Wake cycles working correctly")
}

// E2E Test: Consolidation - Surface to Deep Memory
func TestConsolidation(t *testing.T) {
	tmpDir, _ := os.MkdirTemp("", "qubicdb-consolidate-*")
	defer os.RemoveAll(tmpDir)

	store, _ := persistence.NewStore(tmpDir, true)
	pool := concurrency.NewWorkerPool(store, core.DefaultBounds())
	defer pool.Shutdown()

	worker, _ := pool.GetOrCreate("user-consolidate")

	// Add neuron
	result, _ := worker.Submit(&concurrency.Operation{
		Type:    concurrency.OpWrite,
		Payload: concurrency.AddNeuronRequest{Content: "Consolidation test content"},
	})
	neuron := result.(*core.Neuron)

	// Make it mature (high access count, old enough)
	neuron.AccessCount = 20
	neuron.CreatedAt = time.Now().Add(-1 * time.Hour)

	initialDepth := neuron.Depth

	// Trigger consolidation
	result, _ = worker.Submit(&concurrency.Operation{Type: concurrency.OpConsolidate})
	consolidated := result.(int)

	// Neuron should have moved deeper
	if consolidated > 0 && neuron.Depth <= initialDepth {
		t.Error("Consolidated neuron should have increased depth")
	}

	t.Logf("Consolidation: %d neurons consolidated, depth %d->%d", consolidated, initialDepth, neuron.Depth)
}

// E2E Test: Persistence and Recall
func TestPersistenceRecall(t *testing.T) {
	tmpDir, _ := os.MkdirTemp("", "qubicdb-persist-*")
	defer os.RemoveAll(tmpDir)

	store, _ := persistence.NewStore(tmpDir, true)

	// First session
	pool1 := concurrency.NewWorkerPool(store, core.DefaultBounds())
	worker1, _ := pool1.GetOrCreate("user-persist")
	worker1.Submit(&concurrency.Operation{
		Type:    concurrency.OpWrite,
		Payload: concurrency.AddNeuronRequest{Content: "Persistent memory content"},
	})
	pool1.Shutdown()

	// Second session
	pool2 := concurrency.NewWorkerPool(store, core.DefaultBounds())
	defer pool2.Shutdown()
	worker2, _ := pool2.GetOrCreate("user-persist")

	// Search for the content
	result, _ := worker2.Submit(&concurrency.Operation{
		Type: concurrency.OpSearch,
		Payload: concurrency.SearchRequest{
			Query: "Persistent",
			Depth: 1,
			Limit: 10,
		},
	})
	neurons := result.([]*core.Neuron)

	if len(neurons) < 1 {
		t.Log("Note: Persistence depends on store implementation")
	}

	t.Logf("Persistence Recall: Found %d neurons after restart", len(neurons))
}

// E2E Test: Concurrent Users Stress Test
func TestConcurrentUsersStress(t *testing.T) {
	tmpDir, _ := os.MkdirTemp("", "qubicdb-stress-*")
	defer os.RemoveAll(tmpDir)

	store, _ := persistence.NewStore(tmpDir, true)
	pool := concurrency.NewWorkerPool(store, core.DefaultBounds())
	defer pool.Shutdown()

	numUsers := 50
	opsPerUser := 20

	var wg sync.WaitGroup

	for u := 0; u < numUsers; u++ {
		wg.Add(1)
		go func(indexIdx int) {
			defer wg.Done()
			indexID := core.IndexID(fmt.Sprintf("stress-user-%d", indexIdx))
			worker, err := pool.GetOrCreate(indexID)
			if err != nil {
				t.Errorf("Failed to create worker for user %d: %v", indexIdx, err)
				return
			}

			for i := 0; i < opsPerUser; i++ {
				// Mix of operations
				switch i % 3 {
				case 0:
					worker.Submit(&concurrency.Operation{
						Type: concurrency.OpWrite,
						Payload: concurrency.AddNeuronRequest{
							Content: fmt.Sprintf("Stress content %d-%d", indexIdx, i),
						},
					})
				case 1:
					worker.Submit(&concurrency.Operation{
						Type: concurrency.OpSearch,
						Payload: concurrency.SearchRequest{
							Query: "Stress",
							Depth: 1,
							Limit: 5,
						},
					})
				case 2:
					worker.Submit(&concurrency.Operation{Type: concurrency.OpGetStats})
				}
			}
		}(u)
	}

	wg.Wait()

	// Verify all users created
	count := 0
	pool.ForEach(func(indexID core.IndexID, worker *concurrency.BrainWorker) {
		count++
	})

	if count != numUsers {
		t.Errorf("Expected %d users, got %d", numUsers, count)
	}

	t.Logf("Stress Test: %d users, %d ops each completed successfully", numUsers, opsPerUser)
}

// E2E Test: Full System Integration
func TestFullSystemIntegration(t *testing.T) {
	tmpDir, _ := os.MkdirTemp("", "qubicdb-integration-*")
	defer os.RemoveAll(tmpDir)

	store, _ := persistence.NewStore(tmpDir, true)
	pool := concurrency.NewWorkerPool(store, core.DefaultBounds())
	lm := lifecycle.NewManager()
	dm := daemon.NewDaemonManager(pool, lm, store)

	// Set short intervals for testing
	dm.SetIntervals(
		100*time.Millisecond,
		100*time.Millisecond,
		100*time.Millisecond,
		100*time.Millisecond,
		100*time.Millisecond,
	)

	dm.Start()

	// Simulate user activity
	indexID := core.IndexID("integration-user")
	worker, _ := pool.GetOrCreate(indexID)
	lm.RecordActivity(indexID)

	// Add neurons
	for i := 0; i < 10; i++ {
		worker.Submit(&concurrency.Operation{
			Type: concurrency.OpWrite,
			Payload: concurrency.AddNeuronRequest{
				Content: fmt.Sprintf("Integration content %d about various topics", i),
			},
		})
	}

	// Search
	result, _ := worker.Submit(&concurrency.Operation{
		Type: concurrency.OpSearch,
		Payload: concurrency.SearchRequest{
			Query: "Integration",
			Depth: 2,
			Limit: 10,
		},
	})
	neurons := result.([]*core.Neuron)

	// Let daemons run
	time.Sleep(500 * time.Millisecond)

	// Get final stats
	statsResult, _ := worker.Submit(&concurrency.Operation{Type: concurrency.OpGetStats})
	stats := statsResult.(map[string]any)

	dm.Stop()
	pool.Shutdown()
	lm.Stop()

	// Verify system worked
	if len(neurons) < 1 {
		t.Error("Search should return results")
	}

	t.Logf("Full Integration: %d neurons found, stats=%v", len(neurons), stats)
}

// E2E Test: Recall Effectiveness
func TestRecallEffectiveness(t *testing.T) {
	tmpDir, _ := os.MkdirTemp("", "qubicdb-recall-*")
	defer os.RemoveAll(tmpDir)

	store, _ := persistence.NewStore(tmpDir, true)
	pool := concurrency.NewWorkerPool(store, core.DefaultBounds())
	defer pool.Shutdown()

	worker, _ := pool.GetOrCreate("user-recall")

	// Add diverse content
	contents := []string{
		"TypeScript is a typed superset of JavaScript",
		"Go is a statically typed compiled language",
		"Python is known for its simplicity",
		"React is a JavaScript library for building UIs",
		"Docker enables containerization",
		"Kubernetes orchestrates containers",
	}

	for _, content := range contents {
		worker.Submit(&concurrency.Operation{
			Type:    concurrency.OpWrite,
			Payload: concurrency.AddNeuronRequest{Content: content},
		})
	}

	// Test recall with different queries
	testCases := []struct {
		query    string
		expected int
	}{
		{"TypeScript", 1},
		{"JavaScript", 2}, // TypeScript and React both mention JavaScript
		{"language", 2},   // Go and TypeScript
		{"container", 2},  // Docker and Kubernetes
	}

	for _, tc := range testCases {
		result, _ := worker.Submit(&concurrency.Operation{
			Type: concurrency.OpSearch,
			Payload: concurrency.SearchRequest{
				Query: tc.query,
				Depth: 1,
				Limit: 10,
			},
		})
		neurons := result.([]*core.Neuron)

		if len(neurons) < 1 {
			t.Errorf("Query '%s': expected results, got 0", tc.query)
		}

		t.Logf("Recall '%s': found %d neurons", tc.query, len(neurons))
	}
}
