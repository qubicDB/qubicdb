package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	mcpproto "github.com/mark3labs/mcp-go/mcp"
	mcpserver "github.com/mark3labs/mcp-go/server"
)

const (
	toolWrite              = "qubicdb_write"
	toolRead               = "qubicdb_read"
	toolSearch             = "qubicdb_search"
	toolRecall             = "qubicdb_recall"
	toolContext            = "qubicdb_context"
	toolRegistryFindCreate = "qubicdb_registry_find_or_create"
)

// Config controls MCP route behavior.
type Config struct {
	APIKey         string
	Stateless      bool
	RateLimitRPS   float64
	RateLimitBurst int
	EnablePrompts  bool
	AllowedTools   []string
}

// Backend is the minimal capability contract exposed to MCP tools.
type Backend interface {
	Write(ctx context.Context, indexID, content string, metadata map[string]string) (map[string]any, error)
	Read(ctx context.Context, indexID, neuronID string) (map[string]any, error)
	Search(ctx context.Context, indexID, query string, depth, limit int, metadata map[string]string, strict bool) (map[string]any, error)
	Recall(ctx context.Context, indexID string, limit int) (map[string]any, error)
	Context(ctx context.Context, indexID, cue string, depth, maxTokens int) (map[string]any, error)
	RegistryFindOrCreate(ctx context.Context, uuid string, metadata map[string]any) (map[string]any, error)
}

// NewHandler builds an MCP streamable HTTP handler with optional API-key auth
// and endpoint-local rate limiting.
func NewHandler(cfg Config, backend Backend) (http.Handler, error) {
	if backend == nil {
		return nil, fmt.Errorf("mcp backend is required")
	}

	s := mcpserver.NewMCPServer(
		"qubicdb-mcp",
		"1.0.0",
		mcpserver.WithToolCapabilities(true),
		mcpserver.WithPromptCapabilities(cfg.EnablePrompts),
		mcpserver.WithRecovery(),
	)

	registerTools(s, backend, cfg.AllowedTools)
	if cfg.EnablePrompts {
		registerPrompts(s)
	}

	streamable := mcpserver.NewStreamableHTTPServer(s, mcpserver.WithStateLess(cfg.Stateless))
	var h http.Handler = http.HandlerFunc(streamable.ServeHTTP)

	if strings.TrimSpace(cfg.APIKey) != "" {
		h = apiKeyMiddleware(strings.TrimSpace(cfg.APIKey), h)
	}
	if cfg.RateLimitRPS > 0 && cfg.RateLimitBurst > 0 {
		h = rateLimitMiddleware(newRateLimiter(cfg.RateLimitRPS, cfg.RateLimitBurst), h)
	}

	return h, nil
}

