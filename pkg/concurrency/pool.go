package concurrency

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/qubicDB/qubicdb/pkg/core"
	"github.com/qubicDB/qubicdb/pkg/persistence"
	"github.com/qubicDB/qubicdb/pkg/sentiment"
	"github.com/qubicDB/qubicdb/pkg/vector"
)

// WorkerPool manages all brain workers with isolation
type WorkerPool struct {
	workers map[core.IndexID]*BrainWorker
	store   *persistence.Store
	bounds  core.MatrixBounds

	// Vector layer (shared across all workers)
	vectorizer        *vector.Vectorizer // nil when disabled
	vectorAlpha       float64
	vectorQueryRepeat int

	// Sentiment layer (shared across all workers)
	sentimentAnalyzer *sentiment.Analyzer // nil when disabled

	// Worker lifecycle
	maxIdleTime time.Duration

	// Concurrency control
	mu       sync.RWMutex
	createMu sync.Mutex // Prevents race during worker creation

	// Stats
	totalCreated uint64
	totalEvicted uint64

	ctx    context.Context
	cancel context.CancelFunc
}

// NewWorkerPool creates a new worker pool
func NewWorkerPool(store *persistence.Store, bounds core.MatrixBounds) *WorkerPool {
	ctx, cancel := context.WithCancel(context.Background())

	p := &WorkerPool{
		workers:     make(map[core.IndexID]*BrainWorker),
		store:       store,
		bounds:      bounds,
		maxIdleTime: 30 * time.Minute,
		ctx:         ctx,
		cancel:      cancel,
	}

	// Start background eviction
	go p.evictionLoop()

	return p
}

// GetOrCreate returns existing worker or creates new one
func (p *WorkerPool) GetOrCreate(indexID core.IndexID) (*BrainWorker, error) {
	// Fast path: check if exists
	p.mu.RLock()
	worker, ok := p.workers[indexID]
	p.mu.RUnlock()

	if ok {
		return worker, nil
	}

	// Slow path: create new worker
	p.createMu.Lock()
	defer p.createMu.Unlock()

	// Double-check after acquiring lock
	p.mu.RLock()
	worker, ok = p.workers[indexID]
	p.mu.RUnlock()

	if ok {
		return worker, nil
	}

	// Try to load from persistence
	var matrix *core.Matrix
	if p.store.Exists(indexID) {
		loaded, err := p.store.Load(indexID)
		if err == nil {
			matrix = loaded
		}
	}

	// Create new matrix if not loaded
	if matrix == nil {
		matrix = core.NewMatrix(indexID, p.bounds)
	}

	// Create worker
	worker = NewBrainWorker(indexID, matrix)
	if p.vectorizer != nil {
		worker.SetVectorizer(p.vectorizer, p.vectorAlpha, p.vectorQueryRepeat)
	}
	if p.sentimentAnalyzer != nil {
		worker.SetSentimentAnalyzer(p.sentimentAnalyzer)
	}

	p.mu.Lock()
	p.workers[indexID] = worker
	p.totalCreated++
	p.mu.Unlock()

	return worker, nil
}

// Get returns existing worker or nil
func (p *WorkerPool) Get(indexID core.IndexID) (*BrainWorker, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	worker, ok := p.workers[indexID]
	if !ok {
		return nil, fmt.Errorf("index %s not found", indexID)
	}
	return worker, nil
}

// ListIndexes returns all active index IDs
func (p *WorkerPool) ListIndexes() []string {
	p.mu.RLock()
	defer p.mu.RUnlock()
	indexes := make([]string, 0, len(p.workers))
	for id := range p.workers {
		indexes = append(indexes, string(id))
	}
	return indexes
}

// Evict removes a worker and persists its state
func (p *WorkerPool) Evict(indexID core.IndexID) error {
	p.mu.Lock()
	worker, ok := p.workers[indexID]
	if !ok {
		p.mu.Unlock()
		return nil
	}
	delete(p.workers, indexID)
	p.totalEvicted++
	p.mu.Unlock()

	// Stop worker
	worker.Stop()

	// Persist matrix
	return p.store.Save(worker.Matrix())
}

// Truncate removes an index from memory and disk without persisting the in-memory state first.
func (p *WorkerPool) Truncate(indexID core.IndexID) error {
	p.mu.Lock()
	worker, ok := p.workers[indexID]
	if ok {
		delete(p.workers, indexID)
		p.totalEvicted++
	}
	p.mu.Unlock()

	if ok {
		worker.Stop()
	}

	return p.store.Delete(indexID)
}

// evictionLoop periodically evicts idle workers
func (p *WorkerPool) evictionLoop() {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-p.ctx.Done():
			return
		case <-ticker.C:
			p.evictIdle()
		}
	}
}

