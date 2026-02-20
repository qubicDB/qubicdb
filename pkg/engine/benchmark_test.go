package engine

import (
	"fmt"
	"math/rand"
	"testing"

	"github.com/denizumutdereli/qubicdb/pkg/core"
	"github.com/denizumutdereli/qubicdb/pkg/sentiment"
	"github.com/denizumutdereli/qubicdb/pkg/vector"
)

func BenchmarkMatrixEngineAddNeuron(b *testing.B) {
	m := core.NewMatrix("bench-user", core.DefaultBounds())
	e := NewMatrixEngine(m)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		e.AddNeuron(fmt.Sprintf("Benchmark content %d", i), nil, nil)
	}
}

func BenchmarkMatrixEngineSearch(b *testing.B) {
	m := core.NewMatrix("bench-user", core.DefaultBounds())
	e := NewMatrixEngine(m)

	// Pre-populate with neurons
	for i := 0; i < 1000; i++ {
		e.AddNeuron(fmt.Sprintf("Content about topic %d with TypeScript and Go programming", i), nil, nil)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		e.Search("TypeScript programming", 1, 10, nil, false)
	}
}

func BenchmarkMatrixEngineSearchFuzzy(b *testing.B) {
	m := core.NewMatrix("bench-user", core.DefaultBounds())
	e := NewMatrixEngine(m)

	// Pre-populate
	for i := 0; i < 1000; i++ {
		e.AddNeuron(fmt.Sprintf("Content about various topics %d", i), nil, nil)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		e.Search("topic", 0, 10, nil, false)
	}
}

func BenchmarkMatrixEngineGetNeuron(b *testing.B) {
	m := core.NewMatrix("bench-user", core.DefaultBounds())
	e := NewMatrixEngine(m)

	n, _ := e.AddNeuron("Test neuron", nil, nil)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		e.GetNeuron(n.ID)
	}
}

func BenchmarkMatrixEngineListNeurons(b *testing.B) {
	m := core.NewMatrix("bench-user", core.DefaultBounds())
	e := NewMatrixEngine(m)

	for i := 0; i < 1000; i++ {
		e.AddNeuron(fmt.Sprintf("Neuron %d", i), nil, nil)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		e.ListNeurons(0, 100, nil)
	}
}

func BenchmarkSearcherLevenshtein(b *testing.B) {
	a := "TypeScript"
	c := "typescript"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		levenshteinDistance(a, c)
	}
}

func BenchmarkSearcherTokenize(b *testing.B) {
	text := "The quick brown fox jumps over the lazy dog"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		tokenize(text)
	}
}

func BenchmarkMatrixEngineParallelSearch(b *testing.B) {
	m := core.NewMatrix("bench-user", core.DefaultBounds())
	e := NewMatrixEngine(m)

	for i := 0; i < 1000; i++ {
		e.AddNeuron(fmt.Sprintf("Content %d with programming topics", i), nil, nil)
	}

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			e.Search("programming", 0, 10, nil, false)
		}
	})
}

func BenchmarkSearcherHybridScoreVector384(b *testing.B) {
	m := core.NewMatrix("bench-user", core.DefaultBounds())
	s := NewSearcher(m)
	s.alpha = 0.6

	query := "distributed systems consensus"
	queryLower := "distributed systems consensus"
	queryTokens := tokenize(query)
	queryVec := benchmarkEmbedding(384, 7)

	n := core.NewNeuron("distributed systems and raft consensus", m.CurrentDim)
	n.Embedding = benchmarkEmbedding(384, 11)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = s.scoreNeuron(n, query, queryLower, queryTokens, queryVec, sentiment.LabelNeutral)
	}
}

func BenchmarkSearcherHybridScan5K(b *testing.B) {
	m := core.NewMatrix("bench-user", core.DefaultBounds())
	s := NewSearcher(m)
	s.alpha = 0.6

	neurons := make([]*core.Neuron, 0, 5000)
	for i := 0; i < 5000; i++ {
		n := core.NewNeuron(fmt.Sprintf("Neuron %d distributed systems and search", i), m.CurrentDim)
		n.Embedding = benchmarkEmbedding(384, int64(i+100))
		neurons = append(neurons, n)
	}

	query := "distributed search consensus"
	queryLower := "distributed search consensus"
	queryTokens := tokenize(query)
	queryVec := benchmarkEmbedding(384, 42)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		matches := 0
		for _, n := range neurons {
			if s.scoreNeuron(n, query, queryLower, queryTokens, queryVec, sentiment.LabelNeutral) > 0 {
				matches++
			}
		}
		if matches == 0 {
			b.Fatal("expected at least one match")
		}
	}
}

func benchmarkEmbedding(dim int, seed int64) []float32 {
	r := rand.New(rand.NewSource(seed))
	v := make([]float32, dim)
	for i := range v {
		v[i] = r.Float32()*2 - 1
	}
	vector.Normalize(v)
	return v
}