func registerTools(s *mcpserver.MCPServer, backend Backend, allowed []string) {
	allowedSet := make(map[string]struct{}, len(allowed))
	for _, name := range allowed {
		name = strings.TrimSpace(name)
		if name != "" {
			allowedSet[name] = struct{}{}
		}
	}
	isAllowed := func(name string) bool {
		if len(allowedSet) == 0 {
			return true
		}
		_, ok := allowedSet[name]
		return ok
	}

	if isAllowed(toolWrite) {
		s.AddTool(mcpproto.NewTool(toolWrite,
			mcpproto.WithDescription("Write a new memory into QubicDB."),
			mcpproto.WithString("index_id", mcpproto.Required(), mcpproto.Description("QubicDB index id (X-Index-ID equivalent).")),
			mcpproto.WithString("content", mcpproto.Required(), mcpproto.Description("Memory content to persist.")),
			mcpproto.WithString("metadata", mcpproto.Description("Optional JSON object of string key-value metadata (e.g. {\"thread_id\":\"conv-1\",\"role\":\"user\"}).")),
		), func(ctx context.Context, req mcpproto.CallToolRequest) (*mcpproto.CallToolResult, error) {
			args := req.GetArguments()
			indexID := getString(args, "index_id", "")
			content := getString(args, "content", "")
			if indexID == "" {
				return errResult("index_id is required"), nil
			}
			if strings.TrimSpace(content) == "" {
				return errResult("content is required"), nil
			}
			var metadata map[string]string
			if raw := getString(args, "metadata", ""); raw != "" {
				if err := json.Unmarshal([]byte(raw), &metadata); err != nil {
					return errResult("metadata must be a valid JSON object of string values"), nil
				}
			}
			result, err := backend.Write(ctx, indexID, content, metadata)
			if err != nil {
				return errResult(err.Error()), nil
			}
			return structuredResult("memory written", result)
		})
	}

	if isAllowed(toolRead) {
		s.AddTool(mcpproto.NewTool(toolRead,
			mcpproto.WithDescription("Read one memory by neuron id from QubicDB."),
			mcpproto.WithString("index_id", mcpproto.Required(), mcpproto.Description("QubicDB index id.")),
			mcpproto.WithString("id", mcpproto.Required(), mcpproto.Description("Neuron id.")),
		), func(ctx context.Context, req mcpproto.CallToolRequest) (*mcpproto.CallToolResult, error) {
			args := req.GetArguments()
			indexID := getString(args, "index_id", "")
			id := getString(args, "id", "")
			if indexID == "" || id == "" {
				return errResult("index_id and id are required"), nil
			}
			result, err := backend.Read(ctx, indexID, id)
			if err != nil {
				return errResult(err.Error()), nil
			}
			return structuredResult("memory fetched", result)
		})
	}

	if isAllowed(toolSearch) {
		s.AddTool(mcpproto.NewTool(toolSearch,
			mcpproto.WithDescription("Search memories in QubicDB with optional metadata boost or strict filter."),
			mcpproto.WithString("index_id", mcpproto.Required(), mcpproto.Description("QubicDB index id.")),
			mcpproto.WithString("query", mcpproto.Required(), mcpproto.Description("Search query.")),
			mcpproto.WithNumber("depth", mcpproto.Description("Search depth (optional, default 2).")),
			mcpproto.WithNumber("limit", mcpproto.Description("Result limit (optional, default 20).")),
			mcpproto.WithString("metadata", mcpproto.Description("Optional JSON object of string key-value metadata to filter/boost (e.g. {\"thread_id\":\"conv-1\"}).")),
			mcpproto.WithBoolean("strict", mcpproto.Description("If true, only return neurons matching ALL metadata keys. Default false (soft boost).")),
		), func(ctx context.Context, req mcpproto.CallToolRequest) (*mcpproto.CallToolResult, error) {
			args := req.GetArguments()
			indexID := getString(args, "index_id", "")
			query := getString(args, "query", "")
			if indexID == "" || strings.TrimSpace(query) == "" {
				return errResult("index_id and query are required"), nil
			}
			depth := getInt(args, "depth", 2)
			limit := getInt(args, "limit", 20)
			var metadata map[string]string
			if raw := getString(args, "metadata", ""); raw != "" {
				if err := json.Unmarshal([]byte(raw), &metadata); err != nil {
					return errResult("metadata must be a valid JSON object of string values"), nil
				}
			}
			strict := getBool(args, "strict", false)
			result, err := backend.Search(ctx, indexID, query, depth, limit, metadata, strict)
			if err != nil {
				return errResult(err.Error()), nil
			}
			return structuredResult("search completed", result)
		})
	}

	if isAllowed(toolRecall) {
		s.AddTool(mcpproto.NewTool(toolRecall,
			mcpproto.WithDescription("Recall recent memories from a QubicDB index."),
			mcpproto.WithString("index_id", mcpproto.Required(), mcpproto.Description("QubicDB index id.")),
			mcpproto.WithNumber("limit", mcpproto.Description("Max memories to return (optional).")),
		), func(ctx context.Context, req mcpproto.CallToolRequest) (*mcpproto.CallToolResult, error) {
			args := req.GetArguments()
			indexID := getString(args, "index_id", "")
			if indexID == "" {
				return errResult("index_id is required"), nil
			}
			limit := getInt(args, "limit", 100)
			result, err := backend.Recall(ctx, indexID, limit)
			if err != nil {
				return errResult(err.Error()), nil
			}
			return structuredResult("recall completed", result)
		})
	}

	if isAllowed(toolContext) {
		s.AddTool(mcpproto.NewTool(toolContext,
			mcpproto.WithDescription("Assemble LLM context from QubicDB memory."),
			mcpproto.WithString("index_id", mcpproto.Required(), mcpproto.Description("QubicDB index id.")),
			mcpproto.WithString("cue", mcpproto.Required(), mcpproto.Description("Current user cue/query.")),
			mcpproto.WithNumber("depth", mcpproto.Description("Search depth used during context assembly (optional).")),
			mcpproto.WithNumber("max_tokens", mcpproto.Description("Token budget for assembled context (optional).")),
		), func(ctx context.Context, req mcpproto.CallToolRequest) (*mcpproto.CallToolResult, error) {
			args := req.GetArguments()
			indexID := getString(args, "index_id", "")
			cue := getString(args, "cue", "")
			if indexID == "" || strings.TrimSpace(cue) == "" {
				return errResult("index_id and cue are required"), nil
			}
			depth := getInt(args, "depth", 2)
			maxTokens := getInt(args, "max_tokens", 2000)
			result, err := backend.Context(ctx, indexID, cue, depth, maxTokens)
			if err != nil {
				return errResult(err.Error()), nil
			}
			return structuredResult("context assembled", result)
		})
	}

	if isAllowed(toolRegistryFindCreate) {
		s.AddTool(mcpproto.NewTool(toolRegistryFindCreate,
			mcpproto.WithDescription("Find or create a UUID registry entry for client access."),
			mcpproto.WithString("uuid", mcpproto.Required(), mcpproto.Description("UUID to find or create.")),
		), func(ctx context.Context, req mcpproto.CallToolRequest) (*mcpproto.CallToolResult, error) {
			args := req.GetArguments()
			uuid := getString(args, "uuid", "")
			if uuid == "" {
				return errResult("uuid is required"), nil
			}
			result, err := backend.RegistryFindOrCreate(ctx, uuid, nil)
			if err != nil {
				return errResult(err.Error()), nil
			}
			return structuredResult("registry entry resolved", result)
		})
	}
}

