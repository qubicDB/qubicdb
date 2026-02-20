package concurrency

import (
	"context"
	"sync"
	"time"

	"github.com/qubicDB/qubicdb/pkg/core"
	"github.com/qubicDB/qubicdb/pkg/engine"
	"github.com/qubicDB/qubicdb/pkg/sentiment"
	"github.com/qubicDB/qubicdb/pkg/synapse"
	"github.com/qubicDB/qubicdb/pkg/vector"
)

// Operation types for the worker
type OpType int

const (
	// Brain-like naming (primary)
	OpWrite       OpType = iota // Add/create neuron (memory formation)
	OpRead                      // Get neuron (memory retrieval)
	OpSearch                    // Search neurons (associative recall)
	OpTouch                     // Update neuron (memory modification)
	OpForget                    // Delete neuron (memory erasure)
	OpRecall                    // List neurons (memory scanning)
	OpFire                      // Activate neuron (neural firing)
	OpDecay                     // Energy decay (forgetting curve)
	OpConsolidate               // Memory consolidation (depth increase)
	OpPrune                     // Remove dead neurons (synaptic pruning)
	OpReorg                     // Reorganize matrix (neural plasticity)
	OpGetStats                  // Get statistics
	OpShutdown                  // Shutdown worker
)

// Operation represents a queued operation
type Operation struct {
	Type    OpType
	Payload any
	Result  chan any
	Error   chan error
}

// BrainWorker is a dedicated goroutine per user brain
type BrainWorker struct {
	indexID core.IndexID
	matrix  *core.Matrix
	engine  *engine.MatrixEngine
	hebbian *synapse.HebbianEngine

	// Operation queue
	ops chan *Operation

	// Lifecycle
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup

	// Stats
	opsProcessed uint64
	lastOp       time.Time

	mu sync.RWMutex
}

// NewBrainWorker creates a new worker for a user
func NewBrainWorker(indexID core.IndexID, matrix *core.Matrix) *BrainWorker {
	ctx, cancel := context.WithCancel(context.Background())

	w := &BrainWorker{
		indexID: indexID,
		matrix:  matrix,
		engine:  engine.NewMatrixEngine(matrix),
		hebbian: synapse.NewHebbianEngine(matrix),
		ops:     make(chan *Operation, 1000), // Buffered for burst handling
		ctx:     ctx,
		cancel:  cancel,
		lastOp:  time.Now(),
	}

	// Start worker goroutine
	w.wg.Add(1)
	go w.run()

	return w
}

// run is the main worker loop
func (w *BrainWorker) run() {
	defer w.wg.Done()

	for {
		select {
		case <-w.ctx.Done():
			// Drain remaining operations
			w.drainOps()
			return

		case op := <-w.ops:
			w.processOp(op)
		}
	}
}

// processOp handles a single operation
func (w *BrainWorker) processOp(op *Operation) {
	w.mu.Lock()
	w.opsProcessed++
	w.lastOp = time.Now()
	w.mu.Unlock()

	var result any
	var err error

	switch op.Type {
	case OpWrite: // Memory formation - create new neuron
		req := op.Payload.(AddNeuronRequest)
		result, err = w.engine.AddNeuron(req.Content, req.ParentID, req.Metadata)
		if err == nil {
			w.hebbian.OnNeuronFired(result.(*core.Neuron).ID)
		}

	case OpRead: // Memory retrieval - get specific neuron
		id := op.Payload.(core.NeuronID)
		result, err = w.engine.GetNeuron(id)
		if err == nil {
			w.hebbian.OnNeuronFired(id)
		}

	case OpSearch: // Associative recall - search by content
		req := op.Payload.(SearchRequest)
		neurons := w.engine.Search(req.Query, req.Depth, req.Limit, req.Metadata, req.Strict)
		for _, n := range neurons {
			w.hebbian.OnNeuronFired(n.ID)
		}
		result = neurons

	case OpTouch: // Memory modification - update content
		req := op.Payload.(UpdateNeuronRequest)
		err = w.engine.UpdateNeuron(req.ID, req.Content)

	case OpForget: // Memory erasure - delete neuron
		id := op.Payload.(core.NeuronID)
		err = w.engine.DeleteNeuron(id)

	case OpRecall: // Memory scanning - list neurons
		req := op.Payload.(ListNeuronsRequest)
		result = w.engine.ListNeurons(req.Offset, req.Limit, req.DepthFilter)

	case OpFire:
		id := op.Payload.(core.NeuronID)
		if n, e := w.engine.GetNeuron(id); e == nil {
			n.Fire()
			w.hebbian.OnNeuronFired(id)
		}

	case OpDecay:
		// Apply decay to all neurons
		for _, n := range w.matrix.Neurons {
			n.Decay(w.matrix.DecayRate)
		}
		w.hebbian.DecayAll()
		w.hebbian.PruneDeadSynapses()

	case OpConsolidate:
		result = w.consolidate()

	case OpPrune:
		result = w.prune()

	case OpReorg:
		w.reorg()

	case OpGetStats:
		result = w.engine.GetStats()

	case OpShutdown:
		w.cancel()
		return
	}

	// Send results
	if op.Result != nil {
		op.Result <- result
	}
	if op.Error != nil {
		op.Error <- err
	}
}

