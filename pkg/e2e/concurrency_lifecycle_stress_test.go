package e2e

import (
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/qubicDB/qubicdb/pkg/concurrency"
	"github.com/qubicDB/qubicdb/pkg/core"
	"github.com/qubicDB/qubicdb/pkg/daemon"
	"github.com/qubicDB/qubicdb/pkg/lifecycle"
	"github.com/qubicDB/qubicdb/pkg/persistence"
)

// TestStressConcurrentLifecycleManyUsersNoDeadlock runs a mixed-operation
// concurrent load with lifecycle transitions to catch deadlocks and long stalls.
func TestStressConcurrentLifecycleManyUsersNoDeadlock(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "qubicdb-stress-lifecycle-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	durability := persistence.DefaultDurabilityConfig()
	durability.FsyncPolicy = persistence.FsyncPolicyInterval
	durability.FsyncInterval = 100 * time.Millisecond

	store, err := persistence.NewStoreWithDurability(tmpDir, true, durability)
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}

	pool := concurrency.NewWorkerPool(store, core.DefaultBounds())
	lm := lifecycle.NewManager()
	lm.SetThresholds(40*time.Millisecond, 120*time.Millisecond, 280*time.Millisecond)
	lm.StartMonitor(10 * time.Millisecond)

	dm := daemon.NewDaemonManager(pool, lm, store)
	dm.SetIntervals(
		25*time.Millisecond,
		50*time.Millisecond,
		75*time.Millisecond,
		100*time.Millisecond,
		150*time.Millisecond,
	)
	dm.Start()

	defer dm.Stop()
	defer lm.Stop()
	defer pool.Shutdown()

	const (
		numUsers   = 80
		opsPerUser = 120
		timeout    = 25 * time.Second
	)

	start := time.Now()
	var wg sync.WaitGroup
	errCh := make(chan error, numUsers)

	for u := 0; u < numUsers; u++ {
		wg.Add(1)
		go func(userIdx int) {
			defer wg.Done()

			indexID := core.IndexID(fmt.Sprintf("stress-lifecycle-user-%d", userIdx))
			worker, err := pool.GetOrCreate(indexID)
			if err != nil {
				errCh <- fmt.Errorf("GetOrCreate(%s): %w", indexID, err)
				return
			}

			for i := 0; i < opsPerUser; i++ {
				lm.RecordActivity(indexID)

				switch i % 7 {
				case 0:
					if _, err := worker.Submit(&concurrency.Operation{
						Type: concurrency.OpWrite,
						Payload: concurrency.AddNeuronRequest{
							Content: fmt.Sprintf("user %d workload content %d", userIdx, i),
						},
					}); err != nil {
						errCh <- fmt.Errorf("write failed for %s: %w", indexID, err)
						return
					}
				case 1:
					if _, err := worker.Submit(&concurrency.Operation{
						Type: concurrency.OpSearch,
						Payload: concurrency.SearchRequest{
							Query: "workload content",
							Depth: 2,
							Limit: 10,
						},
					}); err != nil {
						errCh <- fmt.Errorf("search failed for %s: %w", indexID, err)
						return
					}
				case 2:
					if _, err := worker.Submit(&concurrency.Operation{Type: concurrency.OpDecay}); err != nil {
						errCh <- fmt.Errorf("decay failed for %s: %w", indexID, err)
						return
					}
				case 3:
					if _, err := worker.Submit(&concurrency.Operation{Type: concurrency.OpConsolidate}); err != nil {
						errCh <- fmt.Errorf("consolidate failed for %s: %w", indexID, err)
						return
					}
				case 4:
					if _, err := worker.Submit(&concurrency.Operation{Type: concurrency.OpGetStats}); err != nil {
						errCh <- fmt.Errorf("stats failed for %s: %w", indexID, err)
						return
					}
				case 5:
					lm.ForceSleep(indexID)
					_ = lm.CheckAndTransition(indexID)
				case 6:
					lm.ForceWake(indexID)
					_ = lm.CheckAndTransition(indexID)
				}
			}
		}(u)
	}

	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		// all operations completed
	case <-time.After(timeout):
		t.Fatalf("stress run exceeded timeout (%v), potential deadlock/stall", timeout)
	}

	close(errCh)
	for err := range errCh {
		if err != nil {
			t.Fatal(err)
		}
	}

	if err := pool.PersistAll(); err != nil {
		t.Fatalf("persist all failed: %v", err)
	}
	if err := store.FlushAll(); err != nil {
		t.Fatalf("flush all failed: %v", err)
	}

	if got := pool.ActiveCount(); got != numUsers {
		t.Fatalf("expected %d active workers, got %d", numUsers, got)
	}

	restarted, err := persistence.NewStoreWithDurability(tmpDir, true, durability)
	if err != nil {
		t.Fatalf("failed to restart store: %v", err)
	}
	if got := len(restarted.ListIndexes()); got != numUsers {
		t.Fatalf("expected %d persisted indexes after restart, got %d", numUsers, got)
	}

	duration := time.Since(start)
	totalOps := numUsers * opsPerUser
	t.Logf("stress lifecycle completed: users=%d ops=%d duration=%v throughput=%.0f ops/sec",
		numUsers,
		totalOps,
		duration,
		float64(totalOps)/duration.Seconds(),
	)
	t.Logf("lifecycle state distribution: %+v", lm.Stats()["state_distribution"])
}
