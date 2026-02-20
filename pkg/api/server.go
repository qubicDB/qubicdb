package api

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/qubicDB/qubicdb/pkg/api/apierr"
	"github.com/qubicDB/qubicdb/pkg/concurrency"
	"github.com/qubicDB/qubicdb/pkg/core"
	"github.com/qubicDB/qubicdb/pkg/daemon"
	"github.com/qubicDB/qubicdb/pkg/lifecycle"
	mcpapi "github.com/qubicDB/qubicdb/pkg/mcp"
	"github.com/qubicDB/qubicdb/pkg/protocol"
	"github.com/qubicDB/qubicdb/pkg/registry"
)

// Server is the HTTP/REST API server.
type Server struct {
	pool      *concurrency.WorkerPool
	lifecycle *lifecycle.Manager
	executor  *protocol.Executor
	registry  *registry.Store
	config    *core.Config
	daemons   *daemon.DaemonManager

	httpServer *http.Server
	addr       string
	mcpPath    string

	rateLimitEnabled  bool
	rateLimitRequests int
	rateLimitWindow   time.Duration
	rateLimitMu       sync.Mutex
	rateLimitEntries  map[string]rateLimitEntry
}

const (
	defaultSearchDepth      = 2
	defaultSearchLimit      = 20
	maxSearchDepth          = 8
	maxSearchLimit          = 200
	defaultContextDepth     = 2
	defaultContextTokens    = 2000
	maxContextDepth         = 8
	maxContextTokens        = 16000
	defaultRateLimitWindow  = time.Minute
	defaultRateLimitRequest = 10000
)

type rateLimitEntry struct {
	windowStart time.Time
	count       int
}

// NewServer creates a new API server
func NewServer(
	addr string,
	pool *concurrency.WorkerPool,
	lm *lifecycle.Manager,
	reg *registry.Store,
	cfg *core.Config,
) *Server {
	s := &Server{
		pool:              pool,
		lifecycle:         lm,
		executor:          protocol.NewExecutor(),
		registry:          reg,
		config:            cfg,
		addr:              addr,
		rateLimitEnabled:  true,
		rateLimitRequests: defaultRateLimitRequest,
		rateLimitWindow:   defaultRateLimitWindow,
		rateLimitEntries:  make(map[string]rateLimitEntry),
	}
	if err := core.SetMaxNeuronContentBytes(cfg.Security.MaxNeuronContentBytes); err != nil {
		log.Printf("⚠ invalid security.maxNeuronContentBytes=%d, using runtime default: %v", cfg.Security.MaxNeuronContentBytes, err)
	}

	mux := http.NewServeMux()

	// Health
	mux.HandleFunc("/health", s.handleHealth)

	// Index brain operations
	mux.HandleFunc("/v1/brain/", s.handleBrain)

	// Brain-like API endpoints (primary)
	mux.HandleFunc("/v1/write", s.handleWrite)    // Memory formation
	mux.HandleFunc("/v1/read/", s.handleRead)     // Memory retrieval
	mux.HandleFunc("/v1/search", s.handleSearch)  // Associative recall
	mux.HandleFunc("/v1/touch", s.handleTouch)    // Memory modification
	mux.HandleFunc("/v1/forget/", s.handleForget) // Memory erasure
	mux.HandleFunc("/v1/recall", s.handleRecall)  // Memory scanning
	mux.HandleFunc("/v1/fire/", s.handleFire)     // Neural firing

	// MongoDB-like command endpoint
	mux.HandleFunc("/v1/command", s.handleCommand)

	// Context assembly for LLM
	mux.HandleFunc("/v1/context", s.handleContext)

	// Stats
	mux.HandleFunc("/v1/stats", s.handleStats)

	// Synapses endpoint for graph visualization
	mux.HandleFunc("/v1/synapses", s.handleSynapses)

	// Graph data endpoint (neurons + synapses for visualization)
	mux.HandleFunc("/v1/graph", s.handleGraph)

	// Activity log endpoint
	mux.HandleFunc("/v1/activity", s.handleActivity)

	// UUID Registry
	mux.HandleFunc("/v1/registry/find-or-create", s.handleRegistryFindOrCreate)
	mux.HandleFunc("/v1/registry/", s.handleRegistry)
	mux.HandleFunc("/v1/registry", s.handleRegistry)

	if cfg.MCP.Enabled {
		path := cfg.MCP.Path
		if strings.TrimSpace(path) == "" {
			path = "/mcp"
		}
		if len(path) > 1 {
			path = strings.TrimRight(path, "/")
		}

		mcpHandler, err := mcpapi.NewHandler(mcpapi.Config{
			APIKey:         cfg.MCP.APIKey,
			Stateless:      cfg.MCP.Stateless,
			RateLimitRPS:   cfg.MCP.RateLimitRPS,
			RateLimitBurst: cfg.MCP.RateLimitBurst,
			EnablePrompts:  cfg.MCP.EnablePrompts,
			AllowedTools:   cfg.MCP.AllowedTools,
		}, newMCPBackend(s))
		if err != nil {
			log.Printf("⚠ MCP endpoint disabled: %v", err)
		} else {
			s.mcpPath = path
			mux.Handle(path, mcpHandler)
			log.Printf("MCP endpoint enabled at %s (stateless=%v)", path, cfg.MCP.Stateless)
		}
	}

	// Admin endpoints (gated by admin.enabled)
	if cfg.Admin.Enabled {
		mux.HandleFunc("/admin/login", s.handleAdminLogin)
		mux.HandleFunc("/admin/indexes", s.requireAdmin(s.handleAdminUsers))
		mux.HandleFunc("/admin/indexes/", s.requireAdmin(s.handleAdminIndexOps))
		mux.HandleFunc("/v1/config", s.requireAdmin(s.handleConfig))
		mux.HandleFunc("/admin/config", s.requireAdmin(s.handleConfig))
		mux.HandleFunc("/admin/daemons", s.requireAdmin(s.handleAdminDaemons))
		mux.HandleFunc("/admin/daemons/", s.requireAdmin(s.handleAdminDaemonOps))
		mux.HandleFunc("/admin/gc", s.requireAdmin(s.handleAdminGC))
		mux.HandleFunc("/admin/persist", s.requireAdmin(s.handleAdminPersist))
	}

	s.httpServer = &http.Server{
		Addr:         addr,
		Handler:      s.withMiddleware(mux),
		ReadTimeout:  cfg.Security.ReadTimeout,
		WriteTimeout: cfg.Security.WriteTimeout,
	}

	return s
}