// consolidate moves mature neurons to deeper layers
func (w *BrainWorker) consolidate() int {
	consolidated := 0

	for _, n := range w.matrix.Neurons {
		if n.ShouldConsolidate(10, 30*time.Minute) {
			n.Depth++
			consolidated++
		}
	}

	// Self-tune Hebbian parameters
	w.hebbian.SelfTune()

	w.matrix.LastConsolidation = time.Now()
	w.matrix.Version++

	return consolidated
}

// prune removes dead neurons and synapses
func (w *BrainWorker) prune() int {
	pruned := 0

	// Collect dead neurons
	deadNeurons := make([]core.NeuronID, 0)
	for id, n := range w.matrix.Neurons {
		if !n.IsAlive() {
			deadNeurons = append(deadNeurons, id)
		}
	}

	// Delete them
	for _, id := range deadNeurons {
		if err := w.engine.DeleteNeuron(id); err == nil {
			pruned++
		}
	}

	// Also prune dead synapses
	pruned += w.hebbian.PruneDeadSynapses()

	return pruned
}

// reorg performs spatial reorganization of the matrix using fractal clustering.
// Called by the reorg daemon — runs outside any matrix lock, safe to acquire locks internally.
func (w *BrainWorker) reorg() {
	w.hebbian.UpdateFractalClusters()
	w.matrix.Version++
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

// drainOps processes remaining operations before shutdown
func (w *BrainWorker) drainOps() {
	for {
		select {
		case op := <-w.ops:
			if op.Type == OpShutdown {
				return
			}
			w.processOp(op)
		default:
			return
		}
	}
}

// Submit queues an operation and waits for result
func (w *BrainWorker) Submit(op *Operation) (any, error) {
	op.Result = make(chan any, 1)
	op.Error = make(chan error, 1)

	select {
	case w.ops <- op:
	case <-w.ctx.Done():
		return nil, context.Canceled
	}

	select {
	case result := <-op.Result:
		err := <-op.Error
		return result, err
	case <-w.ctx.Done():
		return nil, context.Canceled
	}
}

// SubmitAsync queues an operation without waiting
func (w *BrainWorker) SubmitAsync(op *Operation) {
	select {
	case w.ops <- op:
	default:
		// Queue full, drop operation (could log this)
	}
}

// Stop gracefully stops the worker
func (w *BrainWorker) Stop() {
	w.cancel()
	w.wg.Wait()
}

// Matrix returns the underlying matrix
func (w *BrainWorker) Matrix() *core.Matrix {
	return w.matrix
}

// SetVectorizer attaches a vectorizer to the underlying engine for
// auto-embedding on write and hybrid scoring on search.
func (w *BrainWorker) SetVectorizer(v *vector.Vectorizer, alpha float64, queryRepeat int) {
	w.engine.SetVectorizer(v)
	w.engine.SetAlpha(alpha)
	w.engine.SetQueryRepeat(queryRepeat)
}

// SetSentimentAnalyzer attaches a sentiment analyzer to the underlying engine
// for auto-labeling on write and sentiment-aware scoring on search.
func (w *BrainWorker) SetSentimentAnalyzer(a *sentiment.Analyzer) {
	w.engine.SetSentimentAnalyzer(a)
}

// Stats returns worker stats
func (w *BrainWorker) Stats() map[string]any {
	w.mu.RLock()
	defer w.mu.RUnlock()

	return map[string]any{
		"index_id":       w.indexID,
		"ops_processed":  w.opsProcessed,
		"last_op":        w.lastOp,
		"queue_length":   len(w.ops),
		"queue_capacity": cap(w.ops),
	}
}

// Request types
type AddNeuronRequest struct {
	Content  string
	ParentID *core.NeuronID
	Metadata map[string]string
}

type SearchRequest struct {
	Query    string
	Depth    int
	Limit    int
	Metadata map[string]string
	Strict   bool
}

type UpdateNeuronRequest struct {
	ID      core.NeuronID
	Content string
}

type ListNeuronsRequest struct {
	Offset      int
	Limit       int
	DepthFilter *int
}
