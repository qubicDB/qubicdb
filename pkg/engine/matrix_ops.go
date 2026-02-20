package engine

import (
	"log"
	"math"
	"math/rand"
	"sort"
	"strings"
	"time"

	"github.com/qubicDB/qubicdb/pkg/core"
	"github.com/qubicDB/qubicdb/pkg/sentiment"
	"github.com/qubicDB/qubicdb/pkg/vector"
)

// MatrixEngine handles all matrix operations
type MatrixEngine struct {
	matrix            *core.Matrix
	vectorizer        *vector.Vectorizer  // nil when vector layer is disabled
	alpha             float64             // vector score weight for hybrid search
	queryRepeat       int                 // query repetition count for embedding
	sentimentAnalyzer *sentiment.Analyzer // nil when sentiment layer is disabled
}

// NewMatrixEngine creates a new engine for a matrix
func NewMatrixEngine(matrix *core.Matrix) *MatrixEngine {
	return &MatrixEngine{matrix: matrix}
}

// SetVectorizer attaches a vectorizer to the engine for auto-embedding.
func (e *MatrixEngine) SetVectorizer(v *vector.Vectorizer) {
	e.vectorizer = v
}

// SetSentimentAnalyzer attaches a sentiment analyzer for auto-labeling on write.
func (e *MatrixEngine) SetSentimentAnalyzer(a *sentiment.Analyzer) {
	e.sentimentAnalyzer = a
}

// AddNeuron creates a new neuron and positions it organically.
// metadata is optional key-value pairs (e.g. thread_id, role, source).
func (e *MatrixEngine) AddNeuron(content string, parentID *core.NeuronID, metadata map[string]string) (*core.Neuron, error) {
	e.matrix.Lock()
	defer e.matrix.Unlock()

	if len(e.matrix.Neurons) >= e.matrix.Bounds.MaxNeurons {
		return nil, core.ErrMatrixFull
	}

	if err := core.ValidateNeuronContent(content); err != nil {
		return nil, err
	}

	// Check for duplicate content
	contentHash := core.HashContent(content)
	for _, n := range e.matrix.Neurons {
		if n.ContentHash == contentHash {
			// Existing neuron found - fire it instead
			n.Fire()
			return n, nil
		}
	}

	// Create neuron
	neuron := core.NewNeuron(content, e.matrix.CurrentDim)

	// Position organically - near parent if exists, else random
	if parentID != nil {
		if parent, ok := e.matrix.Neurons[*parentID]; ok {
			neuron.Position = e.perturbPosition(parent.Position, 0.1)
		} else {
			neuron.Position = e.randomPosition()
		}
	} else {
		// Find nearest existing neuron by content similarity (simple)
		nearest := e.findNearestByContent(content)
		if nearest != nil {
			neuron.Position = e.perturbPosition(nearest.Position, 0.2)
		} else {
			neuron.Position = e.randomPosition()
		}
	}

	// Auto-embed if vectorizer is available
	if e.vectorizer != nil && len(neuron.Embedding) == 0 {
		if emb, err := e.vectorizer.EmbedText(content); err == nil {
			vector.Normalize(emb)
			neuron.Embedding = emb
		} else {
			log.Printf("vector: embed failed for neuron %s: %v", neuron.ID, err)
		}
	}

	// Auto-label sentiment if analyzer is available
	if e.sentimentAnalyzer != nil {
		result := e.sentimentAnalyzer.Analyze(content)
		neuron.SentimentLabel = string(result.Label)
		neuron.SentimentScore = result.Compound
	}

	// Apply optional metadata
	if len(metadata) > 0 {
		for k, v := range metadata {
			neuron.Metadata[k] = v
		}
	}

	// Add to matrix
	e.matrix.Neurons[neuron.ID] = neuron
	e.matrix.Adjacency[neuron.ID] = []core.NeuronID{}
	e.matrix.TotalActivations++
	e.matrix.LastActivity = time.Now()
	e.matrix.ModifiedAt = time.Now()
	e.matrix.Version++

	// Check if dimension expansion needed
	e.checkDimensionExpansion()

	return neuron, nil
}