// SetDaemonManager binds the daemon manager for runtime interval reconfiguration.
func (s *Server) SetDaemonManager(dm *daemon.DaemonManager) {
	s.daemons = dm
}

// withMiddleware adds common middleware (CORS, content-type, request body limit, logging).
func (s *Server) withMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if s.isMCPPath(r.URL.Path) {
			start := time.Now()
			next.ServeHTTP(w, r)
			log.Printf("%s %s %v", r.Method, r.URL.Path, time.Since(start))
			return
		}

		// CORS — driven by config
		// AllowedOrigins may be comma-separated; match against the request Origin header.
		requestOrigin := r.Header.Get("Origin")
		if requestOrigin != "" {
			allowed := false
			if s.config.Security.AllowedOrigins == "*" {
				allowed = true
			} else {
				for _, o := range strings.Split(s.config.Security.AllowedOrigins, ",") {
					if strings.TrimSpace(o) == requestOrigin {
						allowed = true
						break
					}
				}
			}
			if allowed {
				w.Header().Set("Access-Control-Allow-Origin", requestOrigin)
			}
		}
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, X-Index-ID, Authorization")

		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}

		if !s.allowRequestByRateLimit(r) {
			retryAfter := int(s.rateLimitWindow.Seconds())
			if retryAfter < 1 {
				retryAfter = 1
			}
			w.Header().Set("Retry-After", strconv.Itoa(retryAfter))
			apierr.TooManyRequests(w, "rate limit exceeded")
			return
		}

		// Request body size limit
		if s.config.Security.MaxRequestBody > 0 && r.Body != nil {
			r.Body = http.MaxBytesReader(w, r.Body, s.config.Security.MaxRequestBody)
		}

		// Content-Type
		w.Header().Set("Content-Type", "application/json")

		// Logging
		start := time.Now()
		next.ServeHTTP(w, r)
		log.Printf("%s %s %v", r.Method, r.URL.Path, time.Since(start))
	})
}

func (s *Server) isMCPPath(path string) bool {
	if s.mcpPath == "" {
		return false
	}
	if path == s.mcpPath {
		return true
	}
	return strings.HasPrefix(path, s.mcpPath+"/")
}

// requireAdmin wraps a handler with admin Basic-Auth verification.
// The client must send an Authorization header: Basic base64(user:password).
func (s *Server) requireAdmin(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user, pass, ok := r.BasicAuth()
		if !ok {
			w.Header().Set("WWW-Authenticate", `Basic realm="qubicdb admin"`)
			apierr.Unauthorized(w, "admin authentication required")
			return
		}

		// Constant-time comparison to prevent timing attacks.
		userHash := sha256.Sum256([]byte(user))
		passHash := sha256.Sum256([]byte(pass))
		expectedUserHash := sha256.Sum256([]byte(s.config.Admin.User))
		expectedPassHash := sha256.Sum256([]byte(s.config.Admin.Password))

		userMatch := subtle.ConstantTimeCompare(userHash[:], expectedUserHash[:]) == 1
		passMatch := subtle.ConstantTimeCompare(passHash[:], expectedPassHash[:]) == 1

		if !userMatch || !passMatch {
			apierr.Unauthorized(w, "invalid admin credentials")
			return
		}

		next(w, r)
	}
}

// writeOperationError maps worker operation errors to HTTP API errors.
func (s *Server) writeOperationError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, core.ErrInvalidContent):
		apierr.BadRequest(w, apierr.CodeInvalidContent, err.Error())
	case errors.Is(err, core.ErrInvalidQuery):
		apierr.BadRequest(w, apierr.CodeQueryRequired, err.Error())
	case errors.Is(err, core.ErrContentTooLarge):
		apierr.PayloadTooLarge(w, err.Error())
	case errors.Is(err, core.ErrNeuronNotFound):
		apierr.NotFound(w, apierr.CodeNeuronNotFound, err.Error())
	default:
		apierr.Internal(w, err.Error())
	}
}

func (s *Server) decodeJSONRequest(w http.ResponseWriter, r *http.Request, dst any) bool {
	if err := json.NewDecoder(r.Body).Decode(dst); err != nil {
		var maxErr *http.MaxBytesError
		if errors.As(err, &maxErr) {
			apierr.PayloadTooLarge(w, err.Error())
			return false
		}
		apierr.InvalidJSON(w)
		return false
	}
	return true
}

func clampPositive(value, fallback, maxValue int) int {
	if value <= 0 {
		value = fallback
	}
	if maxValue > 0 && value > maxValue {
		return maxValue
	}
	return value
}

func parsePositiveQueryInt(raw string) int {
	if raw == "" {
		return 0
	}
	v, err := strconv.Atoi(raw)
	if err != nil {
		return 0
	}
	return v
}

func (s *Server) allowRequestByRateLimit(r *http.Request) bool {
	if !s.rateLimitEnabled || s.rateLimitRequests <= 0 || s.rateLimitWindow <= 0 {
		return true
	}

	key := r.RemoteAddr
	if ip := strings.TrimSpace(r.Header.Get("X-Forwarded-For")); ip != "" {
		parts := strings.Split(ip, ",")
		key = strings.TrimSpace(parts[0])
	} else if ip := strings.TrimSpace(r.Header.Get("X-Real-IP")); ip != "" {
		key = ip
	} else if host, _, err := net.SplitHostPort(r.RemoteAddr); err == nil && host != "" {
		key = host
	}
	if key == "" {
		key = "unknown"
	}

	now := time.Now()
	s.rateLimitMu.Lock()
	defer s.rateLimitMu.Unlock()

	entry := s.rateLimitEntries[key]
	if entry.windowStart.IsZero() || now.Sub(entry.windowStart) >= s.rateLimitWindow {
		s.rateLimitEntries[key] = rateLimitEntry{windowStart: now, count: 1}
		return true
	}
	if entry.count >= s.rateLimitRequests {
		return false
	}
	entry.count++
	s.rateLimitEntries[key] = entry
	return true
}

// Start starts the server. Uses TLS if configured.
func (s *Server) Start() error {
	if s.config.Security.TLSCert != "" && s.config.Security.TLSKey != "" {
		log.Printf("🚀 QubicDB API server starting on %s (TLS)", s.addr)
		return s.httpServer.ListenAndServeTLS(s.config.Security.TLSCert, s.config.Security.TLSKey)
	}
	log.Printf("🚀 QubicDB API server starting on %s", s.addr)
	return s.httpServer.ListenAndServe()
}

// Stop gracefully stops the server
func (s *Server) Stop(ctx context.Context) error {
	return s.httpServer.Shutdown(ctx)
}