// evictIdle evicts workers that have been idle too long
func (p *WorkerPool) evictIdle() {
	now := time.Now()
	toEvict := make([]core.IndexID, 0)

	p.mu.RLock()
	for id, worker := range p.workers {
		stats := worker.Stats()
		lastOp := stats["last_op"].(time.Time)
		if now.Sub(lastOp) > p.maxIdleTime {
			toEvict = append(toEvict, id)
		}
	}
	p.mu.RUnlock()

	for _, id := range toEvict {
		p.Evict(id)
	}
}

// PersistAll persists all active workers
func (p *WorkerPool) PersistAll() error {
	p.mu.RLock()
	workers := make([]*BrainWorker, 0, len(p.workers))
	for _, w := range p.workers {
		workers = append(workers, w)
	}
	p.mu.RUnlock()

	var lastErr error
	for _, w := range workers {
		if err := p.store.Save(w.Matrix()); err != nil {
			lastErr = err
		}
	}
	return lastErr
}

// Shutdown gracefully shuts down all workers
func (p *WorkerPool) Shutdown() error {
	p.cancel()

	// Persist and stop all workers
	p.mu.Lock()
	workers := make(map[core.IndexID]*BrainWorker)
	for k, v := range p.workers {
		workers[k] = v
	}
	p.workers = make(map[core.IndexID]*BrainWorker)
	p.mu.Unlock()

	var lastErr error
	for _, w := range workers {
		w.Stop()
		if err := p.store.Save(w.Matrix()); err != nil {
			lastErr = err
		}
	}

	return lastErr
}

// ActiveCount returns number of active workers
func (p *WorkerPool) ActiveCount() int {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return len(p.workers)
}

// SetVectorizer attaches a global vectorizer to the pool.
// All existing and future workers will use it.
func (p *WorkerPool) SetVectorizer(v *vector.Vectorizer, alpha float64) {
	p.SetVectorizerWithRepeat(v, alpha, 2)
}

// SetVectorizerWithRepeat attaches a global vectorizer with explicit query repeat count.
func (p *WorkerPool) SetVectorizerWithRepeat(v *vector.Vectorizer, alpha float64, queryRepeat int) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.vectorizer = v
	p.vectorAlpha = alpha
	p.vectorQueryRepeat = queryRepeat
	// Update existing workers
	for _, w := range p.workers {
		w.SetVectorizer(v, alpha, queryRepeat)
	}
}

// SetSentimentAnalyzer attaches a global sentiment analyzer to the pool.
// All existing and future workers will use it.
func (p *WorkerPool) SetSentimentAnalyzer(a *sentiment.Analyzer) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.sentimentAnalyzer = a
	for _, w := range p.workers {
		w.SetSentimentAnalyzer(a)
	}
}

// SetVectorAlpha updates the vector alpha used by existing and future workers.
func (p *WorkerPool) SetVectorAlpha(alpha float64) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.vectorAlpha = alpha
	if p.vectorizer == nil {
		return
	}
	for _, w := range p.workers {
		w.SetVectorizer(p.vectorizer, alpha, p.vectorQueryRepeat)
	}
}

// SetMaxIdleTime updates the idle eviction threshold at runtime.
func (p *WorkerPool) SetMaxIdleTime(d time.Duration) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.maxIdleTime = d
}

// SetMaxNeurons updates matrix capacity bounds for active and future indexes.
func (p *WorkerPool) SetMaxNeurons(max int) {
	p.mu.Lock()
	p.bounds.MaxNeurons = max
	workers := make([]*BrainWorker, 0, len(p.workers))
	for _, w := range p.workers {
		workers = append(workers, w)
	}
	p.mu.Unlock()

	for _, w := range workers {
		m := w.Matrix()
		m.Lock()
		m.Bounds.MaxNeurons = max
		m.Unlock()
	}
}

// Stats returns pool statistics
func (p *WorkerPool) Stats() map[string]any {
	p.mu.RLock()
	defer p.mu.RUnlock()

	workerStats := make(map[string]any)
	for id, w := range p.workers {
		workerStats[string(id)] = w.Stats()
	}

	return map[string]any{
		"active_workers": len(p.workers),
		"total_created":  p.totalCreated,
		"total_evicted":  p.totalEvicted,
		"max_idle_time":  p.maxIdleTime.String(),
		"worker_details": workerStats,
	}
}

// ForEach executes a function on each worker
func (p *WorkerPool) ForEach(fn func(core.IndexID, *BrainWorker)) {
	p.mu.RLock()
	workers := make(map[core.IndexID]*BrainWorker)
	for k, v := range p.workers {
		workers[k] = v
	}
	p.mu.RUnlock()

	for id, w := range workers {
		fn(id, w)
	}
}