// GetNeuron retrieves a neuron by ID and fires it
func (e *MatrixEngine) GetNeuron(id core.NeuronID) (*core.Neuron, error) {
	e.matrix.RLock()
	neuron, ok := e.matrix.Neurons[id]
	e.matrix.RUnlock()

	if !ok {
		return nil, core.ErrNeuronNotFound
	}

	neuron.Fire()
	e.matrix.Lock()
	e.matrix.LastActivity = time.Now()
	e.matrix.TotalActivations++
	e.matrix.Unlock()

	return neuron, nil
}

// Search finds neurons matching a pattern with activation spread.
// metadata is an optional filter/boost map (e.g. {"thread_id": "conv-xyz"}).
// strict=false (default): metadata keys boost matching neurons; all neurons remain eligible.
// strict=true: only neurons whose metadata contains ALL specified key-value pairs are returned.
func (e *MatrixEngine) Search(query string, depth int, limit int, metadata map[string]string, strict bool) []*core.Neuron {
	searcher := NewSearcher(e.matrix)
	if e.vectorizer != nil {
		searcher.SetVectorizer(e.vectorizer, e.alpha, e.queryRepeat)
	}
	if e.sentimentAnalyzer != nil {
		searcher.SetSentimentAnalyzer(e.sentimentAnalyzer)
	}
	searcher.SetMetadata(metadata, strict)
	return searcher.Search(query, depth, limit)
}

// SetAlpha sets the vector score weight for hybrid search.
func (e *MatrixEngine) SetAlpha(alpha float64) {
	e.alpha = alpha
}

// SetQueryRepeat sets the query repetition count for embedding.
func (e *MatrixEngine) SetQueryRepeat(n int) {
	if n < 1 {
		n = 1
	}
	e.queryRepeat = n
}

// directMatch finds neurons with content matching query
func (e *MatrixEngine) directMatch(query string) []*core.Neuron {
	var matches []*core.Neuron
	queryLower := strings.ToLower(query)
	queryParts := strings.Fields(queryLower)

	for _, n := range e.matrix.Neurons {
		contentLower := strings.ToLower(n.Content)

		// Exact substring match
		if strings.Contains(contentLower, queryLower) {
			matches = append(matches, n)
			continue
		}

		// Partial word match
		matchCount := 0
		for _, part := range queryParts {
			if strings.Contains(contentLower, part) {
				matchCount++
			}
		}
		if matchCount > 0 && float64(matchCount)/float64(len(queryParts)) >= 0.5 {
			matches = append(matches, n)
		}
	}

	return matches
}

// spreadActivation propagates through synapses
func (e *MatrixEngine) spreadActivation(seeds []*core.Neuron, depth int) []*core.Neuron {
	visited := make(map[core.NeuronID]bool)
	for _, s := range seeds {
		visited[s.ID] = true
	}

	current := seeds
	var spread []*core.Neuron

	for d := 0; d < depth; d++ {
		var next []*core.Neuron
		for _, n := range current {
			// Get connected neurons
			connected := e.matrix.Adjacency[n.ID]
			for _, connID := range connected {
				if visited[connID] {
					continue
				}

				// Check synapse strength
				synID := core.NewSynapseID(n.ID, connID)
				syn, ok := e.matrix.Synapses[synID]
				if !ok {
					// Try reverse
					synID = core.NewSynapseID(connID, n.ID)
					syn, ok = e.matrix.Synapses[synID]
				}

				if ok && syn.Weight >= e.matrix.LinkThreshold {
					if conn, ok := e.matrix.Neurons[connID]; ok {
						visited[connID] = true
						spread = append(spread, conn)
						next = append(next, conn)
					}
				}
			}
		}
		current = next
	}

	return spread
}