// getIndexID extracts index ID from request.
func (s *Server) getIndexID(r *http.Request) core.IndexID {
	// Header takes priority
	if id := r.Header.Get("X-Index-ID"); id != "" {
		return core.IndexID(id)
	}
	// Fallback to query parameter
	if id := r.URL.Query().Get("indexId"); id != "" {
		return core.IndexID(id)
	}
	if id := r.URL.Query().Get("index_id"); id != "" {
		return core.IndexID(id)
	}
	return ""
}

// getWorker gets or creates a worker for the index (requires registered UUID).
// Returns a coded error string that callers can map to an apierr response.
func (s *Server) getWorker(indexID core.IndexID) (*concurrency.BrainWorker, error) {
	if indexID == "" {
		return nil, fmt.Errorf("%s: X-Index-ID header or index_id query parameter required", apierr.CodeIndexIDRequired)
	}

	// Check UUID is registered (only when registry guard is enabled)
	if s.config.Registry.Enabled && !s.registry.Exists(string(indexID)) {
		return nil, fmt.Errorf("%s: uuid not registered: %s", apierr.CodeUUIDNotRegistered, indexID)
	}

	// Record activity
	s.lifecycle.RecordActivity(indexID)

	return s.pool.GetOrCreate(indexID)
}

// writeWorkerError maps a getWorker error to the appropriate apierr response.
func (s *Server) writeWorkerError(w http.ResponseWriter, err error) {
	msg := err.Error()
	switch {
	case strings.HasPrefix(msg, apierr.CodeIndexIDRequired):
		apierr.IndexIDRequired(w)
	case strings.HasPrefix(msg, apierr.CodeUUIDNotRegistered):
		apierr.BadRequest(w, apierr.CodeUUIDNotRegistered, msg)
	default:
		apierr.Internal(w, msg)
	}
}

// handleHealth returns health status
func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	active := s.pool.ActiveCount()
	json.NewEncoder(w).Encode(map[string]any{
		"status":        "healthy",
		"timestamp":     time.Now(),
		"activeIndexes": active,
	})
}

// handleBrain handles brain-level operations
func (s *Server) handleBrain(w http.ResponseWriter, r *http.Request) {
	indexID := s.getIndexID(r)
	if indexID == "" {
		apierr.IndexIDRequired(w)
		return
	}

	// Extract sub-path: /v1/brain/{action}
	path := strings.TrimPrefix(r.URL.Path, "/v1/brain/")

	switch {
	case path == "wake" && r.Method == "POST":
		s.lifecycle.ForceWake(indexID)
		json.NewEncoder(w).Encode(map[string]any{"status": "awake"})

	case path == "sleep" && r.Method == "POST":
		s.lifecycle.ForceSleep(indexID)
		json.NewEncoder(w).Encode(map[string]any{"status": "sleeping"})

	case path == "state" && r.Method == "GET":
		state := s.lifecycle.GetBrainState(indexID)
		if state == nil {
			json.NewEncoder(w).Encode(map[string]any{"state": "dormant"})
		} else {
			stateNames := []string{"active", "idle", "sleeping", "dormant"}
			json.NewEncoder(w).Encode(map[string]any{
				"state":       stateNames[state.State],
				"lastInvoke":  state.LastInvoke,
				"invokeCount": state.InvokeCount,
			})
		}

	case path == "stats" && r.Method == "GET":
		worker, err := s.getWorker(indexID)
		if err != nil {
			s.writeWorkerError(w, err)
			return
		}
		result, _ := worker.Submit(&concurrency.Operation{Type: concurrency.OpGetStats})
		json.NewEncoder(w).Encode(result)

	default:
		apierr.NotFound(w, apierr.CodeNotFound, "unknown brain action")
	}
}

// handleSearch handles semantic-like search
func (s *Server) handleSearch(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" && r.Method != "GET" {
		apierr.MethodNotAllowed(w)
		return
	}

	indexID := s.getIndexID(r)
	worker, err := s.getWorker(indexID)
	if err != nil {
		s.writeWorkerError(w, err)
		return
	}

	var query string
	depth, limit := defaultSearchDepth, defaultSearchLimit
	var metadata map[string]string
	var strict bool

	if r.Method == "GET" {
		query = r.URL.Query().Get("q")
		if v := parsePositiveQueryInt(r.URL.Query().Get("depth")); v > 0 {
			depth = v
		}
		if v := parsePositiveQueryInt(r.URL.Query().Get("limit")); v > 0 {
			limit = v
		}
		// Metadata filter from query params: metadata_<key>=<value>
		for k, vs := range r.URL.Query() {
			if strings.HasPrefix(k, "metadata_") && len(vs) > 0 {
				if metadata == nil {
					metadata = make(map[string]string)
				}
				metadata[strings.TrimPrefix(k, "metadata_")] = vs[0]
			}
		}
		strict = r.URL.Query().Get("strict") == "true"
	} else {
		var req struct {
			Query    string            `json:"query"`
			Depth    int               `json:"depth,omitempty"`
			Limit    int               `json:"limit,omitempty"`
			Metadata map[string]string `json:"metadata,omitempty"`
			Strict   bool              `json:"strict,omitempty"`
		}
		if !s.decodeJSONRequest(w, r, &req) {
			return
		}
		query = req.Query
		if req.Depth > 0 {
			depth = req.Depth
		}
		if req.Limit > 0 {
			limit = req.Limit
		}
		metadata = req.Metadata
		strict = req.Strict
	}

	depth = clampPositive(depth, defaultSearchDepth, maxSearchDepth)
	limit = clampPositive(limit, defaultSearchLimit, maxSearchLimit)

	if query == "" {
		apierr.QueryRequired(w)
		return
	}

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
		s.writeOperationError(w, err)
		return
	}

	neurons := result.([]*core.Neuron)
	docs := make([]map[string]any, 0, len(neurons))
	for _, n := range neurons {
		docs = append(docs, protocol.NeuronToDocument(n, nil))
	}

	json.NewEncoder(w).Encode(map[string]any{
		"results": docs,
		"count":   len(docs),
		"query":   query,
		"depth":   depth,
	})
}

// handleCommand handles MongoDB-like commands
func (s *Server) handleCommand(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		apierr.MethodNotAllowed(w)
		return
	}

	indexID := s.getIndexID(r)
	worker, err := s.getWorker(indexID)
	if err != nil {
		s.writeWorkerError(w, err)
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		var maxErr *http.MaxBytesError
		if errors.As(err, &maxErr) {
			apierr.PayloadTooLarge(w, err.Error())
			return
		}
		apierr.InvalidJSON(w)
		return
	}
	cmd, err := protocol.ParseCommand(body)
	if err != nil {
		apierr.BadRequest(w, apierr.CodeBadRequest, err.Error())
		return
	}

	result := s.executor.Execute(worker, cmd)
	if !result.Success {
		if strings.Contains(result.Error, "direct neuron mutation is disabled") {
			apierr.BadRequest(w, apierr.CodeMutationDisabled, result.Error)
			return
		}
		apierr.BadRequest(w, apierr.CodeBadRequest, result.Error)
		return
	}
	json.NewEncoder(w).Encode(result)
}

