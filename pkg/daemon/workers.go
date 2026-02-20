package daemon

import (
	"context"
	"log"
	"sync"
	"time"

	"github.com/qubicDB/qubicdb/pkg/concurrency"
	"github.com/qubicDB/qubicdb/pkg/core"
	"github.com/qubicDB/qubicdb/pkg/lifecycle"
	"github.com/qubicDB/qubicdb/pkg/persistence"
)

// DaemonManager manages all background daemons
type DaemonManager struct {
	pool      *concurrency.WorkerPool
	lifecycle *lifecycle.Manager
	store     *persistence.Store

	// Daemon intervals
	decayInterval       time.Duration
	consolidateInterval time.Duration
	pruneInterval       time.Duration
	persistInterval     time.Duration
	reorgInterval       time.Duration
	intervalMu          sync.RWMutex

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// NewDaemonManager creates a new daemon manager
func NewDaemonManager(
	pool *concurrency.WorkerPool,
	lm *lifecycle.Manager,
	store *persistence.Store,
) *DaemonManager {
	ctx, cancel := context.WithCancel(context.Background())

	return &DaemonManager{
		pool:                pool,
		lifecycle:           lm,
		store:               store,
		decayInterval:       1 * time.Minute,
		consolidateInterval: 5 * time.Minute,
		pruneInterval:       10 * time.Minute,
		persistInterval:     1 * time.Minute,
		reorgInterval:       15 * time.Minute,
		ctx:                 ctx,
		cancel:              cancel,
	}
}

// Start starts all daemon workers
func (dm *DaemonManager) Start() {
	dm.wg.Add(5)

	go dm.decayDaemon()
	go dm.consolidateDaemon()
	go dm.pruneDaemon()
	go dm.persistDaemon()
	go dm.reorgDaemon()

	log.Println("🧠 Daemon manager started")
}

// Stop stops all daemons gracefully
func (dm *DaemonManager) Stop() {
	dm.cancel()
	dm.wg.Wait()
	log.Println("🧠 Daemon manager stopped")
}

// decayDaemon applies continuous energy decay
func (dm *DaemonManager) decayDaemon() {
	defer dm.wg.Done()

	for dm.waitInterval(dm.getDecayInterval()) {
		dm.pool.ForEach(func(indexID core.IndexID, worker *concurrency.BrainWorker) {
			// Only decay active/idle brains, not sleeping ones
			state := dm.lifecycle.GetState(indexID)
			if state == core.StateActive || state == core.StateIdle {
				worker.SubmitAsync(&concurrency.Operation{Type: concurrency.OpDecay})
			}
		})
	}
}

// consolidateDaemon moves mature memories to deeper layers
func (dm *DaemonManager) consolidateDaemon() {
	defer dm.wg.Done()

	for dm.waitInterval(dm.getConsolidateInterval()) {
		// Consolidate sleeping brains (like real sleep consolidation)
		sleeping := dm.lifecycle.GetSleepingUsers()
		for _, indexID := range sleeping {
			worker, err := dm.pool.Get(indexID)
			if err == nil && worker != nil {
				result, _ := worker.Submit(&concurrency.Operation{
					Type: concurrency.OpConsolidate,
				})
				if count, ok := result.(int); ok && count > 0 {
					log.Printf("🌙 Index %s: consolidated %d neurons", indexID, count)
				}
			}
		}
	}
}

// pruneDaemon removes dead neurons and synapses
func (dm *DaemonManager) pruneDaemon() {
	defer dm.wg.Done()

	for dm.waitInterval(dm.getPruneInterval()) {
		dm.pool.ForEach(func(indexID core.IndexID, worker *concurrency.BrainWorker) {
			// Use worker operation to safely prune
			result, err := worker.Submit(&concurrency.Operation{
				Type: concurrency.OpPrune,
			})
			if err == nil {
				if count, ok := result.(int); ok && count > 0 {
					log.Printf("🧹 Index %s: pruned %d dead neurons", indexID, count)
				}
			}
		})
	}
}

// persistDaemon periodically saves active matrices
func (dm *DaemonManager) persistDaemon() {
	defer dm.wg.Done()

	for dm.waitInterval(dm.getPersistInterval()) {
		// Persist all modified matrices
		dm.pool.ForEach(func(indexID core.IndexID, worker *concurrency.BrainWorker) {
			if err := dm.store.SaveAsync(worker.Matrix()); err != nil {
				log.Printf("persist daemon: async save failed for %s: %v", indexID, err)
			}
		})
		dm.store.FlushAll()
	}

	// Final persist on shutdown
	dm.pool.PersistAll()
}

// reorgDaemon reorganizes spatial positions (sleep-like reorg)
func (dm *DaemonManager) reorgDaemon() {
	defer dm.wg.Done()

	for dm.waitInterval(dm.getReorgInterval()) {
		// Only reorg sleeping brains
		sleeping := dm.lifecycle.GetSleepingUsers()
		for _, indexID := range sleeping {
			worker, err := dm.pool.Get(indexID)
			if err == nil && worker != nil {
				// Use worker operation for thread-safe reorg
				worker.SubmitAsync(&concurrency.Operation{
					Type: concurrency.OpReorg,
				})
			}
		}
	}
}

func (dm *DaemonManager) waitInterval(interval time.Duration) bool {
	timer := time.NewTimer(interval)
	defer timer.Stop()
	select {
	case <-dm.ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}

func (dm *DaemonManager) getDecayInterval() time.Duration {
	dm.intervalMu.RLock()
	defer dm.intervalMu.RUnlock()
	return dm.decayInterval
}

func (dm *DaemonManager) getConsolidateInterval() time.Duration {
	dm.intervalMu.RLock()
	defer dm.intervalMu.RUnlock()
	return dm.consolidateInterval
}

func (dm *DaemonManager) getPruneInterval() time.Duration {
	dm.intervalMu.RLock()
	defer dm.intervalMu.RUnlock()
	return dm.pruneInterval
}

func (dm *DaemonManager) getPersistInterval() time.Duration {
	dm.intervalMu.RLock()
	defer dm.intervalMu.RUnlock()
	return dm.persistInterval
}

func (dm *DaemonManager) getReorgInterval() time.Duration {
	dm.intervalMu.RLock()
	defer dm.intervalMu.RUnlock()
	return dm.reorgInterval
}

func clamp(val, min, max float64) float64 {
	if val < min {
		return min
	}
	if val > max {
		return max
	}
	return val
}

// SetIntervals configures daemon intervals
func (dm *DaemonManager) SetIntervals(
	decay, consolidate, prune, persist, reorg time.Duration,
) {
	dm.intervalMu.Lock()
	defer dm.intervalMu.Unlock()
	dm.decayInterval = decay
	dm.consolidateInterval = consolidate
	dm.pruneInterval = prune
	dm.persistInterval = persist
	dm.reorgInterval = reorg
}

// Stats returns daemon statistics
func (dm *DaemonManager) Stats() map[string]any {
	dm.intervalMu.RLock()
	defer dm.intervalMu.RUnlock()
	return map[string]any{
		"decay_interval":       dm.decayInterval.String(),
		"consolidate_interval": dm.consolidateInterval.String(),
		"prune_interval":       dm.pruneInterval.String(),
		"persist_interval":     dm.persistInterval.String(),
		"reorg_interval":       dm.reorgInterval.String(),
	}
}