// relevanceScore calculates how relevant a neuron is to a query
func (e *MatrixEngine) relevanceScore(n *core.Neuron, query string) float64 {
	// Content match score
	contentMatch := 0.0
	if strings.Contains(strings.ToLower(n.Content), strings.ToLower(query)) {
		contentMatch = 1.0
	} else {
		// Partial match
		queryParts := strings.Fields(strings.ToLower(query))
		matchCount := 0
		for _, part := range queryParts {
			if strings.Contains(strings.ToLower(n.Content), part) {
				matchCount++
			}
		}
		if len(queryParts) > 0 {
			contentMatch = float64(matchCount) / float64(len(queryParts))
		}
	}

	// Recency score (decay over time)
	hoursSinceAccess := time.Since(n.LastFiredAt).Hours()
	recencyScore := math.Exp(-hoursSinceAccess / 24) // Half-life of ~24 hours

	// Energy score
	energyScore := n.Energy

	// Depth penalty (surface = better for immediate recall)
	depthPenalty := 1.0 / (1.0 + float64(n.Depth)*0.2)

	// Combined score
	return (contentMatch*0.4 + recencyScore*0.2 + energyScore*0.2 + depthPenalty*0.2)
}

// UpdateNeuron modifies a neuron's content
func (e *MatrixEngine) UpdateNeuron(id core.NeuronID, newContent string) error {
	e.matrix.Lock()
	defer e.matrix.Unlock()

	neuron, ok := e.matrix.Neurons[id]
	if !ok {
		return core.ErrNeuronNotFound
	}

	if err := core.ValidateNeuronContent(newContent); err != nil {
		return err
	}

	neuron.Content = newContent
	neuron.ContentHash = core.HashContent(newContent)
	neuron.Fire()
	e.matrix.ModifiedAt = time.Now()
	e.matrix.Version++

	return nil
}

// DeleteNeuron removes a neuron and its synapses
func (e *MatrixEngine) DeleteNeuron(id core.NeuronID) error {
	e.matrix.Lock()
	defer e.matrix.Unlock()

	if _, ok := e.matrix.Neurons[id]; !ok {
		return core.ErrNeuronNotFound
	}

	// Remove all connected synapses
	for synID, syn := range e.matrix.Synapses {
		if syn.FromID == id || syn.ToID == id {
			delete(e.matrix.Synapses, synID)
		}
	}

	// Remove from adjacency lists
	delete(e.matrix.Adjacency, id)
	for nID := range e.matrix.Adjacency {
		filtered := make([]core.NeuronID, 0)
		for _, connID := range e.matrix.Adjacency[nID] {
			if connID != id {
				filtered = append(filtered, connID)
			}
		}
		e.matrix.Adjacency[nID] = filtered
	}

	// Remove neuron
	delete(e.matrix.Neurons, id)
	e.matrix.ModifiedAt = time.Now()
	e.matrix.Version++

	// Check if dimension contraction needed
	e.checkDimensionContraction()

	return nil
}

// ListNeurons returns all neurons sorted by energy
func (e *MatrixEngine) ListNeurons(offset, limit int, depthFilter *int) []*core.Neuron {
	e.matrix.RLock()
	defer e.matrix.RUnlock()

	var neurons []*core.Neuron
	for _, n := range e.matrix.Neurons {
		if depthFilter != nil && n.Depth != *depthFilter {
			continue
		}
		neurons = append(neurons, n)
	}

	// Sort by energy descending
	sort.Slice(neurons, func(i, j int) bool {
		return neurons[i].Energy > neurons[j].Energy
	})

	// Apply pagination
	if offset >= len(neurons) {
		return []*core.Neuron{}
	}
	neurons = neurons[offset:]
	if limit > 0 && len(neurons) > limit {
		neurons = neurons[:limit]
	}

	return neurons
}

// perturbPosition creates a new position near the given one
func (e *MatrixEngine) perturbPosition(pos []float64, magnitude float64) []float64 {
	newPos := make([]float64, len(pos))
	for i, p := range pos {
		newPos[i] = p + (rand.Float64()-0.5)*2*magnitude
		// Clamp to [-1, 1]
		newPos[i] = math.Max(-1, math.Min(1, newPos[i]))
	}
	return newPos
}

// randomPosition generates a random position in the matrix
func (e *MatrixEngine) randomPosition() []float64 {
	pos := make([]float64, e.matrix.CurrentDim)
	for i := range pos {
		pos[i] = (rand.Float64() - 0.5) * 2 // [-1, 1]
	}
	return pos
}

