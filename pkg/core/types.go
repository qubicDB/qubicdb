package core

import (
	"sync"
	"time"

	"github.com/google/uuid"
)

// NeuronID is a unique identifier for a neuron
type NeuronID string

// SynapseID is a unique identifier for a synapse connection
type SynapseID string

// IndexID is a unique identifier for a user's brain instance
type IndexID string

// NewNeuronID generates a new unique neuron ID
func NewNeuronID() NeuronID {
	return NeuronID(uuid.New().String())
}

// NewSynapseID generates a synapse ID from two neuron IDs
func NewSynapseID(from, to NeuronID) SynapseID {
	return SynapseID(string(from) + ":" + string(to))
}

// Neuron represents a single memory unit in the brain
type Neuron struct {
	ID          NeuronID `msgpack:"id"`
	Content     string   `msgpack:"content"`
	ContentHash string   `msgpack:"content_hash"`

	// Spatial position in N-dimensional matrix (organic, grows/shrinks)
	Position []float64 `msgpack:"position"`

	// Activation dynamics
	Energy     float64 `msgpack:"energy"`      // Current activation level (0.0 - 1.0)
	BaseEnergy float64 `msgpack:"base_energy"` // Resting energy level

	// Depth in the memory hierarchy (0 = surface/hot, higher = deeper/consolidated)
	Depth int `msgpack:"depth"`

	// Temporal markers
	CreatedAt   time.Time `msgpack:"created_at"`
	LastFiredAt time.Time `msgpack:"last_fired_at"`
	LastDecayAt time.Time `msgpack:"last_decay_at"`
	AccessCount uint64    `msgpack:"access_count"`

	// Classification tags (emergent, not predefined)
	Tags []string `msgpack:"tags"`

	// Sentiment classification (set on creation, updated on content change)
	SentimentLabel string  `msgpack:"sentiment_label,omitempty"` // happiness|sadness|fear|anger|disgust|surprise|neutral
	SentimentScore float64 `msgpack:"sentiment_score,omitempty"` // VADER compound [-1, 1]

	// Vector embedding for semantic search (set once on creation, nil if vector layer disabled)
	Embedding []float32 `msgpack:"embedding,omitempty"`

	// Metadata
	Metadata map[string]any `msgpack:"metadata"`

	mu sync.RWMutex `msgpack:"-"`
}

// NewNeuron creates a new neuron with given content
func NewNeuron(content string, initialDim int) *Neuron {
	now := time.Now()
	n := &Neuron{
		ID:          NewNeuronID(),
		Content:     content,
		ContentHash: HashContent(content),
		Position:    make([]float64, initialDim),
		Energy:      1.0, // Born fully activated
		BaseEnergy:  0.1,
		Depth:       0, // Surface level
		CreatedAt:   now,
		LastFiredAt: now,
		LastDecayAt: now,
		AccessCount: 1,
		Tags:        []string{},
		Metadata:    make(map[string]any),
	}
	return n
}

// Fire activates the neuron, boosting its energy
func (n *Neuron) Fire() {
	n.mu.Lock()
	defer n.mu.Unlock()

	n.Energy = min(1.0, n.Energy+0.3)
	n.LastFiredAt = time.Now()
	n.AccessCount++
}

// Decay reduces energy based on time elapsed since the last decay tick,
// not since last fire. This prevents compounding decay on long-idle neurons.
func (n *Neuron) Decay(rate float64) {
	n.mu.Lock()
	defer n.mu.Unlock()

	now := time.Now()
	elapsed := now.Sub(n.LastDecayAt).Seconds()
	decay := rate * elapsed / 3600 // rate per hour
	n.Energy = max(n.BaseEnergy, n.Energy-decay)
	n.LastDecayAt = now
}

// IsAlive checks if neuron is still active enough
// Note: Neurons never truly "die" - they become dormant with very low energy
// This is used for search relevance, not deletion
func (n *Neuron) IsAlive() bool {
	n.mu.RLock()
	defer n.mu.RUnlock()
	return n.Energy > 0.01
}

// IsDormant checks if neuron has very low energy but still exists
func (n *Neuron) IsDormant() bool {
	n.mu.RLock()
	defer n.mu.RUnlock()
	return n.Energy <= 0.01 && n.Energy > 0
}

