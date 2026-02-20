package engine

import (
	"fmt"
	"math"
	"regexp"
	"sort"
	"strings"
	"sync"
	"unicode"

	"github.com/qubicDB/qubicdb/pkg/core"
	"github.com/qubicDB/qubicdb/pkg/sentiment"
	"github.com/qubicDB/qubicdb/pkg/vector"
)

// SearchResult holds a neuron with its relevance score
type SearchResult struct {
	Neuron *core.Neuron
	Score  float64
}

func (s *Searcher) contentTokens(n *core.Neuron) []string {
	s.tokenCacheMu.RLock()
	entry, ok := s.tokenCache[n.ID]
	s.tokenCacheMu.RUnlock()
	if ok && entry.hash == n.ContentHash {
		return entry.tokens
	}

	tokens := tokenize(n.Content)

	s.tokenCacheMu.Lock()
	if len(s.tokenCache) > len(s.matrix.Neurons)*2 {
		s.tokenCache = make(map[core.NeuronID]tokenCacheEntry, len(s.matrix.Neurons))
	}
	s.tokenCache[n.ID] = tokenCacheEntry{hash: n.ContentHash, tokens: tokens}
	s.tokenCacheMu.Unlock()

	return tokens
}

// Searcher provides advanced search capabilities
type Searcher struct {
	matrix            *core.Matrix
	vectorizer        *vector.Vectorizer  // nil when vector layer is disabled
	alpha             float64             // vector score weight (0.0-1.0)
	queryRepeat       int                 // query repetition count for embedding (1=off, 2+=repeat)
	sentimentAnalyzer *sentiment.Analyzer // nil when sentiment layer is disabled
	metadata          map[string]string   // optional metadata filter/boost
	strict            bool                // if true, only neurons matching all metadata keys are returned

	tokenCacheMu sync.RWMutex
	tokenCache   map[core.NeuronID]tokenCacheEntry
}

type tokenCacheEntry struct {
	hash   string
	tokens []string
}

var tokenSplitRegex = regexp.MustCompile(`[^\p{L}\p{N}]+`)

// NewSearcher creates a new searcher
func NewSearcher(matrix *core.Matrix) *Searcher {
	return &Searcher{
		matrix:     matrix,
		alpha:      0.6,
		tokenCache: make(map[core.NeuronID]tokenCacheEntry),
	}
}

// SetVectorizer attaches a vectorizer, alpha weight, and query repeat count to the searcher.
func (s *Searcher) SetVectorizer(v *vector.Vectorizer, alpha float64, queryRepeat int) {
	s.vectorizer = v
	s.alpha = alpha
	if queryRepeat < 1 {
		queryRepeat = 1
	}
	s.queryRepeat = queryRepeat
}

// SetSentimentAnalyzer attaches a sentiment analyzer to the searcher.
func (s *Searcher) SetSentimentAnalyzer(a *sentiment.Analyzer) {
	s.sentimentAnalyzer = a
}

// SetMetadata configures optional metadata filtering/boosting.
// strict=false: matching neurons get a score boost (1.3x per matching key).
// strict=true: only neurons that match ALL key-value pairs are returned.
func (s *Searcher) SetMetadata(metadata map[string]string, strict bool) {
	s.metadata = metadata
	s.strict = strict
}

// Search performs an intelligent search with multiple scoring factors
func (s *Searcher) Search(query string, depth int, limit int) []*core.Neuron {
	// Clean query through the same pipeline used at write time so that
	// embedding space alignment is consistent between stored and query vectors.
	query = vector.CleanText(query)
	if query == "" {
		return []*core.Neuron{}
	}

	// Tokenize query
	queryTokens := tokenize(query)
	if len(queryTokens) == 0 {
		return []*core.Neuron{}
	}
	queryLower := strings.ToLower(query)

	// Embed query outside matrix lock to avoid extending read-lock hold time.
	// Short queries (≤3 tokens) are verbosely expanded before embedding so the
	// model has enough context for meaningful bidirectional attention.
	// The expanded form is then repeated queryRepeat times (Springer et al. 2024).
	var queryVec []float32
	if s.vectorizer != nil {
		embedInput := query
		if len(queryTokens) <= 3 {
			embedInput = "search for information about " + query
		}
		if s.queryRepeat > 1 {
			parts := make([]string, s.queryRepeat)
			for i := range parts {
				parts[i] = embedInput
			}
			embedInput = strings.Join(parts, " ")
		}
		if emb, err := s.vectorizer.EmbedText(embedInput); err == nil {
			vector.Normalize(emb)
			queryVec = emb
		}
	}

	// Analyze query sentiment for downstream scoring.
	var queryLabel sentiment.Label
	if s.sentimentAnalyzer != nil {
		queryLabel = s.sentimentAnalyzer.Analyze(query).Label
	}

	s.matrix.RLock()

	if len(s.matrix.Neurons) == 0 {
		s.matrix.RUnlock()
		return []*core.Neuron{}
	}

	// Score all neurons
	results := make([]SearchResult, 0, len(s.matrix.Neurons))
	for _, n := range s.matrix.Neurons {
		score := s.scoreNeuron(n, query, queryLower, queryTokens, queryVec, queryLabel)
		if score > 0 {
			results = append(results, SearchResult{Neuron: n, Score: score})
		}
	}

	// Sort by score descending
	sort.Slice(results, func(i, j int) bool {
		return results[i].Score > results[j].Score
	})

	// Apply spread activation if depth > 0
	if depth > 0 && len(results) > 0 {
		results = s.spreadActivation(results, depth)
	}

	// Post-filter: strict metadata — spread activation may have added neurons
	// that don't match; remove them here after spread so graph traversal is
	// not affected but the final result set is clean.
	if s.strict && len(s.metadata) > 0 {
		filtered := results[:0]
		for _, r := range results {
			match := true
			for k, v := range s.metadata {
				if nv, ok := r.Neuron.Metadata[k]; !ok || fmt.Sprintf("%v", nv) != v {
					match = false
					break
				}
			}
			if match {
				filtered = append(filtered, r)
			}
		}
		results = filtered
	}

	// Limit results
	if limit > 0 && len(results) > limit {
		results = results[:limit]
	}

	s.matrix.RUnlock() // release before Fire() acquires neuron write-locks

	// Fire neurons outside matrix lock — Fire() takes neuron.mu.Lock()
	// which must not be acquired while matrix RLock is held (pending matrix
	// writers would cause a deadlock via Go's RWMutex writer-starvation guard).
	neurons := make([]*core.Neuron, len(results))
	for i, r := range results {
		r.Neuron.Fire()
		neurons[i] = r.Neuron
	}

	return neurons
}