// handleContext assembles context for LLM injection
func (s *Server) handleContext(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		apierr.MethodNotAllowed(w)
		return
	}

	indexID := s.getIndexID(r)
	worker, err := s.getWorker(indexID)
	if err != nil {
		s.writeWorkerError(w, err)
		return
	}

	var req struct {
		Cue       string `json:"cue"`       // Current user message/query
		MaxTokens int    `json:"maxTokens"` // Context window budget
		Depth     int    `json:"depth"`     // Spread depth
	}
	if !s.decodeJSONRequest(w, r, &req) {
		return
	}
	if req.Cue == "" {
		apierr.QueryRequired(w)
		return
	}

	req.MaxTokens = clampPositive(req.MaxTokens, defaultContextTokens, maxContextTokens)
	req.Depth = clampPositive(req.Depth, defaultContextDepth, maxContextDepth)

	// Search based on cue
	result, err := worker.Submit(&concurrency.Operation{
		Type: concurrency.OpSearch,
		Payload: concurrency.SearchRequest{
			Query: req.Cue,
			Depth: req.Depth,
			Limit: 50, // Get more, then trim by tokens
		},
	})
	if err != nil {
		s.writeOperationError(w, err)
		return
	}

	neurons := result.([]*core.Neuron)

	// Assemble context string
	var context strings.Builder
	tokenEstimate := 0
	included := 0

	for _, n := range neurons {
		// Approximate token count (~4 characters per token)
		neuronTokens := len(n.Content) / 4
		if tokenEstimate+neuronTokens > req.MaxTokens {
			break
		}

		if context.Len() > 0 {
			context.WriteString("\n---\n")
		}

		context.WriteString(n.Content)

		// Add depth indicator
		if n.Depth > 0 {
			context.WriteString(fmt.Sprintf(" [depth:%d]", n.Depth))
		}

		tokenEstimate += neuronTokens
		included++
	}

	json.NewEncoder(w).Encode(map[string]any{
		"context":         context.String(),
		"text":            context.String(),
		"neuronsUsed":     included,
		"neuronCount":     included,
		"estimatedTokens": tokenEstimate,
		"tokenCount":      tokenEstimate,
		"cue":             req.Cue,
	})
}

// handleStats returns global statistics
func (s *Server) handleStats(w http.ResponseWriter, r *http.Request) {
	json.NewEncoder(w).Encode(map[string]any{
		"pool":      s.pool.Stats(),
		"lifecycle": s.lifecycle.Stats(),
	})
}

// ============================================================
// Admin Handlers
// ============================================================

// handleAdminLogin handles admin authentication
func (s *Server) handleAdminLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		apierr.MethodNotAllowed(w)
		return
	}

	var req struct {
		User     string `json:"user"`
		Password string `json:"password"`
	}
	if !s.decodeJSONRequest(w, r, &req) {
		return
	}

	// Verify credentials from config using constant-time comparison.
	userHash := sha256.Sum256([]byte(req.User))
	passHash := sha256.Sum256([]byte(req.Password))
	expectedUserHash := sha256.Sum256([]byte(s.config.Admin.User))
	expectedPassHash := sha256.Sum256([]byte(s.config.Admin.Password))

	userOK := subtle.ConstantTimeCompare(userHash[:], expectedUserHash[:]) == 1
	passOK := subtle.ConstantTimeCompare(passHash[:], expectedPassHash[:]) == 1

	if userOK && passOK {
		json.NewEncoder(w).Encode(map[string]any{
			"success": true,
			"message": "authenticated — use Basic Auth for subsequent admin requests",
		})
	} else {
		apierr.Unauthorized(w, "invalid credentials")
	}
}

// handleAdminUsers lists all active indexes
func (s *Server) handleAdminUsers(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		apierr.MethodNotAllowed(w)
		return
	}

	indexes := s.pool.ListIndexes()
	json.NewEncoder(w).Encode(indexes)
}

// handleAdminIndexOps handles per-index admin operations.
func (s *Server) handleAdminIndexOps(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/admin/indexes/")
	parts := strings.Split(path, "/")
	if len(parts) < 1 || parts[0] == "" {
		apierr.IndexIDRequired(w)
		return
	}

	indexID := core.IndexID(parts[0])
	action := ""
	if len(parts) > 1 {
		action = parts[1]
	}

	switch {
	case action == "reset" && r.Method == "POST":
		if err := s.pool.Truncate(indexID); err != nil {
			apierr.Internal(w, err.Error())
			return
		}
		s.lifecycle.RemoveIndex(indexID)
		json.NewEncoder(w).Encode(map[string]any{"reset": true, "truncated": true, "indexId": indexID})

	case action == "wake" && r.Method == "POST":
		s.lifecycle.ForceWake(indexID)
		json.NewEncoder(w).Encode(map[string]any{"woke": true, "indexId": indexID})

	case action == "sleep" && r.Method == "POST":
		s.lifecycle.ForceSleep(indexID)
		json.NewEncoder(w).Encode(map[string]any{"slept": true, "indexId": indexID})

	case action == "export" && r.Method == "GET":
		// Export index brain
		worker, err := s.pool.Get(indexID)
		if err != nil {
			apierr.NotFound(w, apierr.CodeNotFound, "index not found")
			return
		}
		result, _ := worker.Submit(&concurrency.Operation{Type: concurrency.OpGetStats})
		json.NewEncoder(w).Encode(result)

	case action == "" && r.Method == "DELETE":
		if err := s.pool.Truncate(indexID); err != nil {
			apierr.Internal(w, err.Error())
			return
		}
		s.lifecycle.RemoveIndex(indexID)
		registryDeleted := false
		if s.registry.Exists(string(indexID)) {
			if err := s.registry.Delete(string(indexID)); err != nil {
				apierr.Internal(w, err.Error())
				return
			}
			registryDeleted = true
		}
		json.NewEncoder(w).Encode(map[string]any{
			"deleted":         true,
			"truncated":       true,
			"registryDeleted": registryDeleted,
			"indexId":         indexID,
		})

	case action == "" && r.Method == "GET":
		// Get index details
		worker, err := s.pool.Get(indexID)
		if err != nil {
			apierr.NotFound(w, apierr.CodeNotFound, "index not found")
			return
		}
		result, _ := worker.Submit(&concurrency.Operation{Type: concurrency.OpGetStats})
		state := s.lifecycle.GetBrainState(indexID)
		json.NewEncoder(w).Encode(map[string]any{
			"stats": result,
			"state": state,
		})

	default:
		apierr.NotFound(w, apierr.CodeNotFound, "unknown operation")
	}
}

