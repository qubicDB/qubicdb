package synapse

import (
	"math"
	"sync"
	"time"

	"github.com/denizumutdereli/qubicdb/pkg/core"
)

// HebbianEngine implements Hebbian learning for synapse formation
// "Neurons that fire together, wire together"
type HebbianEngine struct {
	matrix *core.Matrix

	// Co-activation tracking
	recentFires        map[core.NeuronID]time.Time
	coActivationWindow time.Duration

	// Learning parameters (self-tuning)
	learningRate         float64
	forgettingRate       float64
	minWeightToForm      float64
	maxSynapsesPerNeuron int

	mu sync.Mutex
}

// NewHebbianEngine creates a new Hebbian learning engine
func NewHebbianEngine(matrix *core.Matrix) *HebbianEngine {
	return &HebbianEngine{
		matrix:               matrix,
		recentFires:          make(map[core.NeuronID]time.Time),
		coActivationWindow:   5 * time.Second, // Neurons firing within 5s are "together"
		learningRate:         0.1,
		forgettingRate:       0.01,
		minWeightToForm:      0.2,
		maxSynapsesPerNeuron: 50,
	}
}

// OnNeuronFired is called whenever a neuron fires
// It checks for co-activation with recently fired neurons
func (h *HebbianEngine) OnNeuronFired(neuronID core.NeuronID) {
	h.mu.Lock()
	defer h.mu.Unlock()

	now := time.Now()

	// Find co-activated neurons (fired within the window)
	coActivated := make([]core.NeuronID, 0)
	for id, firedAt := range h.recentFires {
		if id == neuronID {
			continue
		}
		if now.Sub(firedAt) <= h.coActivationWindow {
			coActivated = append(coActivated, id)
		}
	}

	// Record this fire
	h.recentFires[neuronID] = now

	// Clean old fires
	for id, firedAt := range h.recentFires {
		if now.Sub(firedAt) > h.coActivationWindow*2 {
			delete(h.recentFires, id)
		}
	}

	// Strengthen/create synapses with co-activated neurons
	for _, coID := range coActivated {
		h.strengthenOrCreate(neuronID, coID)
	}
}

// strengthenOrCreate either strengthens existing synapse or creates new one
func (h *HebbianEngine) strengthenOrCreate(from, to core.NeuronID) {
	h.matrix.RLock()

	// Check if synapse exists (either direction for bidirectional)
	synID := core.NewSynapseID(from, to)
	syn, exists := h.matrix.Synapses[synID]
	if !exists {
		synID = core.NewSynapseID(to, from)
		syn, exists = h.matrix.Synapses[synID]
	}
	h.matrix.RUnlock()

	if exists {
		// Strengthen existing synapse
		delta := h.learningRate * (1 - syn.Weight) // Asymptotic approach to 1
		syn.Strengthen(delta)

		// Fractal clustering runs in a goroutine so the hot write path is not
		// blocked. The goroutine uses snapshot-based reads (no matrix lock held
		// during neuron writes) so it cannot deadlock with concurrent writers.
		go h.updateFractalCluster(from, to, delta*0.1)
	} else {
		// Create new synapse if under limit
		h.matrix.RLock()
		fromCount := len(h.matrix.Adjacency[from])
		toCount := len(h.matrix.Adjacency[to])
		h.matrix.RUnlock()

		if fromCount < h.maxSynapsesPerNeuron && toCount < h.maxSynapsesPerNeuron {
			h.createSynapse(from, to)
		}
	}
}

// createSynapse creates a new synapse between two neurons
func (h *HebbianEngine) createSynapse(from, to core.NeuronID) {
	h.matrix.Lock()
	defer h.matrix.Unlock()

	synID := core.NewSynapseID(from, to)
	if _, exists := h.matrix.Synapses[synID]; exists {
		return
	}

	syn := core.NewSynapse(from, to, h.minWeightToForm)
	h.matrix.Synapses[synID] = syn

	// Update adjacency lists (bidirectional)
	h.matrix.Adjacency[from] = append(h.matrix.Adjacency[from], to)
	h.matrix.Adjacency[to] = append(h.matrix.Adjacency[to], from)

	h.matrix.ModifiedAt = time.Now()
	h.matrix.Version++
}