// Reactivate boosts a dormant neuron's energy when recalled
func (n *Neuron) Reactivate(boost float64) {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.Energy = min(1.0, n.Energy+boost)
	n.LastFiredAt = time.Now()
	n.AccessCount++
}

// ShouldConsolidate checks if neuron is ready to move deeper.
// Requires sufficient access count, age, AND that energy has decayed below
// the active threshold — a neuron still firing frequently should not consolidate.
func (n *Neuron) ShouldConsolidate(accessThreshold uint64, ageThreshold time.Duration) bool {
	n.mu.RLock()
	defer n.mu.RUnlock()

	age := time.Since(n.CreatedAt)
	return n.AccessCount >= accessThreshold && age >= ageThreshold && n.Energy < 0.5
}

// Synapse represents a connection between two neurons
type Synapse struct {
	ID     SynapseID `msgpack:"id"`
	FromID NeuronID  `msgpack:"from_id"`
	ToID   NeuronID  `msgpack:"to_id"`

	// Connection strength (0.0 - 1.0)
	Weight float64 `msgpack:"weight"`

	// Hebbian learning metrics
	CoFireCount uint64    `msgpack:"co_fire_count"` // Times fired together
	LastCoFire  time.Time `msgpack:"last_co_fire"`

	// Bidirectional flag
	Bidirectional bool `msgpack:"bidirectional"`

	CreatedAt time.Time `msgpack:"created_at"`

	mu sync.RWMutex `msgpack:"-"`
}

// NewSynapse creates a new synapse between two neurons
func NewSynapse(from, to NeuronID, initialWeight float64) *Synapse {
	now := time.Now()
	return &Synapse{
		ID:            NewSynapseID(from, to),
		FromID:        from,
		ToID:          to,
		Weight:        initialWeight,
		CoFireCount:   1,
		LastCoFire:    now,
		Bidirectional: true,
		CreatedAt:     now,
	}
}

// Strengthen increases synapse weight (Hebbian potentiation)
func (s *Synapse) Strengthen(delta float64) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.Weight = min(1.0, s.Weight+delta)
	s.CoFireCount++
	s.LastCoFire = time.Now()
}

// Weaken decreases synapse weight
func (s *Synapse) Weaken(delta float64) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.Weight = max(0.0, s.Weight-delta)
}

// Decay reduces weight based on time
func (s *Synapse) Decay(rate float64) {
	s.mu.Lock()
	defer s.mu.Unlock()

	elapsed := time.Since(s.LastCoFire).Seconds()
	decay := rate * elapsed / 3600
	s.Weight = max(0.0, s.Weight-decay)
}

// IsAlive checks if synapse is strong enough to be active in searches
// Note: Synapses don't die, they become weak but persist
func (s *Synapse) IsAlive() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.Weight > 0.05
}

// IsWeak checks if synapse is very weak (candidate for deep storage, not deletion)
func (s *Synapse) IsWeak() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.Weight <= 0.05 && s.Weight > 0
}

// ShouldArchive checks if synapse is old and weak enough for deep storage
// Returns true if inactive for >30 days and weight < 0.01
func (s *Synapse) ShouldArchive() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.Weight < 0.01 && time.Since(s.LastCoFire) > 30*24*time.Hour
}

// Reactivate strengthens a weak synapse when neurons co-fire again
func (s *Synapse) Reactivate(boost float64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Weight = min(1.0, s.Weight+boost)
	s.CoFireCount++
	s.LastCoFire = time.Now()
}

// MatrixBounds defines the organic growth limits
type MatrixBounds struct {
	MinDimension int `msgpack:"min_dim"`
	MaxDimension int `msgpack:"max_dim"`
	MinNeurons   int `msgpack:"min_neurons"`
	MaxNeurons   int `msgpack:"max_neurons"`
}

// DefaultBounds returns sensible defaults
func DefaultBounds() MatrixBounds {
	return MatrixBounds{
		MinDimension: 3,
		MaxDimension: 1000,
		MinNeurons:   0,
		MaxNeurons:   1000000, // 1M neurons max per user
	}
}