// scoreNeuron calculates relevance score for a neuron using hybrid string+vector scoring.
func (s *Searcher) scoreNeuron(n *core.Neuron, query, queryLower string, queryTokens []string, queryVec []float32, queryLabel sentiment.Label) float64 {
	// --- String-based score (original mechanics) ---
	stringScore := s.stringScore(n, query, queryLower, queryTokens)

	// --- Vector-based score (semantic similarity) ---
	vectorScore := 0.0
	if queryVec != nil && len(n.Embedding) > 0 && len(queryVec) == len(n.Embedding) {
		vectorScore = vector.CosineSimilarity(queryVec, n.Embedding)
		if vectorScore < 0 {
			vectorScore = 0
		}
	}

	// --- Hybrid combination ---
	// String score is normalized with tanh(x/10) instead of tanh(x/5).
	// The /5 divisor compressed the [0,15] range too aggressively:
	// tanh(10/5)=0.964 vs tanh(15/5)=0.995 — only 0.031 separation.
	// With /10: tanh(10/10)=0.762 vs tanh(15/10)=0.905 — 0.143 separation,
	// preserving meaningful signal differences across the full score range.
	var baseScore float64
	if queryVec != nil && len(n.Embedding) > 0 {
		normStringScore := math.Tanh(stringScore / 10.0)
		baseScore = s.alpha*vectorScore + (1.0-s.alpha)*normStringScore
	} else {
		baseScore = stringScore
	}

	if baseScore <= 0 {
		return 0
	}

	// --- Brain mechanics modifiers ---

	// Energy boost (active neurons rank higher)
	baseScore *= (0.5 + n.Energy*0.5)

	// Recency boost
	ageHours := core.TimeSince(n.LastFiredAt).Hours()
	recencyBoost := 1.0 / (1.0 + ageHours/24.0)
	baseScore *= (0.8 + recencyBoost*0.2)

	// Access count boost (frequently accessed = important)
	accessBoost := math.Log10(float64(n.AccessCount) + 1)
	baseScore *= (1.0 + accessBoost*0.1)

	// Depth penalty (deeper = less immediate relevance)
	depthPenalty := 1.0 / (1.0 + float64(n.Depth)*0.2)
	baseScore *= depthPenalty

	// --- Sentiment boost ---
	// Neurons whose emotional valence matches the query's are ranked higher.
	// Multiplier range: [0.8, 1.2] — soft signal, never overrides relevance.
	if queryLabel != sentiment.LabelNeutral && n.SentimentLabel != "" {
		baseScore *= sentiment.SentimentBoost(queryLabel, sentiment.Label(n.SentimentLabel))
	}

	// --- Metadata boost / strict filter ---
	// Requires neuron.Metadata to be map[string]any; values stored as string.
	if len(s.metadata) > 0 {
		matchCount := 0
		for k, v := range s.metadata {
			if nv, ok := n.Metadata[k]; ok {
				if fmt.Sprintf("%v", nv) == v {
					matchCount++
				}
			}
		}
		if s.strict && matchCount < len(s.metadata) {
			return 0 // exclude neuron entirely
		}
		if matchCount > 0 {
			baseScore *= 1.0 + float64(matchCount)*0.3 // +30% per matching key
		}
	}

	return baseScore
}

