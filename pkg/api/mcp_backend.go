package api

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/qubicDB/qubicdb/pkg/api/apierr"
	"github.com/qubicDB/qubicdb/pkg/concurrency"
	"github.com/qubicDB/qubicdb/pkg/core"
	"github.com/qubicDB/qubicdb/pkg/protocol"
)

type mcpBackend struct {
	server *Server
}

func newMCPBackend(s *Server) *mcpBackend {
	return &mcpBackend{server: s}
}

func (b *mcpBackend) Write(_ context.Context, indexID, content string, metadata map[string]string) (map[string]any, error) {
	worker, err := b.getWorker(indexID)
	if err != nil {
		return nil, err
	}

	result, err := worker.Submit(&concurrency.Operation{
		Type: concurrency.OpWrite,
		Payload: concurrency.AddNeuronRequest{
			Content:  content,
			Metadata: metadata,
		},
	})
	if err != nil {
		return nil, err
	}

	n := result.(*core.Neuron)
	doc := protocol.NeuronToDocument(n, nil)
	doc["id"] = doc["_id"]
	return doc, nil
}

func (b *mcpBackend) Read(_ context.Context, indexID, neuronID string) (map[string]any, error) {
	worker, err := b.getWorker(indexID)
	if err != nil {
		return nil, err
	}

	result, err := worker.Submit(&concurrency.Operation{
		Type:    concurrency.OpRead,
		Payload: core.NeuronID(neuronID),
	})
	if err != nil {
		return nil, fmt.Errorf("neuron not found")
	}

	n := result.(*core.Neuron)
	return protocol.NeuronToDocument(n, nil), nil
}

func (b *mcpBackend) Search(_ context.Context, indexID, query string, depth, limit int, metadata map[string]string, strict bool) (map[string]any, error) {
	worker, err := b.getWorker(indexID)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(query) == "" {
		return nil, fmt.Errorf("query is required")
	}

	depth = clampPositive(depth, defaultSearchDepth, maxSearchDepth)
	limit = clampPositive(limit, defaultSearchLimit, maxSearchLimit)

	result, err := worker.Submit(&concurrency.Operation{
		Type: concurrency.OpSearch,
		Payload: concurrency.SearchRequest{
			Query:    query,
			Depth:    depth,
			Limit:    limit,
			Metadata: metadata,
			Strict:   strict,
		},
	})
	if err != nil {
		return nil, err
	}

	neurons := result.([]*core.Neuron)
	docs := make([]map[string]any, 0, len(neurons))
	for _, n := range neurons {
		docs = append(docs, protocol.NeuronToDocument(n, nil))
	}

	return map[string]any{
		"results": docs,
		"count":   len(docs),
		"query":   query,
		"depth":   depth,
	}, nil
}

func (b *mcpBackend) Recall(_ context.Context, indexID string, limit int) (map[string]any, error) {
	worker, err := b.getWorker(indexID)
	if err != nil {
		return nil, err
	}

	limit = clampPositive(limit, 100, 500)

	result, err := worker.Submit(&concurrency.Operation{
		Type: concurrency.OpRecall,
		Payload: concurrency.ListNeuronsRequest{
			Offset: 0,
			Limit:  limit,
		},
	})
	if err != nil {
		return nil, err
	}

	neurons := result.([]*core.Neuron)
	items := make([]map[string]any, len(neurons))
	for i, n := range neurons {
		items[i] = protocol.NeuronToDocument(n, nil)
	}

	return map[string]any{
		"memories": items,
		"neurons":  items,
		"count":    len(items),
	}, nil
}

func (b *mcpBackend) Context(_ context.Context, indexID, cue string, depth, maxTokens int) (map[string]any, error) {
	worker, err := b.getWorker(indexID)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(cue) == "" {
		return nil, fmt.Errorf("cue is required")
	}

	maxTokens = clampPositive(maxTokens, defaultContextTokens, maxContextTokens)
	depth = clampPositive(depth, defaultContextDepth, maxContextDepth)

	result, err := worker.Submit(&concurrency.Operation{
		Type: concurrency.OpSearch,
		Payload: concurrency.SearchRequest{
			Query: cue,
			Depth: depth,
			Limit: 50,
		},
	})
	if err != nil {
		return nil, err
	}

	neurons := result.([]*core.Neuron)
	var contextBuilder strings.Builder
	tokenEstimate := 0
	included := 0

	for _, n := range neurons {
		neuronTokens := len(n.Content) / 4
		if tokenEstimate+neuronTokens > maxTokens {
			break
		}

		if contextBuilder.Len() > 0 {
			contextBuilder.WriteString("\n---\n")
		}
		contextBuilder.WriteString(n.Content)
		if n.Depth > 0 {
			contextBuilder.WriteString(fmt.Sprintf(" [depth:%d]", n.Depth))
		}

		tokenEstimate += neuronTokens
		included++
	}

	assembled := contextBuilder.String()
	return map[string]any{
		"context":         assembled,
		"text":            assembled,
		"neuronsUsed":     included,
		"neuronCount":     included,
		"estimatedTokens": tokenEstimate,
		"tokenCount":      tokenEstimate,
		"cue":             cue,
	}, nil
}

