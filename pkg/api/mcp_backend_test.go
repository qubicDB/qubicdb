package api

import (
	"context"
	"testing"

	"github.com/qubicDB/qubicdb/pkg/concurrency"
	"github.com/qubicDB/qubicdb/pkg/core"
	"github.com/qubicDB/qubicdb/pkg/lifecycle"
	"github.com/qubicDB/qubicdb/pkg/persistence"
	"github.com/qubicDB/qubicdb/pkg/registry"
)

func newTestMCPBackend(t *testing.T) *mcpBackend {
	t.Helper()

	cfg := core.DefaultConfig()
	cfg.Storage.DataPath = t.TempDir()
	cfg.Registry.Enabled = false // Allow any UUID

	store, err := persistence.NewStore(cfg.Storage.DataPath, cfg.Storage.Compress)
	if err != nil {
		t.Fatalf("persistence.NewStore: %v", err)
	}

	bounds := core.MatrixBounds{
		MinDimension: cfg.Matrix.MinDimension,
		MaxDimension: cfg.Matrix.MaxDimension,
		MaxNeurons:   cfg.Matrix.MaxNeurons,
	}

	pool := concurrency.NewWorkerPool(store, bounds)
	lm := lifecycle.NewManager()
	reg, err := registry.NewStore(cfg.Storage.DataPath)
	if err != nil {
		t.Fatalf("registry.NewStore: %v", err)
	}

	server := NewServer(cfg.Server.HTTPAddr, pool, lm, reg, cfg)
	return newMCPBackend(server)
}

func TestMCPBackend_ListIndexes_Empty(t *testing.T) {
	b := newTestMCPBackend(t)
	ctx := context.Background()

	result, err := b.ListIndexes(ctx, true, 100)
	if err != nil {
		t.Fatalf("ListIndexes failed: %v", err)
	}

	indexes := result["indexes"].([]map[string]any)
	if len(indexes) != 0 {
		t.Errorf("expected 0 indexes, got %d", len(indexes))
	}
}

func TestMCPBackend_ListIndexes_WithActiveIndexes(t *testing.T) {
	b := newTestMCPBackend(t)
	ctx := context.Background()

	// Create some indexes by writing to them
	_, err := b.Write(ctx, "test-index-1", "Hello world", nil)
	if err != nil {
		t.Fatalf("Write failed: %v", err)
	}
	_, err = b.Write(ctx, "test-index-2", "Another memory", nil)
	if err != nil {
		t.Fatalf("Write failed: %v", err)
	}

	result, err := b.ListIndexes(ctx, true, 100)
	if err != nil {
		t.Fatalf("ListIndexes failed: %v", err)
	}

	indexes := result["indexes"].([]map[string]any)
	if len(indexes) != 2 {
		t.Errorf("expected 2 indexes, got %d", len(indexes))
	}

	// Check that total_active matches
	totalActive := result["total_active"].(int)
	if totalActive != 2 {
		t.Errorf("expected total_active=2, got %d", totalActive)
	}
}

func TestMCPBackend_GlobalSearch(t *testing.T) {
	b := newTestMCPBackend(t)
	ctx := context.Background()

	// Create multiple indexes with content
	_, _ = b.Write(ctx, "repo-frontend", "React component for user dashboard", nil)
	_, _ = b.Write(ctx, "repo-backend", "API endpoint for user authentication", nil)
	_, _ = b.Write(ctx, "repo-shared", "Shared utilities for user management", nil)

	// Global search across all indexes
	result, err := b.GlobalSearch(ctx, "user", 2, 10, nil)
	if err != nil {
		t.Fatalf("GlobalSearch failed: %v", err)
	}

	totalResults := result["total_results"].(int)
	if totalResults != 3 {
		t.Errorf("expected 3 results, got %d", totalResults)
	}

	indexesSearched := result["indexes_searched"].(int)
	if indexesSearched != 3 {
		t.Errorf("expected 3 indexes searched, got %d", indexesSearched)
	}

	// Verify results have _index field
	results := result["results"].([]map[string]any)
	for _, r := range results {
		if _, ok := r["_index"]; !ok {
			t.Error("result missing _index field")
		}
	}
}