// handleAdminDaemons returns daemon status
func (s *Server) handleAdminDaemons(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		apierr.MethodNotAllowed(w)
		return
	}

	json.NewEncoder(w).Encode(map[string]any{
		"status": "running",
		"daemons": map[string]string{
			"decay":       "running",
			"consolidate": "running",
			"prune":       "running",
			"reorg":       "running",
		},
	})
}

// handleAdminDaemonOps handles daemon control operations
func (s *Server) handleAdminDaemonOps(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		apierr.MethodNotAllowed(w)
		return
	}

	action := strings.TrimPrefix(r.URL.Path, "/admin/daemons/")

	switch action {
	case "pause":
		json.NewEncoder(w).Encode(map[string]any{"paused": true})
	case "resume":
		json.NewEncoder(w).Encode(map[string]any{"resumed": true})
	default:
		apierr.NotFound(w, apierr.CodeNotFound, "unknown daemon action")
	}
}

// handleSynapses returns all synapses for an index
func (s *Server) handleSynapses(w http.ResponseWriter, r *http.Request) {
	worker, err := s.getWorker(s.getIndexID(r))
	if err != nil {
		s.writeWorkerError(w, err)
		return
	}

	matrix := worker.Matrix()
	matrix.RLock()
	defer matrix.RUnlock()

	type SynapseInfo struct {
		ID          string  `json:"id"`
		FromID      string  `json:"from_id"`
		ToID        string  `json:"to_id"`
		Weight      float64 `json:"weight"`
		CoFireCount uint64  `json:"co_fire_count"`
	}

	synapses := make([]SynapseInfo, 0, len(matrix.Synapses))
	for _, syn := range matrix.Synapses {
		synapses = append(synapses, SynapseInfo{
			ID:          string(syn.ID),
			FromID:      string(syn.FromID),
			ToID:        string(syn.ToID),
			Weight:      syn.Weight,
			CoFireCount: syn.CoFireCount,
		})
	}

	json.NewEncoder(w).Encode(map[string]any{
		"synapses": synapses,
		"count":    len(synapses),
	})
}

// handleGraph returns graph data (nodes + edges) for visualization
func (s *Server) handleGraph(w http.ResponseWriter, r *http.Request) {
	worker, err := s.getWorker(s.getIndexID(r))
	if err != nil {
		s.writeWorkerError(w, err)
		return
	}

	matrix := worker.Matrix()
	matrix.RLock()
	defer matrix.RUnlock()

	type Node struct {
		ID          string    `json:"id"`
		Content     string    `json:"content"`
		Energy      float64   `json:"energy"`
		Depth       int       `json:"depth"`
		AccessCount int       `json:"accessCount"`
		Position    []float64 `json:"position"`
	}

	type Edge struct {
		Source      string  `json:"source"`
		Target      string  `json:"target"`
		Weight      float64 `json:"weight"`
		CoFireCount uint64  `json:"coFireCount"`
	}

	nodes := make([]Node, 0, len(matrix.Neurons))
	for _, n := range matrix.Neurons {
		nodes = append(nodes, Node{
			ID:          string(n.ID),
			Content:     n.Content,
			Energy:      n.Energy,
			Depth:       n.Depth,
			AccessCount: int(n.AccessCount),
			Position:    n.Position,
		})
	}

	edges := make([]Edge, 0, len(matrix.Synapses))
	for _, syn := range matrix.Synapses {
		edges = append(edges, Edge{
			Source:      string(syn.FromID),
			Target:      string(syn.ToID),
			Weight:      syn.Weight,
			CoFireCount: syn.CoFireCount,
		})
	}

	json.NewEncoder(w).Encode(map[string]any{
		"nodes": nodes,
		"edges": edges,
	})
}