func (b *mcpBackend) RegistryFindOrCreate(_ context.Context, uuid string, metadata map[string]any) (map[string]any, error) {
	if strings.TrimSpace(uuid) == "" {
		return nil, fmt.Errorf("uuid is required")
	}

	entry, created, err := b.server.registry.FindOrCreate(uuid, metadata)
	if err != nil {
		return nil, err
	}

	return map[string]any{
		"uuid":      entry.UUID,
		"metadata":  entry.Metadata,
		"created":   created,
		"createdAt": entry.CreatedAt,
		"updatedAt": entry.UpdatedAt,
	}, nil
}

func (b *mcpBackend) getWorker(indexID string) (*concurrency.BrainWorker, error) {
	worker, err := b.server.getWorker(core.IndexID(indexID))
	if err != nil {
		msg := err.Error()
		switch {
		case strings.HasPrefix(msg, apierr.CodeIndexIDRequired):
			return nil, fmt.Errorf("index_id is required")
		case strings.HasPrefix(msg, apierr.CodeUUIDNotRegistered):
			return nil, fmt.Errorf(msg)
		default:
			return nil, err
		}
	}
	return worker, nil
}

// ── Cross-index / Global operations ──────────────────────────────────────────

// ListIndexes returns all registered indexes with their metadata and stats.
func (b *mcpBackend) ListIndexes(_ context.Context, activeOnly bool, limit int) (map[string]any, error) {
	if limit <= 0 {
		limit = 100
	}

	var indexes []map[string]any

	if activeOnly {
		// Only return currently loaded/active indexes from the worker pool
		activeIDs := b.server.pool.ListIndexes()
		for i, id := range activeIDs {
			if i >= limit {
				break
			}
			entry := map[string]any{
				"index_id": id,
				"active":   true,
			}
			// Try to get stats from the worker
			if worker, err := b.server.pool.Get(core.IndexID(id)); err == nil {
				stats := worker.Stats()
				entry["last_op"] = stats["last_op"]
				entry["ops_processed"] = stats["ops_processed"]
				// Get neuron count from matrix
				m := worker.Matrix()
				m.RLock()
				entry["neuron_count"] = len(m.Neurons)
				entry["synapse_count"] = len(m.Synapses)
				m.RUnlock()
			}
			// Add registry metadata if available
			if regEntry, ok := b.server.registry.Get(id); ok {
				entry["metadata"] = regEntry.Metadata
				entry["created_at"] = regEntry.CreatedAt
				entry["updated_at"] = regEntry.UpdatedAt
			}
			indexes = append(indexes, entry)
		}
	} else {
		// Return all registered indexes
		regEntries := b.server.registry.List()
		for i, entry := range regEntries {
			if i >= limit {
				break
			}
			indexEntry := map[string]any{
				"index_id":   entry.UUID,
				"metadata":   entry.Metadata,
				"created_at": entry.CreatedAt,
				"updated_at": entry.UpdatedAt,
			}
			// Check if active
			if worker, err := b.server.pool.Get(core.IndexID(entry.UUID)); err == nil {
				indexEntry["active"] = true
				stats := worker.Stats()
				indexEntry["last_op"] = stats["last_op"]
				indexEntry["ops_processed"] = stats["ops_processed"]
				m := worker.Matrix()
				m.RLock()
				indexEntry["neuron_count"] = len(m.Neurons)
				indexEntry["synapse_count"] = len(m.Synapses)
				m.RUnlock()
			} else {
				indexEntry["active"] = false
			}
			indexes = append(indexes, indexEntry)
		}
	}

	return map[string]any{
		"indexes":      indexes,
		"count":        len(indexes),
		"active_only":  activeOnly,
		"total_active": b.server.pool.ActiveCount(),
	}, nil
}

