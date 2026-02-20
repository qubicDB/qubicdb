package e2e

import (
	"fmt"
	"os"
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/qubicDB/qubicdb/pkg/concurrency"
	"github.com/qubicDB/qubicdb/pkg/core"
	"github.com/qubicDB/qubicdb/pkg/daemon"
	"github.com/qubicDB/qubicdb/pkg/lifecycle"
	"github.com/qubicDB/qubicdb/pkg/persistence"
)

type MemStats struct {
	Alloc      uint64
	TotalAlloc uint64
	Sys        uint64
	NumGC      uint32
}

func getMemStats() MemStats {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	return MemStats{
		Alloc:      m.Alloc,
		TotalAlloc: m.TotalAlloc,
		Sys:        m.Sys,
		NumGC:      m.NumGC,
	}
}

func formatBytes(b uint64) string {
	units := []string{"B", "KB", "MB", "GB", "TB", "PB", "EB"}
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%d %s", b, units[0])
	}
	div, exp := uint64(unit), 1
	for n := b / unit; n >= unit && exp < len(units)-1; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %s", float64(b)/float64(div), units[exp])
}

func allocDelta(after, before uint64) uint64 {
	if after <= before {
		return 0
	}
	return after - before
}

// TestPerformanceOrganicLifecycle measures memory behavior across lifecycle
func TestPerformanceOrganicLifecycle(t *testing.T) {
	tmpDir, _ := os.MkdirTemp("", "qubicdb-perf-*")
	defer os.RemoveAll(tmpDir)

	store, _ := persistence.NewStore(tmpDir, true)
	pool := concurrency.NewWorkerPool(store, core.DefaultBounds())
	lm := lifecycle.NewManager()
	dm := daemon.NewDaemonManager(pool, lm, store)

	dm.SetIntervals(
		50*time.Millisecond,  // decay
		100*time.Millisecond, // consolidate
		200*time.Millisecond, // prune
		500*time.Millisecond, // persist
		1*time.Second,        // reorg
	)

	dm.Start()
	defer dm.Stop()
	defer pool.Shutdown()
	defer lm.Stop()

	t.Log("\n=== ORGANIC LIFECYCLE ANALYSIS ===")

	// Phase 1: Initial state
	memBefore := getMemStats()
	t.Logf("Initial memory: Alloc=%s, Sys=%s", formatBytes(memBefore.Alloc), formatBytes(memBefore.Sys))

	// Phase 2: Create users and add neurons
	indexID := core.IndexID("lifecycle-test")
	worker, _ := pool.GetOrCreate(indexID)
	lm.RecordActivity(indexID)

	// Add neurons
	t.Log("\n--- PHASE 1: Neuron Insertion ---")
	for i := 0; i < 100; i++ {
		result, _ := worker.Submit(&concurrency.Operation{
			Type: concurrency.OpWrite,
			Payload: concurrency.AddNeuronRequest{
				Content: fmt.Sprintf("Lifecycle test content %d about programming and development", i),
			},
		})
		n := result.(*core.Neuron)
		if i == 0 {
			t.Logf("First neuron energy: %.2f, depth: %d", n.Energy, n.Depth)
		}
	}

	stats1, _ := worker.Submit(&concurrency.Operation{Type: concurrency.OpGetStats})
	t.Logf("After 100 neurons: %v", stats1)
	memAfterAdd := getMemStats()
	t.Logf("Memory: Alloc=%s (+%s)", formatBytes(memAfterAdd.Alloc), formatBytes(memAfterAdd.Alloc-memBefore.Alloc))

	// Phase 3: Active period - lots of searches
	t.Log("\n--- PHASE 2: Active Usage (Searches) ---")
	searchStart := time.Now()
	for i := 0; i < 50; i++ {
		worker.Submit(&concurrency.Operation{
			Type: concurrency.OpSearch,
			Payload: concurrency.SearchRequest{
				Query: "programming",
				Depth: 2,
				Limit: 10,
			},
		})
		lm.RecordActivity(indexID)
	}
	searchDuration := time.Since(searchStart)
	t.Logf("50 searches duration: %v (avg: %v/op)", searchDuration, searchDuration/50)

	state := lm.GetState(indexID)
	t.Logf("User state: %d (0=Active, 1=Idle, 2=Sleeping, 3=Dormant)", state)

	// Phase 4: Idle period - let daemons run
	t.Log("\n--- PHASE 3: Idle Period (daemons running) ---")
	time.Sleep(500 * time.Millisecond)

	stats2, _ := worker.Submit(&concurrency.Operation{Type: concurrency.OpGetStats})
	t.Logf("Post-daemon stats: %v", stats2)

	// Phase 5: Sleep transition
	t.Log("\n--- PHASE 4: Sleep Transition ---")
	lm.ForceSleep(indexID)
	state = lm.GetState(indexID)
	t.Logf("Post-sleep state: %d", state)

	// Wait for reorg
	time.Sleep(1500 * time.Millisecond)

	stats3, _ := worker.Submit(&concurrency.Operation{Type: concurrency.OpGetStats})
	t.Logf("Post-reorg stats: %v", stats3)

	// Phase 6: Wake up and recall
	t.Log("\n--- PHASE 5: Wake Up & Recall ---")
	lm.ForceWake(indexID)
	lm.RecordActivity(indexID)

	recallStart := time.Now()
	result, _ := worker.Submit(&concurrency.Operation{
		Type: concurrency.OpSearch,
		Payload: concurrency.SearchRequest{
			Query: "programming development",
			Depth: 3,
			Limit: 20,
		},
	})
	recallDuration := time.Since(recallStart)
	neurons := result.([]*core.Neuron)
	t.Logf("Recall: %d neurons found, duration: %v", len(neurons), recallDuration)

	if len(neurons) > 0 {
		t.Logf("Best match energy: %.2f, depth: %d, access: %d",
			neurons[0].Energy, neurons[0].Depth, neurons[0].AccessCount)
	}

	// Final memory
	memFinal := getMemStats()
	t.Logf("\nFinal Memory: Alloc=%s, GC Count=%d", formatBytes(memFinal.Alloc), memFinal.NumGC)
	t.Logf("Total memory increase: %s", formatBytes(memFinal.Alloc-memBefore.Alloc))
}