// updateFractalCluster implements fractal spatial clustering for co-activated neurons.
//
// When two neurons fire together their synapse strengthens (Hebbian). This function
// mirrors that in the spatial domain:
//
//  1. Attraction: the pair moves toward their shared midpoint (cohesion within cluster).
//  2. Cluster-centroid pull: each neuron is also pulled toward the centroid of ALL its
//     current neighbours, reinforcing the cluster's internal density.
//  3. Repulsion: neurons that are NOT connected are gently pushed apart, creating
//     clear separation between clusters (fractal boundary sharpening).
//
// The result is self-similar clustering at multiple scales: tightly connected neurons
// collapse into dense cores, weakly connected groups form looser shells around them,
// and unconnected neurons drift to distinct regions — a fractal-like topology.
func (h *HebbianEngine) updateFractalCluster(id1, id2 core.NeuronID, strength float64) {
	h.matrix.RLock()
	n1, ok1 := h.matrix.Neurons[id1]
	n2, ok2 := h.matrix.Neurons[id2]
	adj1 := append([]core.NeuronID(nil), h.matrix.Adjacency[id1]...)
	adj2 := append([]core.NeuronID(nil), h.matrix.Adjacency[id2]...)
	h.matrix.RUnlock()

	if !ok1 || !ok2 {
		return
	}

	// --- 1. Pairwise attraction ---
	// Lock in consistent ID order to prevent AB-BA deadlock across concurrent goroutines.
	first, second := n1, n2
	if id1 > id2 {
		first, second = n2, n1
	}
	first.Lock()
	second.Lock()
	dim := min(len(n1.Position), len(n2.Position))
	for i := 0; i < dim; i++ {
		mid := (n1.Position[i] + n2.Position[i]) / 2
		n1.Position[i] += (mid - n1.Position[i]) * strength
		n2.Position[i] += (mid - n2.Position[i]) * strength
	}
	second.Unlock()
	first.Unlock()

	// --- 2. Cluster-centroid pull ---
	// Pull each neuron toward the centroid of its neighbourhood.
	h.pullToCentroid(id1, adj1, strength*0.5)
	h.pullToCentroid(id2, adj2, strength*0.5)

	// --- 3. Inter-cluster repulsion ---
	// Push unconnected neurons away to sharpen cluster boundaries.
	h.repelUnconnected(id1, adj1, strength*0.3)
	h.repelUnconnected(id2, adj2, strength*0.3)
}

// pullToCentroid moves neuron `id` toward the centroid of its neighbours.
// Lock discipline: matrix RLock is held only while collecting pointers and
// reading neighbour positions into a local centroid accumulator. It is
// released before any neuron write-lock is acquired.
func (h *HebbianEngine) pullToCentroid(id core.NeuronID, neighbours []core.NeuronID, strength float64) {
	if len(neighbours) == 0 {
		return
	}

	// Phase 1: snapshot under matrix RLock — no neuron write-locks held.
	h.matrix.RLock()
	n, ok := h.matrix.Neurons[id]
	if !ok {
		h.matrix.RUnlock()
		return
	}
	n.RLock()
	dim := len(n.Position)
	n.RUnlock()

	centroid := make([]float64, dim)
	count := 0
	for _, nid := range neighbours {
		if nb, ok2 := h.matrix.Neurons[nid]; ok2 {
			nb.RLock()
			for i := 0; i < dim && i < len(nb.Position); i++ {
				centroid[i] += nb.Position[i]
			}
			nb.RUnlock()
			count++
		}
	}
	h.matrix.RUnlock() // released before any write-lock below

	if count == 0 {
		return
	}
	for i := range centroid {
		centroid[i] /= float64(count)
	}

	// Phase 2: apply update — no matrix lock held.
	n.Lock()
	defer n.Unlock()
	for i := 0; i < dim; i++ {
		n.Position[i] += (centroid[i] - n.Position[i]) * strength
		if n.Position[i] > 1 {
			n.Position[i] = 1
		} else if n.Position[i] < -1 {
			n.Position[i] = -1
		}
	}
}

// repelUnconnected pushes a small random sample of unconnected neurons away from `id`
// to maintain clear inter-cluster separation.
// All matrix reads are completed before any neuron write-lock is acquired to
// prevent lock-order inversions with other goroutines.
func (h *HebbianEngine) repelUnconnected(id core.NeuronID, neighbours []core.NeuronID, strength float64) {
	connected := make(map[core.NeuronID]bool, len(neighbours)+1)
	connected[id] = true
	for _, nid := range neighbours {
		connected[nid] = true
	}

	// --- Phase 1: read everything under matrix RLock, hold no neuron locks ---
	h.matrix.RLock()
	n, ok := h.matrix.Neurons[id]
	if !ok {
		h.matrix.RUnlock()
		return
	}
	n.RLock()
	nPos := append([]float64(nil), n.Position...)
	dim := len(nPos)
	n.RUnlock()

	type candidate struct {
		neuron *core.Neuron
	}
	candidates := make([]candidate, 0, 5)
	for nid, nb := range h.matrix.Neurons {
		if connected[nid] {
			continue
		}
		candidates = append(candidates, candidate{nb})
		if len(candidates) >= 5 {
			break
		}
	}
	h.matrix.RUnlock()

	// --- Phase 2: update candidate positions, no matrix lock held ---
	for _, c := range candidates {
		c.neuron.Lock()
		for i := 0; i < dim && i < len(c.neuron.Position); i++ {
			diff := c.neuron.Position[i] - nPos[i]
			c.neuron.Position[i] += diff * strength
			if c.neuron.Position[i] > 1 {
				c.neuron.Position[i] = 1
			} else if c.neuron.Position[i] < -1 {
				c.neuron.Position[i] = -1
			}
		}
		c.neuron.Unlock()
	}
}

