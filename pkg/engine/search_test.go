package engine

import (
	"testing"

	"github.com/denizumutdereli/qubicdb/pkg/core"
)

func TestSearcherBasicSearch(t *testing.T) {
	m := core.NewMatrix("test-user", core.DefaultBounds())
	e := NewMatrixEngine(m)

	e.AddNeuron("TypeScript programming language", nil, nil)
	e.AddNeuron("Go programming language", nil, nil)
	e.AddNeuron("Docker containers", nil, nil)

	searcher := NewSearcher(m)
	results := searcher.Search("programming", 0, 10)

	if len(results) != 2 {
		t.Errorf("Expected 2 results, got %d", len(results))
	}
}

func TestSearcherFuzzyMatch(t *testing.T) {
	m := core.NewMatrix("test-user", core.DefaultBounds())
	e := NewMatrixEngine(m)

	e.AddNeuron("TypeScript development", nil, nil)

	searcher := NewSearcher(m)

	// Test partial word match
	results := searcher.Search("Type", 0, 10)
	if len(results) == 0 {
		t.Error("Should find result for partial word match")
	}
}

func TestSearcherWordOverlap(t *testing.T) {
	m := core.NewMatrix("test-user", core.DefaultBounds())
	e := NewMatrixEngine(m)

	e.AddNeuron("Go programming with TypeScript integration", nil, nil)
	e.AddNeuron("Python programming", nil, nil)

	searcher := NewSearcher(m)

	// Multi-word query
	results := searcher.Search("Go TypeScript", 0, 10)

	// First result should be the one with both words
	if len(results) > 0 {
		if results[0].Content != "Go programming with TypeScript integration" {
			t.Error("Best match should be the one with both query words")
		}
	}
}

func TestSearcherExactPhrase(t *testing.T) {
	m := core.NewMatrix("test-user", core.DefaultBounds())
	e := NewMatrixEngine(m)

	e.AddNeuron("The quick brown fox", nil, nil)
	e.AddNeuron("quick fox", nil, nil)

	searcher := NewSearcher(m)
	results := searcher.Search("quick brown", 0, 10)

	// Exact phrase should rank higher
	if len(results) > 0 && results[0].Content != "The quick brown fox" {
		t.Error("Exact phrase match should rank highest")
	}
}

func TestSearcherSpreadActivation(t *testing.T) {
	m := core.NewMatrix("test-user", core.DefaultBounds())
	e := NewMatrixEngine(m)

	n1, _ := e.AddNeuron("TypeScript", nil, nil)
	n2, _ := e.AddNeuron("React framework", nil, nil)

	// Create synapse between them
	syn := core.NewSynapse(n1.ID, n2.ID, 0.8)
	m.Synapses[syn.ID] = syn
	m.Adjacency[n1.ID] = append(m.Adjacency[n1.ID], n2.ID)
	m.Adjacency[n2.ID] = append(m.Adjacency[n2.ID], n1.ID)

	searcher := NewSearcher(m)

	// Search with spread depth
	results := searcher.Search("TypeScript", 1, 10)

	// Should find both - direct match and connected
	if len(results) < 2 {
		t.Errorf("Expected spread to find connected neuron, got %d results", len(results))
	}
}

func TestSearcherNoMatch(t *testing.T) {
	m := core.NewMatrix("test-user", core.DefaultBounds())
	e := NewMatrixEngine(m)

	e.AddNeuron("TypeScript programming", nil, nil)

	searcher := NewSearcher(m)
	results := searcher.Search("Python", 0, 10)

	if len(results) != 0 {
		t.Errorf("Expected 0 results for non-matching query, got %d", len(results))
	}
}

func TestSearcherLimit(t *testing.T) {
	m := core.NewMatrix("test-user", core.DefaultBounds())
	e := NewMatrixEngine(m)

	for i := 0; i < 10; i++ {
		e.AddNeuron("test content number "+string(rune('A'+i)), nil, nil)
	}

	searcher := NewSearcher(m)
	results := searcher.Search("test", 0, 3)

	if len(results) != 3 {
		t.Errorf("Expected 3 results with limit, got %d", len(results))
	}
}