// TestPerformanceConcurrentUsers measures concurrent user performance
func TestPerformanceConcurrentUsers(t *testing.T) {
	tmpDir, _ := os.MkdirTemp("", "qubicdb-concurrent-*")
	defer os.RemoveAll(tmpDir)

	store, _ := persistence.NewStore(tmpDir, true)
	pool := concurrency.NewWorkerPool(store, core.DefaultBounds())
	defer pool.Shutdown()

	t.Log("\n=== CONCURRENT USERS ANALYSIS ===")

	memBefore := getMemStats()
	t.Logf("Initial: Alloc=%s", formatBytes(memBefore.Alloc))

	testCases := []int{10, 25, 50, 100}

	for _, numUsers := range testCases {
		runtime.GC() // Clean slate
		memStart := getMemStats()

		var wg sync.WaitGroup
		opsPerUser := 20
		totalOps := numUsers * opsPerUser

		start := time.Now()

		for u := 0; u < numUsers; u++ {
			wg.Add(1)
			go func(indexIdx int) {
				defer wg.Done()
				indexID := core.IndexID(fmt.Sprintf("concurrent-user-%d", indexIdx))
				worker, err := pool.GetOrCreate(indexID)
				if err != nil {
					return
				}

				for i := 0; i < opsPerUser; i++ {
					switch i % 4 {
					case 0:
						worker.Submit(&concurrency.Operation{
							Type: concurrency.OpWrite,
							Payload: concurrency.AddNeuronRequest{
								Content: fmt.Sprintf("User %d content %d", indexIdx, i),
							},
						})
					case 1:
						worker.Submit(&concurrency.Operation{
							Type: concurrency.OpSearch,
							Payload: concurrency.SearchRequest{
								Query: "content",
								Depth: 1,
								Limit: 5,
							},
						})
					case 2:
						worker.Submit(&concurrency.Operation{Type: concurrency.OpDecay})
					case 3:
						worker.Submit(&concurrency.Operation{Type: concurrency.OpGetStats})
					}
				}
			}(u)
		}

		wg.Wait()
		duration := time.Since(start)
		memEnd := getMemStats()

		opsPerSec := float64(totalOps) / duration.Seconds()
		memPerUser := (memEnd.Alloc - memStart.Alloc) / uint64(numUsers)

		t.Logf("\n--- %d Users, %d ops/user ---", numUsers, opsPerUser)
		t.Logf("Duration: %v", duration)
		t.Logf("Throughput: %.0f ops/sec", opsPerSec)
		t.Logf("Avg latency: %v/op", duration/time.Duration(totalOps))
		t.Logf("Memory increase: %s (avg %s/user)", formatBytes(memEnd.Alloc-memStart.Alloc), formatBytes(memPerUser))
	}

	// Cleanup - evict all
	pool.Shutdown()
	runtime.GC()

	memAfterCleanup := getMemStats()
	t.Logf("\nPost-cleanup: Alloc=%s", formatBytes(memAfterCleanup.Alloc))
}