// handleActivity returns recent brain activity for an index
func (s *Server) handleActivity(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	worker, err := s.getWorker(s.getIndexID(r))
	if err != nil {
		s.writeWorkerError(w, err)
		return
	}

	matrix := worker.Matrix()
	matrix.RLock()
	defer matrix.RUnlock()

	// Generate activity events from recent operations
	type Event struct {
		Timestamp string `json:"timestamp"`
		Type      string `json:"type"`
		Action    string `json:"action"`
		Details   string `json:"details"`
	}

	events := []Event{}
	now := time.Now()

	// Add neuron activity
	for _, n := range matrix.Neurons {
		if now.Sub(n.LastFiredAt) < 5*time.Minute {
			events = append(events, Event{
				Timestamp: n.LastFiredAt.Format(time.RFC3339),
				Type:      "neuron",
				Action:    "FIRED",
				Details:   truncate(n.Content, 50),
			})
		}
		if now.Sub(n.CreatedAt) < 5*time.Minute {
			events = append(events, Event{
				Timestamp: n.CreatedAt.Format(time.RFC3339),
				Type:      "neuron",
				Action:    "CREATED",
				Details:   truncate(n.Content, 50),
			})
		}
	}

	// Add synapse activity
	for _, syn := range matrix.Synapses {
		if now.Sub(syn.LastCoFire) < 5*time.Minute {
			events = append(events, Event{
				Timestamp: syn.LastCoFire.Format(time.RFC3339),
				Type:      "synapse",
				Action:    fmt.Sprintf("STRENGTHENED (%.2f)", syn.Weight),
				Details:   fmt.Sprintf("co-fired %d times", syn.CoFireCount),
			})
		}
		if now.Sub(syn.CreatedAt) < 5*time.Minute {
			events = append(events, Event{
				Timestamp: syn.CreatedAt.Format(time.RFC3339),
				Type:      "synapse",
				Action:    "FORMED",
				Details:   fmt.Sprintf("weight: %.2f", syn.Weight),
			})
		}
	}

	// Sort by timestamp descending
	sort.Slice(events, func(i, j int) bool {
		return events[i].Timestamp > events[j].Timestamp
	})

	// Limit to 100 most recent
	if len(events) > 100 {
		events = events[:100]
	}

	// Reverse to show oldest first (terminal style)
	for i, j := 0, len(events)-1; i < j; i, j = i+1, j-1 {
		events[i], events[j] = events[j], events[i]
	}

	json.NewEncoder(w).Encode(map[string]any{
		"events": events,
		"count":  len(events),
	})
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

// ============================================================================
// BRAIN-LIKE API ENDPOINTS
// ============================================================================

// handleWrite - Memory formation (POST /v1/write)
func (s *Server) handleWrite(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		apierr.MethodNotAllowed(w)
		return
	}

	w.Header().Set("Content-Type", "application/json")

	worker, err := s.getWorker(s.getIndexID(r))
	if err != nil {
		s.writeWorkerError(w, err)
		return
	}

	var req struct {
		Content  string            `json:"content"`
		ParentID string            `json:"parent_id,omitempty"`
		Metadata map[string]string `json:"metadata,omitempty"`
		Tags     []string          `json:"tags,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		var maxErr *http.MaxBytesError
		if errors.As(err, &maxErr) {
			apierr.PayloadTooLarge(w, err.Error())
			return
		}
		apierr.InvalidJSON(w)
		return
	}

	var parentID *core.NeuronID
	if req.ParentID != "" {
		pid := core.NeuronID(req.ParentID)
		parentID = &pid
	}

	result, err := worker.Submit(&concurrency.Operation{
		Type: concurrency.OpWrite,
		Payload: concurrency.AddNeuronRequest{
			Content:  req.Content,
			ParentID: parentID,
			Metadata: req.Metadata,
		},
	})

	if err != nil {
		s.writeOperationError(w, err)
		return
	}

	n := result.(*core.Neuron)
	doc := protocol.NeuronToDocument(n, nil)
	doc["id"] = doc["_id"]
	json.NewEncoder(w).Encode(doc)
}

// handleRead - Memory retrieval (GET /v1/read/{id})
func (s *Server) handleRead(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		apierr.MethodNotAllowed(w)
		return
	}

	w.Header().Set("Content-Type", "application/json")

	worker, err := s.getWorker(s.getIndexID(r))
	if err != nil {
		s.writeWorkerError(w, err)
		return
	}

	// Extract neuron ID from path
	id := strings.TrimPrefix(r.URL.Path, "/v1/read/")
	if id == "" {
		apierr.NeuronIDRequired(w)
		return
	}

	result, err := worker.Submit(&concurrency.Operation{
		Type:    concurrency.OpRead,
		Payload: core.NeuronID(id),
	})

	if err != nil {
		apierr.NotFound(w, apierr.CodeNeuronNotFound, "neuron not found")
		return
	}

	n := result.(*core.Neuron)
	json.NewEncoder(w).Encode(protocol.NeuronToDocument(n, nil))
}

// handleTouch - Memory modification (PUT /v1/touch)
func (s *Server) handleTouch(w http.ResponseWriter, r *http.Request) {
	if r.Method != "PUT" && r.Method != "POST" {
		apierr.MethodNotAllowed(w)
		return
	}
	apierr.BadRequest(w, apierr.CodeMutationDisabled, "direct neuron mutation is disabled; use high-level index operations")
}

// handleForget - Memory erasure (DELETE /v1/forget/{id})
func (s *Server) handleForget(w http.ResponseWriter, r *http.Request) {
	if r.Method != "DELETE" {
		apierr.MethodNotAllowed(w)
		return
	}
	apierr.BadRequest(w, apierr.CodeMutationDisabled, "direct neuron mutation is disabled; use high-level index operations")
}

// handleRecall - Memory scanning (GET /v1/recall)
func (s *Server) handleRecall(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		apierr.MethodNotAllowed(w)
		return
	}

	w.Header().Set("Content-Type", "application/json")

	worker, err := s.getWorker(s.getIndexID(r))
	if err != nil {
		s.writeWorkerError(w, err)
		return
	}

	result, err := worker.Submit(&concurrency.Operation{
		Type: concurrency.OpRecall,
		Payload: concurrency.ListNeuronsRequest{
			Offset: 0,
			Limit:  100,
		},
	})

	if err != nil {
		apierr.Internal(w, err.Error())
		return
	}

	neurons := result.([]*core.Neuron)
	items := make([]map[string]any, len(neurons))
	for i, n := range neurons {
		items[i] = protocol.NeuronToDocument(n, nil)
	}

	json.NewEncoder(w).Encode(map[string]any{
		"memories": items,
		"neurons":  items,
		"count":    len(items),
	})
}

// handleFire - Neural firing (POST /v1/fire/{id})
func (s *Server) handleFire(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		apierr.MethodNotAllowed(w)
		return
	}
	apierr.BadRequest(w, apierr.CodeMutationDisabled, "direct neuron mutation is disabled; use high-level index operations")
}

// ============================================================================
// UUID REGISTRY ENDPOINTS
// ============================================================================

// handleRegistry routes /v1/registry and /v1/registry/{uuid}
func (s *Server) handleRegistry(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	// Extract UUID from path (empty for collection-level operations)
	path := strings.TrimSuffix(r.URL.Path, "/")
	uuid := strings.TrimPrefix(path, "/v1/registry")
	uuid = strings.TrimPrefix(uuid, "/")

	switch r.Method {
	case "POST":
		// POST /v1/registry — create new entry
		if uuid != "" {
			apierr.BadRequest(w, apierr.CodeBadRequest, "POST only on /v1/registry")
			return
		}
		s.handleRegistryCreate(w, r)

	case "GET":
		if uuid == "" {
			// GET /v1/registry — list all
			s.handleRegistryList(w, r)
		} else {
			// GET /v1/registry/{uuid} — get one
			s.handleRegistryGet(w, r, uuid)
		}

	case "PUT":
		if uuid == "" {
			apierr.UUIDRequired(w)
			return
		}
		// PUT /v1/registry/{uuid} — update
		s.handleRegistryUpdate(w, r, uuid)

	case "DELETE":
		if uuid == "" {
			apierr.UUIDRequired(w)
			return
		}
		// DELETE /v1/registry/{uuid} — delete
		s.handleRegistryDelete(w, r, uuid)

	default:
		apierr.MethodNotAllowed(w)
	}
}

// handleRegistryCreate — POST /v1/registry
func (s *Server) handleRegistryCreate(w http.ResponseWriter, r *http.Request) {
	var req struct {
		UUID     string         `json:"uuid"`
		Metadata map[string]any `json:"metadata,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		apierr.InvalidJSON(w)
		return
	}
	if req.UUID == "" {
		apierr.UUIDRequired(w)
		return
	}

	entry, err := s.registry.Create(req.UUID, req.Metadata)
	if err != nil {
		apierr.Conflict(w, apierr.CodeUUIDConflict, err.Error())
		return
	}

	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(entry)
}