func TestSearcherEmptyQuery(t *testing.T) {
	m := core.NewMatrix("test-user", core.DefaultBounds())
	e := NewMatrixEngine(m)

	e.AddNeuron("Test content", nil, nil)

	searcher := NewSearcher(m)
	results := searcher.Search("", 0, 10)

	if len(results) != 0 {
		t.Error("Empty query should return no results")
	}
}

func TestSearcherEmptyMatrix(t *testing.T) {
	m := core.NewMatrix("test-user", core.DefaultBounds())

	searcher := NewSearcher(m)
	results := searcher.Search("test", 0, 10)

	if len(results) != 0 {
		t.Error("Empty matrix should return no results")
	}
}

func TestLevenshteinDistance(t *testing.T) {
	tests := []struct {
		a, b     string
		expected int
	}{
		{"", "", 0},
		{"a", "", 1},
		{"", "a", 1},
		{"abc", "abc", 0},
		{"abc", "abd", 1},
		{"abc", "adc", 1},
		{"abc", "abcd", 1},
		{"kitten", "sitting", 3},
	}

	for _, tt := range tests {
		result := levenshteinDistance(tt.a, tt.b)
		if result != tt.expected {
			t.Errorf("levenshteinDistance(%q, %q) = %d, expected %d",
				tt.a, tt.b, result, tt.expected)
		}
	}
}

func TestTokenize(t *testing.T) {
	tests := []struct {
		input    string
		expected int // number of tokens
	}{
		{"Hello World", 2},
		{"Hello, World!", 2},
		{"a b c", 0}, // single chars filtered
		{"TypeScript Programming", 2},
		{"", 0},
	}

	for _, tt := range tests {
		tokens := tokenize(tt.input)
		if len(tokens) != tt.expected {
			t.Errorf("tokenize(%q) returned %d tokens, expected %d",
				tt.input, len(tokens), tt.expected)
		}
	}
}

func TestSearcherEnergyBoost(t *testing.T) {
	m := core.NewMatrix("test-user", core.DefaultBounds())
	e := NewMatrixEngine(m)

	n1, _ := e.AddNeuron("test content one", nil, nil)
	n2, _ := e.AddNeuron("test content two", nil, nil)

	// Lower energy for n1
	n1.Energy = 0.2
	n2.Energy = 1.0

	searcher := NewSearcher(m)
	results := searcher.Search("test", 0, 10)

	// Higher energy neuron should rank first
	if len(results) >= 2 && results[0].ID != n2.ID {
		t.Error("Higher energy neuron should rank higher")
	}
}

func TestSearcherDepthPenalty(t *testing.T) {
	m := core.NewMatrix("test-user", core.DefaultBounds())
	e := NewMatrixEngine(m)

	n1, _ := e.AddNeuron("test surface", nil, nil)
	n2, _ := e.AddNeuron("test deep", nil, nil)

	n1.Depth = 0
	n2.Depth = 5

	searcher := NewSearcher(m)
	results := searcher.Search("test", 0, 10)

	// Surface neuron should rank higher due to depth penalty
	if len(results) >= 2 && results[0].ID != n1.ID {
		t.Error("Surface neuron should rank higher than deep neuron")
	}
}