func registerPrompts(s *mcpserver.MCPServer) {
	s.AddPrompt(mcpproto.NewPrompt("qubicdb_memory_recall",
		mcpproto.WithPromptDescription("Generate a memory recall workflow for a user cue."),
		mcpproto.WithArgument("index_id", mcpproto.RequiredArgument(), mcpproto.ArgumentDescription("QubicDB index id.")),
		mcpproto.WithArgument("cue", mcpproto.RequiredArgument(), mcpproto.ArgumentDescription("The current question or user cue.")),
	), func(_ context.Context, req mcpproto.GetPromptRequest) (*mcpproto.GetPromptResult, error) {
		indexID := req.Params.Arguments["index_id"]
		cue := req.Params.Arguments["cue"]
		return &mcpproto.GetPromptResult{
			Description: "QubicDB memory recall workflow",
			Messages: []mcpproto.PromptMessage{
				{
					Role: mcpproto.RoleUser,
					Content: mcpproto.TextContent{
						Type: "text",
						Text: fmt.Sprintf("For index %q, gather context for cue %q by calling qubicdb_search and qubicdb_context, then summarize relevant memories and cite ids.", indexID, cue),
					},
				},
			},
		}, nil
	})
}

func textResult(text string) *mcpproto.CallToolResult {
	return &mcpproto.CallToolResult{
		Content: []mcpproto.Content{
			mcpproto.TextContent{Type: "text", Text: text},
		},
	}
}