// UpdateFractalClusters runs a full pass of fractal spatial clustering over
// all synapses whose weight exceeds 0.3. Called by the reorg daemon — safe
// to call concurrently with read/write operations because each sub-call
// acquires only neuron-level locks (no matrix lock held during writes).
func (h *HebbianEngine) UpdateFractalClusters() {
	h.matrix.RLock()
	type pair struct {
		from, to core.NeuronID
		weight   float64
	}
	pairs := make([]pair, 0, len(h.matrix.Synapses))
	for _, syn := range h.matrix.Synapses {
		if syn.Weight > 0.3 {
			pairs = append(pairs, pair{syn.FromID, syn.ToID, syn.Weight})
		}
	}
	h.matrix.RUnlock()

	for _, p := range pairs {
		h.updateFractalCluster(p.from, p.to, p.weight*0.05)
	}
}

// DecayAll applies decay to all synapses
func (h *HebbianEngine) DecayAll() {
	h.matrix.RLock()
	synapses := make([]*core.Synapse, 0, len(h.matrix.Synapses))
	for _, s := range h.matrix.Synapses {
		synapses = append(synapses, s)
	}
	h.matrix.RUnlock()

	for _, s := range synapses {
		s.Decay(h.forgettingRate)
	}
}

// PruneDeadSynapses removes synapses that have decayed below threshold
func (h *HebbianEngine) PruneDeadSynapses() int {
	h.matrix.Lock()
	defer h.matrix.Unlock()

	pruned := 0
	for synID, syn := range h.matrix.Synapses {
		if !syn.IsAlive() {
			// Remove from adjacency
			h.removeFromAdjacency(syn.FromID, syn.ToID)
			h.removeFromAdjacency(syn.ToID, syn.FromID)

			delete(h.matrix.Synapses, synID)
			pruned++
		}
	}

	if pruned > 0 {
		h.matrix.ModifiedAt = time.Now()
		h.matrix.Version++
	}

	return pruned
}

// removeFromAdjacency removes 'remove' from the adjacency list of 'from'
func (h *HebbianEngine) removeFromAdjacency(from, remove core.NeuronID) {
	adj := h.matrix.Adjacency[from]
	filtered := make([]core.NeuronID, 0, len(adj))
	for _, id := range adj {
		if id != remove {
			filtered = append(filtered, id)
		}
	}
	h.matrix.Adjacency[from] = filtered
}

// GetSynapseWeight returns the weight between two neurons
func (h *HebbianEngine) GetSynapseWeight(from, to core.NeuronID) float64 {
	h.matrix.RLock()
	defer h.matrix.RUnlock()

	synID := core.NewSynapseID(from, to)
	if syn, ok := h.matrix.Synapses[synID]; ok {
		return syn.Weight
	}

	synID = core.NewSynapseID(to, from)
	if syn, ok := h.matrix.Synapses[synID]; ok {
		return syn.Weight
	}

	return 0
}

// GetConnectedNeurons returns all neurons connected to a given neuron
func (h *HebbianEngine) GetConnectedNeurons(neuronID core.NeuronID, minWeight float64) []core.NeuronID {
	h.matrix.RLock()
	defer h.matrix.RUnlock()

	adj := h.matrix.Adjacency[neuronID]
	result := make([]core.NeuronID, 0, len(adj))

	for _, connID := range adj {
		weight := h.getWeightUnsafe(neuronID, connID)
		if weight >= minWeight {
			result = append(result, connID)
		}
	}

	return result
}

func (h *HebbianEngine) getWeightUnsafe(from, to core.NeuronID) float64 {
	synID := core.NewSynapseID(from, to)
	if syn, ok := h.matrix.Synapses[synID]; ok {
		return syn.Weight
	}
	synID = core.NewSynapseID(to, from)
	if syn, ok := h.matrix.Synapses[synID]; ok {
		return syn.Weight
	}
	return 0
}

// SelfTune adjusts learning parameters based on matrix state
func (h *HebbianEngine) SelfTune() {
	h.matrix.RLock()
	neuronCount := len(h.matrix.Neurons)
	synapseCount := len(h.matrix.Synapses)
	h.matrix.RUnlock()

	if neuronCount == 0 {
		return
	}

	// Adjust synapse limit based on neuron count
	avgSynapses := float64(synapseCount*2) / float64(neuronCount) // *2 for bidirectional

	// If too few synapses, increase learning rate
	if avgSynapses < 3 {
		h.learningRate = math.Min(0.3, h.learningRate*1.1)
		h.minWeightToForm = math.Max(0.1, h.minWeightToForm*0.9)
	}

	// If too many synapses, decrease learning rate
	if avgSynapses > 20 {
		h.learningRate = math.Max(0.05, h.learningRate*0.9)
		h.minWeightToForm = math.Min(0.5, h.minWeightToForm*1.1)
	}
}

// Stats returns Hebbian engine statistics
func (h *HebbianEngine) Stats() map[string]any {
	h.mu.Lock()
	defer h.mu.Unlock()

	return map[string]any{
		"recent_fires_count":    len(h.recentFires),
		"learning_rate":         h.learningRate,
		"forgetting_rate":       h.forgettingRate,
		"min_weight_to_form":    h.minWeightToForm,
		"co_activation_window":  h.coActivationWindow.String(),
		"max_synapses_per_node": h.maxSynapsesPerNeuron,
	}
}
