package e2e

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/qubicDB/qubicdb/pkg/concurrency"
	"github.com/qubicDB/qubicdb/pkg/core"
	"github.com/qubicDB/qubicdb/pkg/persistence"
	"github.com/qubicDB/qubicdb/pkg/vector"
)

func BenchmarkVectorizerEmbedTextLive(b *testing.B) {
	v := openLiveVectorizerOrSkip(b)
	defer v.Close()

	inputs := []string{
		"distributed systems consistency and consensus",
		"vector databases and semantic retrieval",
		"lifecycle and memory consolidation in adaptive systems",
		"concurrency deadlock detection and throughput tuning",
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := v.EmbedText(inputs[i%len(inputs)]); err != nil {
			b.Fatalf("EmbedText failed: %v", err)
		}
	}
}

func BenchmarkHybridSearchLiveVectorizer(b *testing.B) {
	v := openLiveVectorizerOrSkip(b)
	defer v.Close()

	tmpDir, err := os.MkdirTemp("", "qubicdb-vector-live-bench-*")
	if err != nil {
		b.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	durability := persistence.DefaultDurabilityConfig()
	durability.FsyncPolicy = persistence.FsyncPolicyInterval
	durability.FsyncInterval = 100 * time.Millisecond

	store, err := persistence.NewStoreWithDurability(tmpDir, true, durability)
	if err != nil {
		b.Fatalf("failed to create store: %v", err)
	}

	pool := concurrency.NewWorkerPool(store, core.DefaultBounds())
	defer pool.Shutdown()

	pool.SetVectorizer(v, 0.6)

	worker, err := pool.GetOrCreate("vector-bench-user")
	if err != nil {
		b.Fatalf("failed to create worker: %v", err)
	}

	for i := 0; i < 400; i++ {
		if _, err := worker.Submit(&concurrency.Operation{
			Type: concurrency.OpWrite,
			Payload: concurrency.AddNeuronRequest{
				Content: fmt.Sprintf("Document %d about distributed consensus and vector retrieval", i),
			},
		}); err != nil {
			b.Fatalf("setup write failed: %v", err)
		}
	}

	queries := []string{
		"distributed consensus",
		"vector retrieval",
		"deadlock detection",
		"memory consolidation",
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		result, err := worker.Submit(&concurrency.Operation{
			Type: concurrency.OpSearch,
			Payload: concurrency.SearchRequest{
				Query: queries[i%len(queries)],
				Depth: 2,
				Limit: 20,
			},
		})
		if err != nil {
			b.Fatalf("search failed: %v", err)
		}

		if len(result.([]*core.Neuron)) == 0 {
			b.Fatalf("search returned zero results for query %q", queries[i%len(queries)])
		}
	}
}

func openLiveVectorizerOrSkip(tb testing.TB) *vector.Vectorizer {
	tb.Helper()

	if !vector.IsLibraryAvailable() {
		tb.Skip("llama.cpp shared library is not available")
	}

	modelPath := os.Getenv("QUBICDB_VECTOR_MODEL_PATH")
	if modelPath == "" {
		modelPath = filepath.Join("dist", "MiniLM-L6-v2.Q8_0.gguf")
	}

	if _, err := os.Stat(modelPath); err != nil {
		tb.Skipf("vector model not found at %s", modelPath)
	}

	v, err := vector.NewVectorizer(modelPath, 0, 0)
	if err != nil {
		tb.Skipf("failed to initialize vectorizer: %v", err)
	}

	return v
}