func errResult(msg string) *mcpproto.CallToolResult {
	return &mcpproto.CallToolResult{
		Content: []mcpproto.Content{
			mcpproto.TextContent{Type: "text", Text: "Error: " + msg},
		},
		IsError: true,
	}
}

func structuredResult(summary string, data any) (*mcpproto.CallToolResult, error) {
	blob, err := json.Marshal(data)
	if err != nil {
		return errResult(fmt.Sprintf("failed to marshal result: %v", err)), nil
	}
	return &mcpproto.CallToolResult{
		Content: []mcpproto.Content{
			mcpproto.TextContent{Type: "text", Text: summary},
			mcpproto.TextContent{Type: "text", Text: string(blob)},
		},
	}, nil
}

func getString(args map[string]any, key string, def string) string {
	if args == nil {
		return def
	}
	if v, ok := args[key].(string); ok {
		return v
	}
	return def
}

func getInt(args map[string]any, key string, def int) int {
	if args == nil {
		return def
	}
	v, ok := args[key].(float64)
	if !ok {
		return def
	}
	if math.IsNaN(v) || math.IsInf(v, 0) {
		return def
	}
	return int(v)
}

func getBool(args map[string]any, key string, def bool) bool {
	if args == nil {
		return def
	}
	if v, ok := args[key].(bool); ok {
		return v
	}
	return def
}

func apiKeyMiddleware(expected string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}

		provided := strings.TrimSpace(r.Header.Get("X-API-Key"))
		if provided == "" {
			auth := strings.TrimSpace(r.Header.Get("Authorization"))
			if strings.HasPrefix(strings.ToLower(auth), "bearer ") {
				provided = strings.TrimSpace(auth[7:])
			}
		}

		if provided == "" || provided != expected {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte("unauthorized"))
			return
		}
		next.ServeHTTP(w, r)
	})
}

type rateLimitEntry struct {
	tokens float64
	last   time.Time
}

type rateLimiter struct {
	rps   float64
	burst float64

	mu      sync.Mutex
	clients map[string]rateLimitEntry
}

func newRateLimiter(rps float64, burst int) *rateLimiter {
	return &rateLimiter{
		rps:     rps,
		burst:   float64(burst),
		clients: make(map[string]rateLimitEntry),
	}
}

func (rl *rateLimiter) allow(key string) bool {
	now := time.Now()

	rl.mu.Lock()
	defer rl.mu.Unlock()

	entry, ok := rl.clients[key]
	if !ok {
		rl.clients[key] = rateLimitEntry{tokens: rl.burst - 1, last: now}
		return true
	}

	elapsed := now.Sub(entry.last).Seconds()
	entry.tokens = math.Min(rl.burst, entry.tokens+elapsed*rl.rps)
	entry.last = now
	if entry.tokens < 1 {
		rl.clients[key] = entry
		return false
	}
	entry.tokens -= 1
	rl.clients[key] = entry
	return true
}

func rateLimitMiddleware(rl *rateLimiter, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		key := clientAddr(r)
		if !rl.allow(key) {
			w.Header().Set("Retry-After", "1")
			w.WriteHeader(http.StatusTooManyRequests)
			_, _ = w.Write([]byte("rate limit exceeded"))
			return
		}
		next.ServeHTTP(w, r)
	})
}

func clientAddr(r *http.Request) string {
	if fwd := strings.TrimSpace(r.Header.Get("X-Forwarded-For")); fwd != "" {
		parts := strings.Split(fwd, ",")
		if len(parts) > 0 {
			return strings.TrimSpace(parts[0])
		}
	}
	host, _, err := net.SplitHostPort(strings.TrimSpace(r.RemoteAddr))
	if err == nil && host != "" {
		return host
	}
	if strings.TrimSpace(r.RemoteAddr) != "" {
		return strings.TrimSpace(r.RemoteAddr)
	}
	return "unknown"
}
