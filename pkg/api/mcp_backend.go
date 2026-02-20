package api

import (
	"context"
	"fmt"
	"strings"

	"github.com/denizumutdereli/qubicdb/pkg/api/apierr"
	"github.com/denizumutdereli/qubicdb/pkg/concurrency"
	"github.com/denizumutdereli/qubicdb/pkg/core"
	"github.com/denizumutdereli/qubicdb/pkg/protocol"
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