// GlobalSearch searches across ALL active indexes using semantic/vector similarity.
func (b *mcpBackend) GlobalSearch(_ context.Context, query string, depth, limit int, metadata map[string]string) (map[string]any, error) {
	if strings.TrimSpace(query) == "" {
		return nil, fmt.Errorf("query is required")
	}

	depth = clampPositive(depth, defaultSearchDepth, maxSearchDepth)
	limit = clampPositive(limit, 10, 50)

	activeIDs := b.server.pool.ListIndexes()
	if len(activeIDs) == 0 {
		return map[string]any{
			"results":        []any{},
			"indexes_found":  []string{},
			"total_results":  0,
			"indexes_searched": 0,
			"query":          query,
		}, nil
	}

	// Search all indexes concurrently
	type indexResult struct {
		indexID string
		results []map[string]any
		err     error
	}

	resultChan := make(chan indexResult, len(activeIDs))
	var wg sync.WaitGroup

	for _, id := range activeIDs {
		wg.Add(1)
		go func(indexID string) {
			defer wg.Done()
			worker, err := b.server.pool.Get(core.IndexID(indexID))
			if err != nil {
				resultChan <- indexResult{indexID: indexID, err: err}
				return
			}

			result, err := worker.Submit(&concurrency.Operation{
				Type: concurrency.OpSearch,
				Payload: concurrency.SearchRequest{
					Query:    query,
					Depth:    depth,
					Limit:    limit,
					Metadata: metadata,
					Strict:   false,
				},
			})
			if err != nil {
				resultChan <- indexResult{indexID: indexID, err: err}
				return
			}

			neurons := result.([]*core.Neuron)
			docs := make([]map[string]any, 0, len(neurons))
			for _, n := range neurons {
				doc := protocol.NeuronToDocument(n, nil)
				doc["_index"] = indexID
				docs = append(docs, doc)
			}
			resultChan <- indexResult{indexID: indexID, results: docs}
		}(id)
	}

	// Wait for all searches to complete
	go func() {
		wg.Wait()
		close(resultChan)
	}()

	// Collect and merge results
	var allResults []map[string]any
	indexesWithResults := make([]string, 0)

	for res := range resultChan {
		if res.err == nil && len(res.results) > 0 {
			allResults = append(allResults, res.results...)
			indexesWithResults = append(indexesWithResults, res.indexID)
		}
	}

	// Sort by score (energy * accessCount as proxy)
	sort.Slice(allResults, func(i, j int) bool {
		scoreI := getFloat(allResults[i], "energy", 0) * float64(getIntFromMap(allResults[i], "accessCount", 1))
		scoreJ := getFloat(allResults[j], "energy", 0) * float64(getIntFromMap(allResults[j], "accessCount", 1))
		return scoreI > scoreJ
	})

	// Limit total results
	maxTotal := limit * 5
	if len(allResults) > maxTotal {
		allResults = allResults[:maxTotal]
	}

	return map[string]any{
		"results":          allResults,
		"indexes_found":    indexesWithResults,
		"total_results":    len(allResults),
		"indexes_searched": len(activeIDs),
		"query":            query,
		"depth":            depth,
	}, nil
}