// handleRegistryList — GET /v1/registry
func (s *Server) handleRegistryList(w http.ResponseWriter, r *http.Request) {
	entries := s.registry.List()
	json.NewEncoder(w).Encode(map[string]any{
		"entries": entries,
		"count":   len(entries),
	})
}

// handleRegistryGet — GET /v1/registry/{uuid}
func (s *Server) handleRegistryGet(w http.ResponseWriter, r *http.Request, uuid string) {
	entry, ok := s.registry.Get(uuid)
	if !ok {
		apierr.NotFound(w, apierr.CodeUUIDNotFound, "uuid not found")
		return
	}
	json.NewEncoder(w).Encode(entry)
}

// handleRegistryUpdate — PUT /v1/registry/{uuid}
func (s *Server) handleRegistryUpdate(w http.ResponseWriter, r *http.Request, oldUUID string) {
	var req struct {
		UUID     string         `json:"uuid"`
		Metadata map[string]any `json:"metadata,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		apierr.InvalidJSON(w)
		return
	}

	// If no new UUID provided, keep the old one
	newUUID := req.UUID
	if newUUID == "" {
		newUUID = oldUUID
	}

	entry, err := s.registry.Update(oldUUID, newUUID, req.Metadata)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			apierr.NotFound(w, apierr.CodeUUIDNotFound, err.Error())
		} else {
			apierr.Conflict(w, apierr.CodeUUIDConflict, err.Error())
		}
		return
	}

	json.NewEncoder(w).Encode(entry)
}

// handleRegistryDelete — DELETE /v1/registry/{uuid}
func (s *Server) handleRegistryDelete(w http.ResponseWriter, r *http.Request, uuid string) {
	if err := s.registry.Delete(uuid); err != nil {
		apierr.NotFound(w, apierr.CodeUUIDNotFound, err.Error())
		return
	}
	json.NewEncoder(w).Encode(map[string]any{"deleted": true, "uuid": uuid})
}

// handleRegistryFindOrCreate — POST /v1/registry/find-or-create
func (s *Server) handleRegistryFindOrCreate(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	if r.Method != "POST" {
		apierr.MethodNotAllowed(w)
		return
	}

	var req struct {
		UUID     string         `json:"uuid"`
		Metadata map[string]any `json:"metadata,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		apierr.InvalidJSON(w)
		return
	}
	if req.UUID == "" {
		apierr.UUIDRequired(w)
		return
	}

	entry, created, err := s.registry.FindOrCreate(req.UUID, req.Metadata)
	if err != nil {
		apierr.Internal(w, err.Error())
		return
	}

	status := http.StatusOK
	if created {
		status = http.StatusCreated
	}

	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]any{
		"uuid":      entry.UUID,
		"metadata":  entry.Metadata,
		"created":   created,
		"createdAt": entry.CreatedAt,
		"updatedAt": entry.UpdatedAt,
	})
}

// ============================================================================
// ADMIN ENDPOINTS
// ============================================================================

// handleAdminGC forces garbage collection
func (s *Server) handleAdminGC(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		apierr.MethodNotAllowed(w)
		return
	}

	// Placeholder — wire to runtime.GC() or pool-level cleanup as needed.
	json.NewEncoder(w).Encode(map[string]any{"gc": "triggered"})
}

// handleAdminPersist forces persistence of all brains
func (s *Server) handleAdminPersist(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		apierr.MethodNotAllowed(w)
		return
	}

	s.pool.PersistAll()
	json.NewEncoder(w).Encode(map[string]any{"persisted": true})
}

// ============================================================================
// RUNTIME CONFIGURATION ENDPOINT
// ============================================================================

// handleConfig exposes the active server configuration.
//
//   - GET  /v1/config — returns the full running configuration as JSON.
//   - POST /v1/config — applies a partial configuration patch at runtime.
//     Only daemon intervals, lifecycle thresholds, and worker settings
//     can be changed at runtime; server addresses and storage paths are
//     startup-only and will be rejected.
func (s *Server) handleConfig(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case "GET":
		s.handleConfigGet(w, r)
	case "POST":
		s.handleConfigSet(w, r)
	default:
		apierr.MethodNotAllowed(w)
	}
}

// handleConfigGet returns the active configuration snapshot.
func (s *Server) handleConfigGet(w http.ResponseWriter, _ *http.Request) {
	json.NewEncoder(w).Encode(map[string]any{
		"server": map[string]any{
			"httpAddr": s.config.Server.HTTPAddr,
		},
		"storage": map[string]any{
			"dataPath": s.config.Storage.DataPath,
			"compress": s.config.Storage.Compress,
		},
		"matrix": map[string]any{
			"minDimension": s.config.Matrix.MinDimension,
			"maxDimension": s.config.Matrix.MaxDimension,
			"maxNeurons":   s.config.Matrix.MaxNeurons,
		},
		"lifecycle": map[string]any{
			"idleThreshold":    s.config.Lifecycle.IdleThreshold.String(),
			"sleepThreshold":   s.config.Lifecycle.SleepThreshold.String(),
			"dormantThreshold": s.config.Lifecycle.DormantThreshold.String(),
		},
		"daemons": map[string]any{
			"decayInterval":       s.config.Daemons.DecayInterval.String(),
			"consolidateInterval": s.config.Daemons.ConsolidateInterval.String(),
			"pruneInterval":       s.config.Daemons.PruneInterval.String(),
			"persistInterval":     s.config.Daemons.PersistInterval.String(),
			"reorgInterval":       s.config.Daemons.ReorgInterval.String(),
		},
		"worker": map[string]any{
			"maxIdleTime": s.config.Worker.MaxIdleTime.String(),
		},
		"registry": map[string]any{
			"enabled": s.config.Registry.Enabled,
		},
		"vector": map[string]any{
			"enabled":   s.config.Vector.Enabled,
			"modelPath": s.config.Vector.ModelPath,
			"gpuLayers": s.config.Vector.GPULayers,
			"alpha":     s.config.Vector.Alpha,
		},
		"admin": map[string]any{
			"enabled": s.config.Admin.Enabled,
			"user":    s.config.Admin.User,
		},
		"security": map[string]any{
			"allowedOrigins": s.config.Security.AllowedOrigins,
			"maxRequestBody": s.config.Security.MaxRequestBody,
			"tlsEnabled":     s.config.Security.TLSCert != "",
			"readTimeout":    s.config.Security.ReadTimeout.String(),
			"writeTimeout":   s.config.Security.WriteTimeout.String(),
		},
	})
}