// TestPerformanceMemorySizes measures memory per neuron/synapse
func TestPerformanceMemorySizes(t *testing.T) {
	t.Log("\n=== MEMORY SIZE ANALYSIS ===")

	// Single neuron size
	runtime.GC()
	memBefore := getMemStats()

	neurons := make([]*core.Neuron, 1000)
	for i := 0; i < 1000; i++ {
		neurons[i] = core.NewNeuron(fmt.Sprintf("Test content for neuron %d with some additional text", i), 10)
	}
	memAfter := getMemStats()

	neuronAlloc := allocDelta(memAfter.TotalAlloc, memBefore.TotalAlloc)
	neuronSize := neuronAlloc / 1000
	t.Logf("Average Neuron size: %s", formatBytes(neuronSize))
	runtime.KeepAlive(neurons)

	// Single synapse size
	runtime.GC()
	memBefore = getMemStats()

	synapses := make([]*core.Synapse, 1000)
	for i := 0; i < 1000; i++ {
		synapses[i] = core.NewSynapse(
			core.NeuronID(fmt.Sprintf("from-%d", i)),
			core.NeuronID(fmt.Sprintf("to-%d", i)),
			0.5,
		)
	}
	memAfter = getMemStats()

	synapseAlloc := allocDelta(memAfter.TotalAlloc, memBefore.TotalAlloc)
	synapseSize := synapseAlloc / 1000
	t.Logf("Average Synapse size: %s", formatBytes(synapseSize))
	runtime.KeepAlive(synapses)

	// Matrix size projections
	t.Log("\n--- Projection (Estimated Sizes) ---")
	projections := []struct {
		neurons  int
		synapses int
	}{
		{100, 200},
		{1000, 5000},
		{10000, 50000},
		{100000, 500000},
	}

	for _, p := range projections {
		totalSize := uint64(p.neurons)*neuronSize + uint64(p.synapses)*synapseSize
		t.Logf("%d neurons + %d synapses ≈ %s", p.neurons, p.synapses, formatBytes(totalSize))
	}
}

// TestPerformanceDecayEffectiveness measures decay behavior
func TestPerformanceDecayEffectiveness(t *testing.T) {
	tmpDir, _ := os.MkdirTemp("", "qubicdb-decay-*")
	defer os.RemoveAll(tmpDir)

	store, _ := persistence.NewStore(tmpDir, true)
	pool := concurrency.NewWorkerPool(store, core.DefaultBounds())
	defer pool.Shutdown()

	t.Log("\n=== DECAY EFFECTIVENESS ANALYSIS ===")

	worker, _ := pool.GetOrCreate("decay-test")

	// Add neurons
	var neurons []*core.Neuron
	for i := 0; i < 10; i++ {
		result, _ := worker.Submit(&concurrency.Operation{
			Type: concurrency.OpWrite,
			Payload: concurrency.AddNeuronRequest{
				Content: fmt.Sprintf("Decay test content %d", i),
			},
		})
		neurons = append(neurons, result.(*core.Neuron))
	}

	t.Log("Initial energies:")
	for i, n := range neurons {
		t.Logf("  Neuron %d: energy=%.3f", i, n.Energy)
	}

	// Simulate time passing and decay
	for round := 1; round <= 5; round++ {
		// Set last fired to past
		for _, n := range neurons {
			n.LastFiredAt = time.Now().Add(-time.Duration(round) * time.Hour)
		}

		worker.Submit(&concurrency.Operation{Type: concurrency.OpDecay})

		t.Logf("\nDecay round %d (simulated %dh ago):", round, round)
		aliveCount := 0
		for i, n := range neurons {
			alive := "alive"
			if !n.IsAlive() {
				alive = "DEAD"
			} else {
				aliveCount++
			}
			t.Logf("  Neuron %d: energy=%.3f (%s)", i, n.Energy, alive)
		}
		t.Logf("  Alive: %d/10", aliveCount)
	}
}

// BenchmarkNeuronOperations benchmarks core operations
func BenchmarkNeuronOperations(b *testing.B) {
	tmpDir, _ := os.MkdirTemp("", "qubicdb-bench-*")
	defer os.RemoveAll(tmpDir)

	store, _ := persistence.NewStore(tmpDir, true)
	pool := concurrency.NewWorkerPool(store, core.DefaultBounds())
	defer pool.Shutdown()

	worker, _ := pool.GetOrCreate("bench-user")

	// Populate
	for i := 0; i < 100; i++ {
		worker.Submit(&concurrency.Operation{
			Type: concurrency.OpWrite,
			Payload: concurrency.AddNeuronRequest{
				Content: fmt.Sprintf("Benchmark content %d", i),
			},
		})
	}

	b.Run("AddNeuron", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			worker.Submit(&concurrency.Operation{
				Type: concurrency.OpWrite,
				Payload: concurrency.AddNeuronRequest{
					Content: fmt.Sprintf("New content %d", i),
				},
			})
		}
	})

	b.Run("Search", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			worker.Submit(&concurrency.Operation{
				Type: concurrency.OpSearch,
				Payload: concurrency.SearchRequest{
					Query: "content",
					Depth: 1,
					Limit: 10,
				},
			})
		}
	})

	b.Run("Decay", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			worker.Submit(&concurrency.Operation{Type: concurrency.OpDecay})
		}
	})

	b.Run("GetStats", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			worker.Submit(&concurrency.Operation{Type: concurrency.OpGetStats})
		}
	})
}