// stringScore calculates pure lexical relevance (original scoring logic).
func (s *Searcher) stringScore(n *core.Neuron, query, queryLower string, queryTokens []string) float64 {
	content := strings.ToLower(n.Content)
	contentTokens := s.contentTokens(n)

	var score float64

	// 1. Exact phrase match (highest weight)
	if strings.Contains(content, queryLower) {
		score += 10.0
	}

	// 2. Word overlap scoring (Jaccard-like)
	matchedWords := 0
	for _, qt := range queryTokens {
		for _, ct := range contentTokens {
			if ct == qt {
				matchedWords++
				break
			}
			// Fuzzy match - prefix/suffix
			if len(ct) > 3 && len(qt) > 3 {
				if strings.HasPrefix(ct, qt[:3]) || strings.HasPrefix(qt, ct[:3]) {
					matchedWords++
					score += 0.3 // Partial credit for fuzzy match
					break
				}
			}
		}
	}

	if len(queryTokens) > 0 {
		wordOverlapScore := float64(matchedWords) / float64(len(queryTokens))
		score += wordOverlapScore * 5.0
	}

	// 3. Levenshtein distance for fuzzy matching (bounded to reduce hot-path cost)
	if len(query) <= 20 && score < 8.0 {
		maxComparisons := 8
		for i, ct := range contentTokens {
			if i >= maxComparisons {
				break
			}
			dist := levenshteinDistance(queryLower, ct)
			maxLen := max(len(queryLower), len(ct))
			if maxLen > 0 {
				similarity := 1.0 - float64(dist)/float64(maxLen)
				if similarity > 0.7 {
					score += similarity * 2.0
				}
			}
		}
	}

	return score
}

// spreadActivation finds related neurons through synapse connections
func (s *Searcher) spreadActivation(initial []SearchResult, depth int) []SearchResult {
	seen := make(map[core.NeuronID]bool)
	results := make([]SearchResult, 0, len(initial)*2)

	// Add initial results
	for _, r := range initial {
		seen[r.Neuron.ID] = true
		results = append(results, r)
	}

	// Spread through connections
	current := initial
	for d := 0; d < depth; d++ {
		next := make([]SearchResult, 0)

		for _, r := range current {
			// Get connected neurons via adjacency
			connected := s.matrix.Adjacency[r.Neuron.ID]
			for _, connID := range connected {
				if seen[connID] {
					continue
				}

				connNeuron, ok := s.matrix.Neurons[connID]
				if !ok {
					continue
				}

				// Get synapse weight
				synapseID := core.NewSynapseID(r.Neuron.ID, connID)
				synapse, ok := s.matrix.Synapses[synapseID]
				if !ok {
					// Try reverse
					synapseID = core.NewSynapseID(connID, r.Neuron.ID)
					synapse, ok = s.matrix.Synapses[synapseID]
				}

				weight := 0.3 // Default weight
				if ok {
					weight = synapse.Weight
				}

				// Spread score decays with distance and is multiplied by synapse weight
				spreadScore := r.Score * weight * (1.0 / float64(d+2))

				if spreadScore > 0.1 { // Threshold to avoid noise
					seen[connID] = true
					next = append(next, SearchResult{
						Neuron: connNeuron,
						Score:  spreadScore,
					})
				}
			}
		}

		results = append(results, next...)
		current = next

		if len(next) == 0 {
			break
		}
	}

	// Re-sort combined results
	sort.Slice(results, func(i, j int) bool {
		return results[i].Score > results[j].Score
	})

	return results
}

// tokenize splits text into lowercase tokens
func tokenize(text string) []string {
	// Remove punctuation and split
	cleaned := tokenSplitRegex.ReplaceAllString(text, " ")

	words := strings.Fields(cleaned)
	tokens := make([]string, 0, len(words))

	for _, w := range words {
		w = strings.ToLower(w)
		// Skip very short words
		if len(w) >= 2 {
			tokens = append(tokens, w)
		}
	}

	return tokens
}

// levenshteinDistance calculates edit distance between two strings
func levenshteinDistance(a, b string) int {
	if len(a) == 0 {
		return len(b)
	}
	if len(b) == 0 {
		return len(a)
	}

	// Convert to runes for proper unicode handling
	aRunes := []rune(a)
	bRunes := []rune(b)

	// Create matrix
	matrix := make([][]int, len(aRunes)+1)
	for i := range matrix {
		matrix[i] = make([]int, len(bRunes)+1)
		matrix[i][0] = i
	}
	for j := range matrix[0] {
		matrix[0][j] = j
	}

	// Fill matrix
	for i := 1; i <= len(aRunes); i++ {
		for j := 1; j <= len(bRunes); j++ {
			cost := 1
			if unicode.ToLower(aRunes[i-1]) == unicode.ToLower(bRunes[j-1]) {
				cost = 0
			}

			matrix[i][j] = minInt(
				matrix[i-1][j]+1,      // deletion
				matrix[i][j-1]+1,      // insertion
				matrix[i-1][j-1]+cost, // substitution
			)
		}
	}

	return matrix[len(aRunes)][len(bRunes)]
}

func minInt(a, b, c int) int {
	if a < b {
		if a < c {
			return a
		}
		return c
	}
	if b < c {
		return b
	}
	return c
}