// handleConfigSet applies a partial runtime configuration patch.
// Only fields that are safe to change at runtime are accepted.
func (s *Server) handleConfigSet(w http.ResponseWriter, r *http.Request) {
	var patch struct {
		Lifecycle *struct {
			IdleThreshold    string `json:"idleThreshold,omitempty"`
			SleepThreshold   string `json:"sleepThreshold,omitempty"`
			DormantThreshold string `json:"dormantThreshold,omitempty"`
		} `json:"lifecycle,omitempty"`
		Daemons *struct {
			DecayInterval       string `json:"decayInterval,omitempty"`
			ConsolidateInterval string `json:"consolidateInterval,omitempty"`
			PruneInterval       string `json:"pruneInterval,omitempty"`
			PersistInterval     string `json:"persistInterval,omitempty"`
			ReorgInterval       string `json:"reorgInterval,omitempty"`
		} `json:"daemons,omitempty"`
		Worker *struct {
			MaxIdleTime string `json:"maxIdleTime,omitempty"`
		} `json:"worker,omitempty"`
		Registry *struct {
			Enabled *bool `json:"enabled,omitempty"`
		} `json:"registry,omitempty"`
		Matrix *struct {
			MaxNeurons *int `json:"maxNeurons,omitempty"`
		} `json:"matrix,omitempty"`
		Security *struct {
			AllowedOrigins *string `json:"allowedOrigins,omitempty"`
			MaxRequestBody *int64  `json:"maxRequestBody,omitempty"`
		} `json:"security,omitempty"`
		Vector *struct {
			Alpha *float64 `json:"alpha,omitempty"`
		} `json:"vector,omitempty"`
	}

	if err := json.NewDecoder(r.Body).Decode(&patch); err != nil {
		apierr.InvalidJSON(w)
		return
	}

	changed := []string{}
	rejected := []string{}

	// tryDuration parses a Go duration string, applies it to target on
	// success, and records the field name in the appropriate list.
	tryDuration := func(field, raw string, target *time.Duration) {
		d, err := time.ParseDuration(raw)
		if err != nil {
			rejected = append(rejected, field+": invalid duration "+raw)
			return
		}
		*target = d
		changed = append(changed, field)
	}

	// Apply lifecycle patches
	if patch.Lifecycle != nil {
		if v := patch.Lifecycle.IdleThreshold; v != "" {
			tryDuration("lifecycle.idleThreshold", v, &s.config.Lifecycle.IdleThreshold)
		}
		if v := patch.Lifecycle.SleepThreshold; v != "" {
			tryDuration("lifecycle.sleepThreshold", v, &s.config.Lifecycle.SleepThreshold)
		}
		if v := patch.Lifecycle.DormantThreshold; v != "" {
			tryDuration("lifecycle.dormantThreshold", v, &s.config.Lifecycle.DormantThreshold)
		}
		s.lifecycle.SetThresholds(
			s.config.Lifecycle.IdleThreshold,
			s.config.Lifecycle.SleepThreshold,
			s.config.Lifecycle.DormantThreshold,
		)
	}

	// Apply daemon patches
	if patch.Daemons != nil {
		if v := patch.Daemons.DecayInterval; v != "" {
			tryDuration("daemons.decayInterval", v, &s.config.Daemons.DecayInterval)
		}
		if v := patch.Daemons.ConsolidateInterval; v != "" {
			tryDuration("daemons.consolidateInterval", v, &s.config.Daemons.ConsolidateInterval)
		}
		if v := patch.Daemons.PruneInterval; v != "" {
			tryDuration("daemons.pruneInterval", v, &s.config.Daemons.PruneInterval)
		}
		if v := patch.Daemons.PersistInterval; v != "" {
			tryDuration("daemons.persistInterval", v, &s.config.Daemons.PersistInterval)
		}
		if v := patch.Daemons.ReorgInterval; v != "" {
			tryDuration("daemons.reorgInterval", v, &s.config.Daemons.ReorgInterval)
		}
		if s.daemons != nil {
			s.daemons.SetIntervals(
				s.config.Daemons.DecayInterval,
				s.config.Daemons.ConsolidateInterval,
				s.config.Daemons.PruneInterval,
				s.config.Daemons.PersistInterval,
				s.config.Daemons.ReorgInterval,
			)
		} else {
			rejected = append(rejected, "daemons.*: daemon manager not available in this runtime")
		}
	}

	// Apply worker patches
	if patch.Worker != nil {
		if v := patch.Worker.MaxIdleTime; v != "" {
			tryDuration("worker.maxIdleTime", v, &s.config.Worker.MaxIdleTime)
			s.pool.SetMaxIdleTime(s.config.Worker.MaxIdleTime)
		}
	}

	// Apply registry patches
	if patch.Registry != nil && patch.Registry.Enabled != nil {
		s.config.Registry.Enabled = *patch.Registry.Enabled
		changed = append(changed, "registry.enabled")
	}

	// Apply matrix patches
	if patch.Matrix != nil {
		if v := patch.Matrix.MaxNeurons; v != nil {
			if *v < 1 {
				rejected = append(rejected, "matrix.maxNeurons: must be positive")
			} else {
				s.config.Matrix.MaxNeurons = *v
				s.pool.SetMaxNeurons(*v)
				changed = append(changed, "matrix.maxNeurons")
			}
		}
	}

	// Apply security patches
	if patch.Security != nil {
		if v := patch.Security.AllowedOrigins; v != nil {
			s.config.Security.AllowedOrigins = *v
			changed = append(changed, "security.allowedOrigins")
		}
		if v := patch.Security.MaxRequestBody; v != nil {
			if *v < 0 {
				rejected = append(rejected, "security.maxRequestBody: must be >= 0")
			} else {
				s.config.Security.MaxRequestBody = *v
				changed = append(changed, "security.maxRequestBody")
			}
		}
	}

	// Apply vector patches
	if patch.Vector != nil {
		if v := patch.Vector.Alpha; v != nil {
			if *v < 0.0 || *v > 1.0 {
				rejected = append(rejected, "vector.alpha: must be 0.0–1.0")
			} else {
				s.config.Vector.Alpha = *v
				s.pool.SetVectorAlpha(*v)
				changed = append(changed, "vector.alpha")
			}
		}
	}

	if len(changed) == 0 {
		msg := "no valid runtime parameters provided"
		if len(rejected) > 0 {
			msg = "all parameters rejected"
		}
		apierr.BadRequest(w, apierr.CodeBadRequest, msg)
		return
	}

	resp := map[string]any{
		"ok":      true,
		"changed": changed,
		"count":   len(changed),
	}
	if len(rejected) > 0 {
		resp["rejected"] = rejected
	}
	json.NewEncoder(w).Encode(resp)
}