func TestSearcherTokenCacheParityAndInvalidation(t *testing.T) {
	m := core.NewMatrix("test-user", core.DefaultBounds())
	e := NewMatrixEngine(m)

	n, err := e.AddNeuron("Go language basics", nil, nil)
	if err != nil {
		t.Fatalf("AddNeuron failed: %v", err)
	}

	searcher := NewSearcher(m)
	cold := searcher.Search("go", 0, 10)
	warm := searcher.Search("go", 0, 10)

	if len(cold) != len(warm) {
		t.Fatalf("cold/warm result length mismatch: %d vs %d", len(cold), len(warm))
	}
	for i := range cold {
		if cold[i].ID != warm[i].ID {
			t.Fatalf("cold/warm ranking mismatch at %d: %s vs %s", i, cold[i].ID, warm[i].ID)
		}
	}

	entry, ok := searcher.tokenCache[n.ID]
	if !ok {
		t.Fatal("expected token cache entry after search")
	}
	if entry.hash != n.ContentHash {
		t.Fatalf("expected cached hash %s, got %s", n.ContentHash, entry.hash)
	}

	if err := e.UpdateNeuron(n.ID, "Rust systems programming"); err != nil {
		t.Fatalf("UpdateNeuron failed: %v", err)
	}

	after := searcher.Search("rust", 0, 10)
	if len(after) == 0 || after[0].ID != n.ID {
		t.Fatal("expected updated neuron to match rust query")
	}

	entry, ok = searcher.tokenCache[n.ID]
	if !ok {
		t.Fatal("expected token cache entry after update search")
	}
	if entry.hash != n.ContentHash {
		t.Fatalf("expected cache invalidation to refresh hash to %s, got %s", n.ContentHash, entry.hash)
	}
}

func TestMetadataBoostRanksMatchingNeuronHigher(t *testing.T) {
	m := core.NewMatrix("test-user", core.DefaultBounds())
	e := NewMatrixEngine(m)

	// Two neurons with identical content — one has thread_id metadata
	n1, _ := e.AddNeuron("memory and learning in neural systems", nil, map[string]string{"thread_id": "conv-abc"})
	n2, _ := e.AddNeuron("memory and learning in neural systems extra", nil, nil)

	results := e.Search("memory learning neural", 0, 10, map[string]string{"thread_id": "conv-abc"}, false)

	if len(results) < 2 {
		t.Fatalf("expected at least 2 results, got %d", len(results))
	}
	if results[0].ID != n1.ID {
		t.Errorf("expected metadata-matching neuron (%s) to rank first, got %s", n1.ID, results[0].ID)
	}
	_ = n2
}

func TestMetadataStrictFilterExcludesNonMatching(t *testing.T) {
	m := core.NewMatrix("test-user", core.DefaultBounds())
	e := NewMatrixEngine(m)

	n1, _ := e.AddNeuron("quantum entanglement physics", nil, map[string]string{"thread_id": "conv-xyz"})
	_, _ = e.AddNeuron("quantum entanglement physics extra", nil, nil)

	results := e.Search("quantum entanglement", 0, 10, map[string]string{"thread_id": "conv-xyz"}, true)

	if len(results) != 1 {
		t.Fatalf("strict filter: expected 1 result, got %d", len(results))
	}
	if results[0].ID != n1.ID {
		t.Errorf("strict filter: expected neuron %s, got %s", n1.ID, results[0].ID)
	}
}

func TestMetadataWritePreservesOnNeuron(t *testing.T) {
	m := core.NewMatrix("test-user", core.DefaultBounds())
	e := NewMatrixEngine(m)

	n, err := e.AddNeuron("test content for metadata", nil, map[string]string{
		"thread_id": "t-001",
		"role":      "user",
	})
	if err != nil {
		t.Fatalf("AddNeuron failed: %v", err)
	}

	if v, ok := n.Metadata["thread_id"]; !ok || v != "t-001" {
		t.Errorf("expected thread_id=t-001, got %v", n.Metadata["thread_id"])
	}
	if v, ok := n.Metadata["role"]; !ok || v != "user" {
		t.Errorf("expected role=user, got %v", n.Metadata["role"])
	}
}

func TestMetadataNoFilterReturnsAll(t *testing.T) {
	m := core.NewMatrix("test-user", core.DefaultBounds())
	e := NewMatrixEngine(m)

	e.AddNeuron("fractal topology network", nil, map[string]string{"thread_id": "t-a"})
	e.AddNeuron("fractal topology network extended", nil, map[string]string{"thread_id": "t-b"})
	e.AddNeuron("fractal topology network more", nil, nil)

	results := e.Search("fractal topology", 0, 10, nil, false)

	if len(results) != 3 {
		t.Errorf("no metadata filter: expected 3 results, got %d", len(results))
	}
}