// Matrix represents the organic memory space for a single user
type Matrix struct {
	IndexID    IndexID      `msgpack:"index_id"`
	Bounds     MatrixBounds `msgpack:"bounds"`
	CurrentDim int          `msgpack:"current_dim"`

	// Neurons indexed by ID
	Neurons map[NeuronID]*Neuron `msgpack:"neurons"`

	// Synapses indexed by ID
	Synapses map[SynapseID]*Synapse `msgpack:"synapses"`

	// Adjacency list for fast traversal (neuron -> connected neurons)
	Adjacency map[NeuronID][]NeuronID `msgpack:"adjacency"`

	// Learned parameters (self-tuning)
	DecayRate       float64       `msgpack:"decay_rate"`
	LinkThreshold   float64       `msgpack:"link_threshold"`
	ConsolFrequency time.Duration `msgpack:"consol_freq"`

	// Statistics
	TotalActivations  uint64    `msgpack:"total_activations"`
	LastActivity      time.Time `msgpack:"last_activity"`
	LastConsolidation time.Time `msgpack:"last_consolidation"`

	// Version for persistence
	Version    uint64    `msgpack:"version"`
	CreatedAt  time.Time `msgpack:"created_at"`
	ModifiedAt time.Time `msgpack:"modified_at"`

	mu sync.RWMutex `msgpack:"-"`
}

// NewMatrix creates a new organic memory matrix for a user
func NewMatrix(indexID IndexID, bounds MatrixBounds) *Matrix {
	now := time.Now()
	return &Matrix{
		IndexID:           indexID,
		Bounds:            bounds,
		CurrentDim:        bounds.MinDimension,
		Neurons:           make(map[NeuronID]*Neuron),
		Synapses:          make(map[SynapseID]*Synapse),
		Adjacency:         make(map[NeuronID][]NeuronID),
		DecayRate:         0.1, // Will self-tune
		LinkThreshold:     0.3, // Will self-tune
		ConsolFrequency:   5 * time.Minute,
		TotalActivations:  0,
		LastActivity:      now,
		LastConsolidation: now,
		Version:           1,
		CreatedAt:         now,
		ModifiedAt:        now,
	}
}

// ActivityState represents the current activity level
type ActivityState int

const (
	StateActive   ActivityState = iota // User actively interacting
	StateIdle                          // User paused
	StateSleeping                      // Consolidation in progress
	StateDormant                       // Persisted, not in memory
)

// BrainState tracks the lifecycle state of a user's brain
type BrainState struct {
	IndexID        IndexID       `msgpack:"index_id"`
	State          ActivityState `msgpack:"state"`
	LastInvoke     time.Time     `msgpack:"last_invoke"`
	InvokeCount    uint64        `msgpack:"invoke_count"`
	SessionStart   time.Time     `msgpack:"session_start"`
	IdleThreshold  time.Duration `msgpack:"idle_threshold"`
	SleepThreshold time.Duration `msgpack:"sleep_threshold"`
}

// NewBrainState creates initial brain state
func NewBrainState(indexID IndexID) *BrainState {
	now := time.Now()
	return &BrainState{
		IndexID:        indexID,
		State:          StateActive,
		LastInvoke:     now,
		InvokeCount:    1,
		SessionStart:   now,
		IdleThreshold:  30 * time.Second,
		SleepThreshold: 5 * time.Minute,
	}
}

// Matrix lock methods for external packages
func (m *Matrix) Lock()    { m.mu.Lock() }
func (m *Matrix) Unlock()  { m.mu.Unlock() }
func (m *Matrix) RLock()   { m.mu.RLock() }
func (m *Matrix) RUnlock() { m.mu.RUnlock() }

// Neuron lock methods for external packages
func (n *Neuron) Lock()    { n.mu.Lock() }
func (n *Neuron) Unlock()  { n.mu.Unlock() }
func (n *Neuron) RLock()   { n.mu.RLock() }
func (n *Neuron) RUnlock() { n.mu.RUnlock() }

// HashContent generates a consistent hash for content deduplication
// Exported for use across packages
func HashContent(content string) string {
	return uuid.NewSHA1(uuid.NameSpaceOID, []byte(content)).String()
}

// TimeSince is a wrapper for time.Since for easier mocking in tests
func TimeSince(t time.Time) time.Duration {
	return time.Since(t)
}