// findNearestByContent finds the most similar existing neuron
func (e *MatrixEngine) findNearestByContent(content string) *core.Neuron {
	contentLower := strings.ToLower(content)
	var best *core.Neuron
	bestScore := 0.0

	for _, n := range e.matrix.Neurons {
		// Word-overlap similarity score
		words1 := strings.Fields(contentLower)
		words2 := strings.Fields(strings.ToLower(n.Content))

		overlap := 0
		for _, w1 := range words1 {
			for _, w2 := range words2 {
				if w1 == w2 {
					overlap++
					break
				}
			}
		}

		score := float64(overlap) / math.Max(1, math.Max(float64(len(words1)), float64(len(words2))))
		if score > bestScore {
			bestScore = score
			best = n
		}
	}

	return best
}

// checkDimensionExpansion expands dimension if density is too high
func (e *MatrixEngine) checkDimensionExpansion() {
	neuronCount := len(e.matrix.Neurons)
	density := float64(neuronCount) / float64(e.matrix.CurrentDim)

	// If more than 100 neurons per dimension, expand
	if density > 100 && e.matrix.CurrentDim < e.matrix.Bounds.MaxDimension {
		newDim := min(e.matrix.CurrentDim+1, e.matrix.Bounds.MaxDimension)
		e.expandDimension(newDim)
	}
}

// checkDimensionContraction contracts dimension if too sparse
func (e *MatrixEngine) checkDimensionContraction() {
	neuronCount := len(e.matrix.Neurons)
	density := float64(neuronCount) / float64(e.matrix.CurrentDim)

	// If less than 10 neurons per dimension, contract
	if density < 10 && e.matrix.CurrentDim > e.matrix.Bounds.MinDimension {
		newDim := max(e.matrix.CurrentDim-1, e.matrix.Bounds.MinDimension)
		e.contractDimension(newDim)
	}
}

// expandDimension adds a new dimension
func (e *MatrixEngine) expandDimension(newDim int) {
	for _, n := range e.matrix.Neurons {
		// Add new dimension with small random value
		for len(n.Position) < newDim {
			n.Position = append(n.Position, (rand.Float64()-0.5)*0.1)
		}
	}
	e.matrix.CurrentDim = newDim
}

// contractDimension removes the last dimension
func (e *MatrixEngine) contractDimension(newDim int) {
	for _, n := range e.matrix.Neurons {
		if len(n.Position) > newDim {
			n.Position = n.Position[:newDim]
		}
	}
	e.matrix.CurrentDim = newDim
}

// GetStats returns matrix statistics
func (e *MatrixEngine) GetStats() map[string]any {
	e.matrix.RLock()
	defer e.matrix.RUnlock()

	depthCounts := make(map[int]int)
	totalEnergy := 0.0
	for _, n := range e.matrix.Neurons {
		depthCounts[n.Depth]++
		totalEnergy += n.Energy
	}

	avgEnergy := 0.0
	if len(e.matrix.Neurons) > 0 {
		avgEnergy = totalEnergy / float64(len(e.matrix.Neurons))
	}

	synapseWeights := make([]float64, 0, len(e.matrix.Synapses))
	totalWeight := 0.0
	for _, syn := range e.matrix.Synapses {
		synapseWeights = append(synapseWeights, syn.Weight)
		totalWeight += syn.Weight
	}
	avgWeight := 0.0
	if len(synapseWeights) > 0 {
		avgWeight = totalWeight / float64(len(synapseWeights))
	}

	return map[string]any{
		"index_id":               e.matrix.IndexID,
		"neuron_count":           len(e.matrix.Neurons),
		"synapse_count":          len(e.matrix.Synapses),
		"current_dimension":      e.matrix.CurrentDim,
		"depth_distribution":     depthCounts,
		"average_energy":         avgEnergy,
		"total_activations":      e.matrix.TotalActivations,
		"last_activity":          e.matrix.LastActivity,
		"version":                e.matrix.Version,
		"synapse_weights":        synapseWeights,
		"average_synapse_weight": avgWeight,
	}
}