// MultiSearch searches across a specific list of indexes.
func (b *mcpBackend) MultiSearch(_ context.Context, indexIDs []string, query string, depth, limit int, metadata map[string]string) (map[string]any, error) {
	if len(indexIDs) == 0 {
		return nil, fmt.Errorf("index_ids cannot be empty")
	}
	if strings.TrimSpace(query) == "" {
		return nil, fmt.Errorf("query is required")
	}

	depth = clampPositive(depth, defaultSearchDepth, maxSearchDepth)
	limit = clampPositive(limit, 10, 50)

	// Search specified indexes concurrently
	type indexResult struct {
		indexID string
		results []map[string]any
		err     error
	}

	resultChan := make(chan indexResult, len(indexIDs))
	var wg sync.WaitGroup

	for _, id := range indexIDs {
		wg.Add(1)
		go func(indexID string) {
			defer wg.Done()
			worker, err := b.server.pool.GetOrCreate(core.IndexID(indexID))
			if err != nil {
				resultChan <- indexResult{indexID: indexID, err: err}
				return
			}

			result, err := worker.Submit(&concurrency.Operation{
				Type: concurrency.OpSearch,
				Payload: concurrency.SearchRequest{
					Query:    query,
					Depth:    depth,
					Limit:    limit,
					Metadata: metadata,
					Strict:   false,
				},
			})
			if err != nil {
				resultChan <- indexResult{indexID: indexID, err: err}
				return
			}

			neurons := result.([]*core.Neuron)
			docs := make([]map[string]any, 0, len(neurons))
			for _, n := range neurons {
				doc := protocol.NeuronToDocument(n, nil)
				doc["_index"] = indexID
				docs = append(docs, doc)
			}
			resultChan <- indexResult{indexID: indexID, results: docs}
		}(id)
	}

	go func() {
		wg.Wait()
		close(resultChan)
	}()

	// Group results by index
	resultsByIndex := make(map[string][]map[string]any)
	var allResults []map[string]any
	errors := make(map[string]string)

	for res := range resultChan {
		if res.err != nil {
			errors[res.indexID] = res.err.Error()
			continue
		}
		resultsByIndex[res.indexID] = res.results
		allResults = append(allResults, res.results...)
	}

	// Sort merged results by score
	sort.Slice(allResults, func(i, j int) bool {
		scoreI := getFloat(allResults[i], "energy", 0) * float64(getIntFromMap(allResults[i], "accessCount", 1))
		scoreJ := getFloat(allResults[j], "energy", 0) * float64(getIntFromMap(allResults[j], "accessCount", 1))
		return scoreI > scoreJ
	})

	return map[string]any{
		"results":          allResults,
		"results_by_index": resultsByIndex,
		"total_results":    len(allResults),
		"indexes_searched": len(indexIDs),
		"errors":           errors,
		"query":            query,
		"depth":            depth,
	}, nil
}

// RecentIndexes returns the most recently active indexes sorted by last operation time.
func (b *mcpBackend) RecentIndexes(_ context.Context, limit int, minNeurons int) (map[string]any, error) {
	if limit <= 0 {
		limit = 20
	}

	type indexInfo struct {
		IndexID      string         `json:"index_id"`
		LastOp       time.Time      `json:"last_op"`
		OpsProcessed uint64         `json:"ops_processed"`
		NeuronCount  int            `json:"neuron_count"`
		SynapseCount int            `json:"synapse_count"`
		Metadata     map[string]any `json:"metadata,omitempty"`
		Active       bool           `json:"active"`
	}

	var indexes []indexInfo

	// Collect stats from all active workers
	b.server.pool.ForEach(func(id core.IndexID, worker *concurrency.BrainWorker) {
		stats := worker.Stats()
		m := worker.Matrix()
		m.RLock()
		neuronCount := len(m.Neurons)
		synapseCount := len(m.Synapses)
		m.RUnlock()

		// Filter by minimum neurons
		if minNeurons > 0 && neuronCount < minNeurons {
			return
		}

		info := indexInfo{
			IndexID:      string(id),
			LastOp:       stats["last_op"].(time.Time),
			OpsProcessed: stats["ops_processed"].(uint64),
			NeuronCount:  neuronCount,
			SynapseCount: synapseCount,
			Active:       true,
		}

		// Add registry metadata if available
		if entry, ok := b.server.registry.Get(string(id)); ok {
			info.Metadata = entry.Metadata
		}

		indexes = append(indexes, info)
	})

	// Sort by last operation time (most recent first)
	sort.Slice(indexes, func(i, j int) bool {
		return indexes[i].LastOp.After(indexes[j].LastOp)
	})

	// Limit results
	if len(indexes) > limit {
		indexes = indexes[:limit]
	}

	// Convert to []map[string]any for JSON serialization
	result := make([]map[string]any, len(indexes))
	for i, idx := range indexes {
		result[i] = map[string]any{
			"index_id":      idx.IndexID,
			"last_op":       idx.LastOp,
			"ops_processed": idx.OpsProcessed,
			"neuron_count":  idx.NeuronCount,
			"synapse_count": idx.SynapseCount,
			"metadata":      idx.Metadata,
			"active":        idx.Active,
		}
	}

	return map[string]any{
		"indexes":      result,
		"count":        len(result),
		"total_active": b.server.pool.ActiveCount(),
	}, nil
}

// Helper functions for extracting values from map[string]any
func getFloat(m map[string]any, key string, def float64) float64 {
	if v, ok := m[key].(float64); ok {
		return v
	}
	return def
}

func getIntFromMap(m map[string]any, key string, def int) int {
	switch v := m[key].(type) {
	case int:
		return v
	case int64:
		return int(v)
	case float64:
		return int(v)
	}
	return def
}