func TestMCPBackend_MultiSearch(t *testing.T) {
	b := newTestMCPBackend(t)
	ctx := context.Background()

	// Create multiple indexes
	_, _ = b.Write(ctx, "brain-repo1", "TypeScript project configuration", nil)
	_, _ = b.Write(ctx, "brain-repo2", "Python project setup", nil)
	_, _ = b.Write(ctx, "brain-repo3", "Go project structure", nil)

	// Search only specific indexes
	result, err := b.MultiSearch(ctx, []string{"brain-repo1", "brain-repo2"}, "project", 2, 10, nil)
	if err != nil {
		t.Fatalf("MultiSearch failed: %v", err)
	}

	indexesSearched := result["indexes_searched"].(int)
	if indexesSearched != 2 {
		t.Errorf("expected 2 indexes searched, got %d", indexesSearched)
	}

	// Verify results_by_index contains both indexes
	resultsByIndex := result["results_by_index"].(map[string][]map[string]any)
	if _, ok := resultsByIndex["brain-repo1"]; !ok {
		t.Error("missing results for brain-repo1")
	}
	if _, ok := resultsByIndex["brain-repo2"]; !ok {
		t.Error("missing results for brain-repo2")
	}
}

func TestMCPBackend_RecentIndexes(t *testing.T) {
	b := newTestMCPBackend(t)
	ctx := context.Background()

	// Create indexes with some activity
	_, _ = b.Write(ctx, "old-index", "Old content", nil)
	_, _ = b.Write(ctx, "new-index", "New content", nil)
	// Do more operations on new-index to make it "more recent"
	_, _ = b.Search(ctx, "new-index", "content", 2, 10, nil, false)

	result, err := b.RecentIndexes(ctx, 10, 0)
	if err != nil {
		t.Fatalf("RecentIndexes failed: %v", err)
	}

	indexes := result["indexes"].([]map[string]any)
	if len(indexes) != 2 {
		t.Errorf("expected 2 indexes, got %d", len(indexes))
	}

	// First index should be the most recent (new-index)
	if len(indexes) > 0 {
		firstIndex := indexes[0]["index_id"].(string)
		if firstIndex != "new-index" {
			t.Errorf("expected new-index to be first (most recent), got %s", firstIndex)
		}
	}
}

func TestMCPBackend_RecentIndexes_MinNeurons(t *testing.T) {
	b := newTestMCPBackend(t)
	ctx := context.Background()

	// Create one index with 1 neuron
	_, _ = b.Write(ctx, "small-index", "Single neuron", nil)

	// Create another index with 3 neurons
	_, _ = b.Write(ctx, "large-index", "First neuron", nil)
	_, _ = b.Write(ctx, "large-index", "Second neuron", nil)
	_, _ = b.Write(ctx, "large-index", "Third neuron", nil)

	// Filter by min_neurons=2
	result, err := b.RecentIndexes(ctx, 10, 2)
	if err != nil {
		t.Fatalf("RecentIndexes failed: %v", err)
	}

	indexes := result["indexes"].([]map[string]any)
	if len(indexes) != 1 {
		t.Errorf("expected 1 index with min_neurons=2, got %d", len(indexes))
	}

	if len(indexes) > 0 && indexes[0]["index_id"].(string) != "large-index" {
		t.Errorf("expected large-index, got %s", indexes[0]["index_id"])
	}
}

func TestMCPBackend_GlobalSearch_Empty(t *testing.T) {
	b := newTestMCPBackend(t)
	ctx := context.Background()

	// Search with no indexes
	result, err := b.GlobalSearch(ctx, "test", 2, 10, nil)
	if err != nil {
		t.Fatalf("GlobalSearch failed: %v", err)
	}

	totalResults := result["total_results"].(int)
	if totalResults != 0 {
		t.Errorf("expected 0 results, got %d", totalResults)
	}
}

func TestMCPBackend_GlobalSearch_WithMetadata(t *testing.T) {
	b := newTestMCPBackend(t)
	ctx := context.Background()

	// Create indexes with metadata
	_, _ = b.Write(ctx, "repo-a", "Decision to use React", map[string]string{"type": "decision"})
	_, _ = b.Write(ctx, "repo-b", "Note about React hooks", map[string]string{"type": "note"})

	// Search with metadata filter
	result, err := b.GlobalSearch(ctx, "React", 2, 10, map[string]string{"type": "decision"})
	if err != nil {
		t.Fatalf("GlobalSearch failed: %v", err)
	}

	// Both should be returned but "decision" type should be boosted
	results := result["results"].([]map[string]any)
	if len(results) == 0 {
		t.Error("expected at least 1 result")
	}
}
