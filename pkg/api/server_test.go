package api

import (
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/denizumutdereli/qubicdb/pkg/concurrency"
	"github.com/denizumutdereli/qubicdb/pkg/core"
	"github.com/denizumutdereli/qubicdb/pkg/lifecycle"
	"github.com/denizumutdereli/qubicdb/pkg/persistence"
	"github.com/denizumutdereli/qubicdb/pkg/registry"
)

// ---------------------------------------------------------------------------
// Test helpers
// ---------------------------------------------------------------------------

// newTestServer creates a minimal Server wired with real components for
// integration-style HTTP handler tests. The registry store uses a temp dir
// so tests don't pollute each other.
func newTestServer(t *testing.T, cfgMutator func(*core.Config)) *Server {
	t.Helper()

	cfg := core.DefaultConfig()
	cfg.Storage.DataPath = t.TempDir()
	if cfgMutator != nil {
		cfgMutator(cfg)
	}

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

	return NewServer(cfg.Server.HTTPAddr, pool, lm, reg, cfg)
}

// doRequest is a compact helper for firing HTTP requests at the test server.
func doRequest(t *testing.T, s *Server, method, path, body string, headers map[string]string) *httptest.ResponseRecorder {
	t.Helper()

	var bodyReader io.Reader
	if body != "" {
		bodyReader = strings.NewReader(body)
	}

	req := httptest.NewRequest(method, path, bodyReader)
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	rr := httptest.NewRecorder()
	s.httpServer.Handler.ServeHTTP(rr, req)
	return rr
}

// decodeJSON decodes the response body into a generic map.
func decodeJSON(t *testing.T, rr *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.NewDecoder(rr.Body).Decode(&m); err != nil {
		t.Fatalf("failed to decode response JSON: %v\nbody: %s", err, rr.Body.String())
	}
	return m
}

func adminAuthHeader(user, pass string) string {
	token := base64.StdEncoding.EncodeToString([]byte(user + ":" + pass))
	return "Basic " + token
}

// ---------------------------------------------------------------------------
// Health endpoint
// ---------------------------------------------------------------------------

func TestHealthEndpoint(t *testing.T) {
	s := newTestServer(t, nil)
	rr := doRequest(t, s, "GET", "/health", "", nil)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
	m := decodeJSON(t, rr)
	if m["status"] != "healthy" {
		t.Errorf("expected status 'healthy', got %v", m["status"])
	}
}

// ---------------------------------------------------------------------------
// CORS from config
// ---------------------------------------------------------------------------

func TestCORS_DefaultOrigin(t *testing.T) {
	s := newTestServer(t, nil)
	rr := doRequest(t, s, "OPTIONS", "/health", "", map[string]string{"Origin": "http://localhost:6060"})

	if rr.Code != http.StatusOK {
		t.Errorf("OPTIONS expected 200, got %d", rr.Code)
	}
	origin := rr.Header().Get("Access-Control-Allow-Origin")
	if origin != "http://localhost:6060" {
		t.Errorf("expected CORS origin 'http://localhost:6060', got %q", origin)
	}
}

func TestCORS_CustomOrigin(t *testing.T) {
	s := newTestServer(t, func(cfg *core.Config) {
		cfg.Security.AllowedOrigins = "https://app.example.com"
	})
	rr := doRequest(t, s, "OPTIONS", "/health", "", map[string]string{"Origin": "https://app.example.com"})

	origin := rr.Header().Get("Access-Control-Allow-Origin")
	if origin != "https://app.example.com" {
		t.Errorf("expected CORS origin 'https://app.example.com', got %q", origin)
	}
}

func TestCORS_AuthorizationHeaderAllowed(t *testing.T) {
	s := newTestServer(t, nil)
	rr := doRequest(t, s, "OPTIONS", "/health", "", nil)

	allowed := rr.Header().Get("Access-Control-Allow-Headers")
	if !strings.Contains(allowed, "Authorization") {
		t.Errorf("CORS should allow Authorization header, got %q", allowed)
	}
}

// ---------------------------------------------------------------------------
// MCP endpoint wiring
// ---------------------------------------------------------------------------

func TestMCP_DisabledReturnsNotFound(t *testing.T) {
	s := newTestServer(t, func(cfg *core.Config) {
		cfg.MCP.Enabled = false
	})

	rr := doRequest(t, s, "POST", "/mcp", `{}`, map[string]string{
		"Content-Type": "application/json",
	})

	if rr.Code != http.StatusNotFound {
		t.Fatalf("expected 404 when MCP disabled, got %d", rr.Code)
	}
}

func TestMCP_EnabledRejectsMissingAPIKey(t *testing.T) {
	s := newTestServer(t, func(cfg *core.Config) {
		cfg.MCP.Enabled = true
		cfg.MCP.APIKey = "secret"
	})

	rr := doRequest(t, s, "POST", "/mcp", `{}`, map[string]string{
		"Content-Type": "application/json",
	})

	if rr.Code != http.StatusUnauthorized && rr.Code != http.StatusNotFound {
		t.Fatalf("expected 401 (MCP active) or 404 (MCP unavailable), got %d (body=%s)", rr.Code, rr.Body.String())
	}
}

func TestMCP_EnabledCustomPathIsRouted(t *testing.T) {
	s := newTestServer(t, func(cfg *core.Config) {
		cfg.MCP.Enabled = true
		cfg.MCP.Path = "/ai-mcp"
		cfg.MCP.APIKey = "secret"
	})

	rr := doRequest(t, s, "POST", "/ai-mcp", `{}`, map[string]string{
		"Content-Type": "application/json",
		"X-API-Key":    "secret",
	})

	if rr.Code == http.StatusUnauthorized {
		t.Fatalf("expected authorized MCP request with key, got 401")
	}
	if rr.Code == http.StatusInternalServerError {
		t.Fatalf("expected MCP request to avoid 500, got %d", rr.Code)
	}
}

// ---------------------------------------------------------------------------
// Request body size limit
// ---------------------------------------------------------------------------

func TestBodySizeLimit_RejectsOversized(t *testing.T) {
	s := newTestServer(t, func(cfg *core.Config) {
		cfg.Security.MaxRequestBody = 64 // 64 bytes max
		cfg.Registry.Enabled = false
	})

	// Create a body larger than 64 bytes
	bigBody := strings.Repeat("x", 128)
	rr := doRequest(t, s, "POST", "/v1/write", bigBody, map[string]string{
		"X-Index-ID":   "test-idx",
		"Content-Type": "application/json",
	})

	// Should get an error (either 413 or 400 from MaxBytesReader)
	if rr.Code == http.StatusOK {
		t.Error("expected error for oversized body, got 200")
	}
}

func TestBodySizeLimit_AllowsSmallBody(t *testing.T) {
	s := newTestServer(t, func(cfg *core.Config) {
		cfg.Security.MaxRequestBody = 1 << 20 // 1MB
		cfg.Registry.Enabled = false
	})

	body := `{"content":"hello world"}`
	rr := doRequest(t, s, "POST", "/v1/write", body, map[string]string{
		"X-Index-ID":   "test-idx",
		"Content-Type": "application/json",
	})

	// Should succeed (201 or 200)
	if rr.Code >= 400 {
		t.Errorf("expected success for small body, got %d: %s", rr.Code, rr.Body.String())
	}
}

// ---------------------------------------------------------------------------
// Admin auth middleware — requireAdmin()
// ---------------------------------------------------------------------------

func TestAdminAuth_NoCredentials(t *testing.T) {
	s := newTestServer(t, func(cfg *core.Config) {
		cfg.Admin.Enabled = true
		cfg.Admin.User = "admin"
		cfg.Admin.Password = "secret"
	})

	rr := doRequest(t, s, "GET", "/admin/indexes", "", nil)
	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 without credentials, got %d", rr.Code)
	}

	m := decodeJSON(t, rr)
	if m["code"] != "UNAUTHORIZED" {
		t.Errorf("expected UNAUTHORIZED code, got %v", m["code"])
	}

	// Should have WWW-Authenticate header
	wwwAuth := rr.Header().Get("WWW-Authenticate")
	if !strings.Contains(wwwAuth, "Basic") {
		t.Errorf("expected WWW-Authenticate Basic, got %q", wwwAuth)
	}
}

func TestAdminAuth_WrongCredentials(t *testing.T) {
	s := newTestServer(t, func(cfg *core.Config) {
		cfg.Admin.Enabled = true
		cfg.Admin.User = "admin"
		cfg.Admin.Password = "secret"
	})

	req := httptest.NewRequest("GET", "/admin/indexes", nil)
	req.SetBasicAuth("admin", "wrong-password")
	rr := httptest.NewRecorder()
	s.httpServer.Handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 with wrong password, got %d", rr.Code)
	}
}

func TestAdminAuth_WrongUsername(t *testing.T) {
	s := newTestServer(t, func(cfg *core.Config) {
		cfg.Admin.Enabled = true
		cfg.Admin.User = "admin"
		cfg.Admin.Password = "secret"
	})

	req := httptest.NewRequest("GET", "/admin/indexes", nil)
	req.SetBasicAuth("hacker", "secret")
	rr := httptest.NewRecorder()
	s.httpServer.Handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 with wrong username, got %d", rr.Code)
	}
}

func TestAdminAuth_CorrectCredentials(t *testing.T) {
	s := newTestServer(t, func(cfg *core.Config) {
		cfg.Admin.Enabled = true
		cfg.Admin.User = "admin"
		cfg.Admin.Password = "secret"
	})

	req := httptest.NewRequest("GET", "/admin/indexes", nil)
	req.SetBasicAuth("admin", "secret")
	rr := httptest.NewRecorder()
	s.httpServer.Handler.ServeHTTP(rr, req)

	if rr.Code == http.StatusUnauthorized {
		t.Error("expected auth to succeed with correct credentials")
	}
	// Should be 200 (list is just empty)
	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}
}

// ---------------------------------------------------------------------------
// Admin endpoints gating (admin.enabled = false)
// ---------------------------------------------------------------------------

func TestAdminDisabled_Returns404(t *testing.T) {
	s := newTestServer(t, func(cfg *core.Config) {
		cfg.Admin.Enabled = false
	})

	endpoints := []string{
		"/admin/login",
		"/admin/indexes",
		"/admin/daemons",
		"/admin/gc",
		"/admin/persist",
	}

	for _, ep := range endpoints {
		t.Run(ep, func(t *testing.T) {
			rr := doRequest(t, s, "GET", ep, "", nil)
			if rr.Code != http.StatusNotFound {
				t.Errorf("admin disabled: %s expected 404, got %d", ep, rr.Code)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Admin login endpoint
// ---------------------------------------------------------------------------

func TestAdminLogin_Success(t *testing.T) {
	s := newTestServer(t, func(cfg *core.Config) {
		cfg.Admin.Enabled = true
		cfg.Admin.User = "admin"
		cfg.Admin.Password = "mypass"
	})

	body := `{"user":"admin","password":"mypass"}`
	rr := doRequest(t, s, "POST", "/admin/login", body, map[string]string{
		"Content-Type": "application/json",
	})

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}
	m := decodeJSON(t, rr)
	if m["success"] != true {
		t.Errorf("expected success=true, got %v", m["success"])
	}
}

func TestAdminLogin_Failure(t *testing.T) {
	s := newTestServer(t, func(cfg *core.Config) {
		cfg.Admin.Enabled = true
		cfg.Admin.User = "admin"
		cfg.Admin.Password = "mypass"
	})

	body := `{"user":"admin","password":"wrongpass"}`
	rr := doRequest(t, s, "POST", "/admin/login", body, map[string]string{
		"Content-Type": "application/json",
	})

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rr.Code)
	}
}

func TestAdminLogin_MethodNotAllowed(t *testing.T) {
	s := newTestServer(t, func(cfg *core.Config) {
		cfg.Admin.Enabled = true
	})

	rr := doRequest(t, s, "GET", "/admin/login", "", nil)
	if rr.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected 405, got %d", rr.Code)
	}
}

func TestAdminLogin_InvalidJSON(t *testing.T) {
	s := newTestServer(t, func(cfg *core.Config) {
		cfg.Admin.Enabled = true
		cfg.Admin.User = "admin"
		cfg.Admin.Password = "mypass"
	})

	rr := doRequest(t, s, "POST", "/admin/login", `{"user":`, map[string]string{
		"Content-Type": "application/json",
	})
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rr.Code, rr.Body.String())
	}
	m := decodeJSON(t, rr)
	if m["code"] != "INVALID_JSON" {
		t.Fatalf("expected INVALID_JSON, got %v", m["code"])
	}
}

func TestRateLimit_TooManyRequests(t *testing.T) {
	s := newTestServer(t, nil)
	s.rateLimitEnabled = true
	s.rateLimitRequests = 2
	s.rateLimitWindow = time.Minute

	for i := 0; i < 2; i++ {
		rr := doRequest(t, s, "GET", "/health", "", nil)
		if rr.Code != http.StatusOK {
			t.Fatalf("request %d expected 200, got %d", i+1, rr.Code)
		}
	}

	rr := doRequest(t, s, "GET", "/health", "", nil)
	if rr.Code != http.StatusTooManyRequests {
		t.Fatalf("expected 429, got %d: %s", rr.Code, rr.Body.String())
	}
	m := decodeJSON(t, rr)
	if m["code"] != "RATE_LIMITED" {
		t.Fatalf("expected RATE_LIMITED, got %v", m["code"])
	}
	if rr.Header().Get("Retry-After") == "" {
		t.Fatal("expected Retry-After header on rate limit response")
	}
}

// ---------------------------------------------------------------------------
// Registry guard — getWorker() behavior with Registry.Enabled
// ---------------------------------------------------------------------------

func TestRegistryGuard_EnabledRejectsUnregistered(t *testing.T) {
	s := newTestServer(t, func(cfg *core.Config) {
		cfg.Registry.Enabled = true
	})

	// Try to write without registering the UUID first
	body := `{"content":"test"}`
	rr := doRequest(t, s, "POST", "/v1/write", body, map[string]string{
		"X-Index-ID":   "unregistered-uuid",
		"Content-Type": "application/json",
	})

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for unregistered UUID, got %d: %s", rr.Code, rr.Body.String())
	}
	m := decodeJSON(t, rr)
	if m["code"] != "UUID_NOT_REGISTERED" {
		t.Errorf("expected UUID_NOT_REGISTERED, got %v", m["code"])
	}
}

func TestRegistryGuard_EnabledAcceptsRegistered(t *testing.T) {
	s := newTestServer(t, func(cfg *core.Config) {
		cfg.Registry.Enabled = true
	})

	// Register the UUID first
	regBody := `{"uuid":"my-test-uuid"}`
	rr := doRequest(t, s, "POST", "/v1/registry", regBody, map[string]string{
		"Content-Type": "application/json",
	})
	if rr.Code >= 400 {
		t.Fatalf("registry create failed: %d %s", rr.Code, rr.Body.String())
	}

	// Now write should work
	body := `{"content":"hello"}`
	rr = doRequest(t, s, "POST", "/v1/write", body, map[string]string{
		"X-Index-ID":   "my-test-uuid",
		"Content-Type": "application/json",
	})

	if rr.Code >= 400 {
		t.Errorf("expected success for registered UUID, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestRegistryGuard_DisabledAllowsAnyUUID(t *testing.T) {
	s := newTestServer(t, func(cfg *core.Config) {
		cfg.Registry.Enabled = false
	})

	body := `{"content":"hello"}`
	rr := doRequest(t, s, "POST", "/v1/write", body, map[string]string{
		"X-Index-ID":   "any-random-uuid",
		"Content-Type": "application/json",
	})

	if rr.Code >= 400 {
		t.Errorf("registry disabled should allow any UUID, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestRegistryGuard_MissingIndexIDAlwaysFails(t *testing.T) {
	s := newTestServer(t, func(cfg *core.Config) {
		cfg.Registry.Enabled = false
	})

	body := `{"content":"hello"}`
	rr := doRequest(t, s, "POST", "/v1/write", body, map[string]string{
		"Content-Type": "application/json",
	})

	if rr.Code != http.StatusBadRequest {
		t.Errorf("missing Index-ID should return 400, got %d", rr.Code)
	}
	m := decodeJSON(t, rr)
	if m["code"] != "INDEX_ID_REQUIRED" {
		t.Errorf("expected INDEX_ID_REQUIRED, got %v", m["code"])
	}
}

// ---------------------------------------------------------------------------
// Config endpoint — full output coverage
// ---------------------------------------------------------------------------

func TestConfigEndpoint_ContainsAllSections(t *testing.T) {
	s := newTestServer(t, func(cfg *core.Config) {
		cfg.Admin.Enabled = true
		cfg.Admin.User = "testadmin"
		cfg.Vector.Enabled = true
		cfg.Vector.Alpha = 0.7
		cfg.Security.AllowedOrigins = "https://test.example.com"
		cfg.Security.MaxRequestBody = 2097152
	})

	rr := doRequest(t, s, "GET", "/v1/config", "", map[string]string{
		"Authorization": adminAuthHeader("testadmin", "qubicdb"),
	})
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}

	m := decodeJSON(t, rr)

	// Check all expected top-level sections exist
	sections := []string{"server", "storage", "matrix", "lifecycle", "daemons", "worker", "registry", "vector", "admin", "security"}
	for _, sec := range sections {
		if _, ok := m[sec]; !ok {
			t.Errorf("config response missing section %q", sec)
		}
	}

	// Verify specific values
	admin, ok := m["admin"].(map[string]any)
	if !ok {
		t.Fatal("admin section not a map")
	}
	if admin["enabled"] != true {
		t.Errorf("admin.enabled: got %v", admin["enabled"])
	}
	if admin["user"] != "testadmin" {
		t.Errorf("admin.user: got %v", admin["user"])
	}

	security, ok := m["security"].(map[string]any)
	if !ok {
		t.Fatal("security section not a map")
	}
	if security["allowedOrigins"] != "https://test.example.com" {
		t.Errorf("security.allowedOrigins: got %v", security["allowedOrigins"])
	}
	// maxRequestBody comes back as float64 from JSON
	if security["maxRequestBody"].(float64) != 2097152 {
		t.Errorf("security.maxRequestBody: got %v", security["maxRequestBody"])
	}

	vector, ok := m["vector"].(map[string]any)
	if !ok {
		t.Fatal("vector section not a map")
	}
	if vector["enabled"] != true {
		t.Errorf("vector.enabled: got %v", vector["enabled"])
	}
	if vector["alpha"].(float64) != 0.7 {
		t.Errorf("vector.alpha: got %v", vector["alpha"])
	}
}

// ---------------------------------------------------------------------------
// Server timeout configuration
// ---------------------------------------------------------------------------

func TestServerTimeoutsFromConfig(t *testing.T) {
	s := newTestServer(t, func(cfg *core.Config) {
		cfg.Security.ReadTimeout = 45 * time.Second
		cfg.Security.WriteTimeout = 90 * time.Second
	})

	if s.httpServer.ReadTimeout != 45*time.Second {
		t.Errorf("ReadTimeout: expected 45s, got %v", s.httpServer.ReadTimeout)
	}
	if s.httpServer.WriteTimeout != 90*time.Second {
		t.Errorf("WriteTimeout: expected 90s, got %v", s.httpServer.WriteTimeout)
	}
}

// ---------------------------------------------------------------------------
// Admin protected endpoints with auth
// ---------------------------------------------------------------------------

func TestAdminGC_RequiresAuth(t *testing.T) {
	s := newTestServer(t, func(cfg *core.Config) {
		cfg.Admin.Enabled = true
		cfg.Admin.User = "admin"
		cfg.Admin.Password = "pass123"
	})

	// Without auth
	rr := doRequest(t, s, "POST", "/admin/gc", "", nil)
	if rr.Code != http.StatusUnauthorized {
		t.Errorf("gc without auth: expected 401, got %d", rr.Code)
	}

	// With auth
	req := httptest.NewRequest("POST", "/admin/gc", nil)
	req.SetBasicAuth("admin", "pass123")
	rr = httptest.NewRecorder()
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code == http.StatusUnauthorized {
		t.Error("gc with correct auth should not return 401")
	}
}

func TestAdminPersist_RequiresAuth(t *testing.T) {
	s := newTestServer(t, func(cfg *core.Config) {
		cfg.Admin.Enabled = true
		cfg.Admin.User = "admin"
		cfg.Admin.Password = "pass123"
	})

	// Without auth
	rr := doRequest(t, s, "POST", "/admin/persist", "", nil)
	if rr.Code != http.StatusUnauthorized {
		t.Errorf("persist without auth: expected 401, got %d", rr.Code)
	}

	// With auth
	req := httptest.NewRequest("POST", "/admin/persist", nil)
	req.SetBasicAuth("admin", "pass123")
	rr = httptest.NewRecorder()
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code == http.StatusUnauthorized {
		t.Error("persist with correct auth should not return 401")
	}
}

func TestAdminDaemons_RequiresAuth(t *testing.T) {
	s := newTestServer(t, func(cfg *core.Config) {
		cfg.Admin.Enabled = true
		cfg.Admin.User = "admin"
		cfg.Admin.Password = "pass123"
	})

	rr := doRequest(t, s, "GET", "/admin/daemons", "", nil)
	if rr.Code != http.StatusUnauthorized {
		t.Errorf("daemons without auth: expected 401, got %d", rr.Code)
	}
}

// ---------------------------------------------------------------------------
// Write + Read round-trip (integration)
// ---------------------------------------------------------------------------

func TestWriteReadRoundTrip(t *testing.T) {
	s := newTestServer(t, func(cfg *core.Config) {
		cfg.Registry.Enabled = false
	})

	indexID := "roundtrip-test"

	// Write
	writeBody := `{"content":"integration test memory"}`
	rr := doRequest(t, s, "POST", "/v1/write", writeBody, map[string]string{
		"X-Index-ID":   indexID,
		"Content-Type": "application/json",
	})
	if rr.Code >= 400 {
		t.Fatalf("write failed: %d %s", rr.Code, rr.Body.String())
	}
	writeResp := decodeJSON(t, rr)
	neuronID, ok := writeResp["_id"].(string)
	if !ok || neuronID == "" {
		t.Fatalf("write did not return neuron _id: %v", writeResp)
	}

	// Read back
	rr = doRequest(t, s, "GET", "/v1/read/"+neuronID, "", map[string]string{
		"X-Index-ID": indexID,
	})
	if rr.Code != http.StatusOK {
		t.Fatalf("read failed: %d %s", rr.Code, rr.Body.String())
	}
	readResp := decodeJSON(t, rr)
	if readResp["content"] != "integration test memory" {
		t.Errorf("read content mismatch: got %v", readResp["content"])
	}

	// Recall (list all)
	rr = doRequest(t, s, "GET", "/v1/recall", "", map[string]string{
		"X-Index-ID": indexID,
	})
	if rr.Code != http.StatusOK {
		t.Fatalf("recall failed: %d %s", rr.Code, rr.Body.String())
	}
}

// ---------------------------------------------------------------------------
// Search endpoint
// ---------------------------------------------------------------------------

// ---------------------------------------------------------------------------
// Config SET endpoint — runtime patching
// ---------------------------------------------------------------------------

func TestConfigSet_DaemonInterval(t *testing.T) {
	s := newTestServer(t, func(cfg *core.Config) {
		cfg.Registry.Enabled = false
	})

	body := `{"daemons":{"decayInterval":"2m","pruneInterval":"15m"}}`
	rr := doRequest(t, s, "POST", "/v1/config", body, map[string]string{
		"Content-Type":  "application/json",
		"Authorization": adminAuthHeader("admin", "qubicdb"),
	})
	if rr.Code != http.StatusOK {
		t.Fatalf("config set failed: %d %s", rr.Code, rr.Body.String())
	}
	m := decodeJSON(t, rr)
	if m["ok"] != true {
		t.Error("expected ok=true")
	}
	changed := m["changed"].([]any)
	if len(changed) != 2 {
		t.Errorf("expected 2 changed, got %d", len(changed))
	}

	// Verify the values stuck
	if s.config.Daemons.DecayInterval != 2*time.Minute {
		t.Errorf("DecayInterval not updated: %v", s.config.Daemons.DecayInterval)
	}
	if s.config.Daemons.PruneInterval != 15*time.Minute {
		t.Errorf("PruneInterval not updated: %v", s.config.Daemons.PruneInterval)
	}
}

func TestConfigSet_LifecycleThresholds(t *testing.T) {
	s := newTestServer(t, nil)

	body := `{"lifecycle":{"idleThreshold":"1m","sleepThreshold":"10m","dormantThreshold":"1h"}}`
	rr := doRequest(t, s, "POST", "/v1/config", body, map[string]string{
		"Content-Type":  "application/json",
		"Authorization": adminAuthHeader("admin", "qubicdb"),
	})
	if rr.Code != http.StatusOK {
		t.Fatalf("config set failed: %d %s", rr.Code, rr.Body.String())
	}

	if s.config.Lifecycle.IdleThreshold != 1*time.Minute {
		t.Errorf("IdleThreshold: got %v", s.config.Lifecycle.IdleThreshold)
	}
	if s.config.Lifecycle.SleepThreshold != 10*time.Minute {
		t.Errorf("SleepThreshold: got %v", s.config.Lifecycle.SleepThreshold)
	}
	if s.config.Lifecycle.DormantThreshold != 1*time.Hour {
		t.Errorf("DormantThreshold: got %v", s.config.Lifecycle.DormantThreshold)
	}
}

func TestConfigSet_RegistryEnabled(t *testing.T) {
	s := newTestServer(t, func(cfg *core.Config) {
		cfg.Registry.Enabled = false
	})

	body := `{"registry":{"enabled":true}}`
	rr := doRequest(t, s, "POST", "/v1/config", body, map[string]string{
		"Content-Type":  "application/json",
		"Authorization": adminAuthHeader("admin", "qubicdb"),
	})
	if rr.Code != http.StatusOK {
		t.Fatalf("config set failed: %d %s", rr.Code, rr.Body.String())
	}
	if !s.config.Registry.Enabled {
		t.Error("registry.enabled should be true after patch")
	}
}

func TestConfigSet_MatrixMaxNeurons(t *testing.T) {
	s := newTestServer(t, nil)

	body := `{"matrix":{"maxNeurons":500000}}`
	rr := doRequest(t, s, "POST", "/v1/config", body, map[string]string{
		"Content-Type":  "application/json",
		"Authorization": adminAuthHeader("admin", "qubicdb"),
	})
	if rr.Code != http.StatusOK {
		t.Fatalf("config set failed: %d %s", rr.Code, rr.Body.String())
	}
	if s.config.Matrix.MaxNeurons != 500000 {
		t.Errorf("maxNeurons: got %d", s.config.Matrix.MaxNeurons)
	}
}

func TestConfigSet_MatrixMaxNeuronsRejectsNegative(t *testing.T) {
	s := newTestServer(t, nil)

	body := `{"matrix":{"maxNeurons":-1}}`
	rr := doRequest(t, s, "POST", "/v1/config", body, map[string]string{
		"Content-Type":  "application/json",
		"Authorization": adminAuthHeader("admin", "qubicdb"),
	})
	// Should fail — no valid changes
	if rr.Code == http.StatusOK {
		m := decodeJSON(t, rr)
		if m["ok"] == true {
			t.Error("negative maxNeurons should not succeed")
		}
	}
}

func TestConfigSet_SecurityAllowedOrigins(t *testing.T) {
	s := newTestServer(t, nil)

	body := `{"security":{"allowedOrigins":"https://prod.example.com"}}`
	rr := doRequest(t, s, "POST", "/v1/config", body, map[string]string{
		"Content-Type":  "application/json",
		"Authorization": adminAuthHeader("admin", "qubicdb"),
	})
	if rr.Code != http.StatusOK {
		t.Fatalf("config set failed: %d %s", rr.Code, rr.Body.String())
	}
	if s.config.Security.AllowedOrigins != "https://prod.example.com" {
		t.Errorf("allowedOrigins: got %q", s.config.Security.AllowedOrigins)
	}
}

func TestConfigSet_SecurityMaxRequestBody(t *testing.T) {
	s := newTestServer(t, nil)

	body := `{"security":{"maxRequestBody":5242880}}`
	rr := doRequest(t, s, "POST", "/v1/config", body, map[string]string{
		"Content-Type":  "application/json",
		"Authorization": adminAuthHeader("admin", "qubicdb"),
	})
	if rr.Code != http.StatusOK {
		t.Fatalf("config set failed: %d %s", rr.Code, rr.Body.String())
	}
	if s.config.Security.MaxRequestBody != 5242880 {
		t.Errorf("maxRequestBody: got %d", s.config.Security.MaxRequestBody)
	}
}

func TestConfigSet_SecurityMaxRequestBodyRejectsNegative(t *testing.T) {
	s := newTestServer(t, nil)

	body := `{"security":{"maxRequestBody":-1}}`
	rr := doRequest(t, s, "POST", "/v1/config", body, map[string]string{
		"Content-Type":  "application/json",
		"Authorization": adminAuthHeader("admin", "qubicdb"),
	})
	if rr.Code == http.StatusOK {
		m := decodeJSON(t, rr)
		if m["ok"] == true {
			t.Error("negative maxRequestBody should not succeed")
		}
	}
}

func TestConfigSet_VectorAlpha(t *testing.T) {
	s := newTestServer(t, func(cfg *core.Config) {
		cfg.Vector.Alpha = 0.6
	})

	body := `{"vector":{"alpha":0.8}}`
	rr := doRequest(t, s, "POST", "/v1/config", body, map[string]string{
		"Content-Type":  "application/json",
		"Authorization": adminAuthHeader("admin", "qubicdb"),
	})
	if rr.Code != http.StatusOK {
		t.Fatalf("config set failed: %d %s", rr.Code, rr.Body.String())
	}
	if s.config.Vector.Alpha != 0.8 {
		t.Errorf("alpha: got %f", s.config.Vector.Alpha)
	}
}

func TestConfigSet_VectorAlphaRejectsOutOfRange(t *testing.T) {
	s := newTestServer(t, nil)

	body := `{"vector":{"alpha":1.5}}`
	rr := doRequest(t, s, "POST", "/v1/config", body, map[string]string{
		"Content-Type":  "application/json",
		"Authorization": adminAuthHeader("admin", "qubicdb"),
	})
	if rr.Code == http.StatusOK {
		m := decodeJSON(t, rr)
		if m["ok"] == true {
			t.Error("alpha > 1.0 should not succeed")
		}
	}
}

func TestConfigSet_InvalidDuration(t *testing.T) {
	s := newTestServer(t, nil)

	body := `{"daemons":{"decayInterval":"not-a-duration"}}`
	rr := doRequest(t, s, "POST", "/v1/config", body, map[string]string{
		"Content-Type":  "application/json",
		"Authorization": adminAuthHeader("admin", "qubicdb"),
	})
	// Should fail — invalid duration rejected
	if rr.Code == http.StatusOK {
		m := decodeJSON(t, rr)
		if m["ok"] == true {
			t.Error("invalid duration should not succeed")
		}
	}
}

func TestConfigSet_MultiSection(t *testing.T) {
	s := newTestServer(t, nil)

	body := `{"daemons":{"decayInterval":"3m"},"registry":{"enabled":true},"vector":{"alpha":0.9}}`
	rr := doRequest(t, s, "POST", "/v1/config", body, map[string]string{
		"Content-Type":  "application/json",
		"Authorization": adminAuthHeader("admin", "qubicdb"),
	})
	if rr.Code != http.StatusOK {
		t.Fatalf("multi-section set failed: %d %s", rr.Code, rr.Body.String())
	}
	m := decodeJSON(t, rr)
	changed := m["changed"].([]any)
	if len(changed) != 3 {
		t.Errorf("expected 3 changed, got %d: %v", len(changed), changed)
	}
}

func TestConfigSet_WorkerMaxIdleTime(t *testing.T) {
	s := newTestServer(t, nil)

	body := `{"worker":{"maxIdleTime":"1h"}}`
	rr := doRequest(t, s, "POST", "/v1/config", body, map[string]string{
		"Content-Type":  "application/json",
		"Authorization": adminAuthHeader("admin", "qubicdb"),
	})
	if rr.Code != http.StatusOK {
		t.Fatalf("config set failed: %d %s", rr.Code, rr.Body.String())
	}
	if s.config.Worker.MaxIdleTime != 1*time.Hour {
		t.Errorf("maxIdleTime: got %v", s.config.Worker.MaxIdleTime)
	}
}

func TestConfigSet_EmptyBody(t *testing.T) {
	s := newTestServer(t, nil)

	body := `{}`
	rr := doRequest(t, s, "POST", "/v1/config", body, map[string]string{
		"Content-Type":  "application/json",
		"Authorization": adminAuthHeader("admin", "qubicdb"),
	})
	if rr.Code != http.StatusBadRequest {
		t.Errorf("empty patch should return 400, got %d", rr.Code)
	}
}

// ---------------------------------------------------------------------------
// Search endpoint
// ---------------------------------------------------------------------------

func TestSearchEndpoint(t *testing.T) {
	s := newTestServer(t, func(cfg *core.Config) {
		cfg.Registry.Enabled = false
	})

	indexID := "search-test"

	// Write a few memories
	for _, content := range []string{"Go is a compiled language", "Rust is safe", "Python is dynamic"} {
		body := `{"content":"` + content + `"}`
		rr := doRequest(t, s, "POST", "/v1/write", body, map[string]string{
			"X-Index-ID":   indexID,
			"Content-Type": "application/json",
		})
		if rr.Code >= 400 {
			t.Fatalf("write failed: %d %s", rr.Code, rr.Body.String())
		}
	}

	// Search
	searchBody := `{"query":"compiled language","depth":2,"limit":10}`
	rr := doRequest(t, s, "POST", "/v1/search", searchBody, map[string]string{
		"X-Index-ID":   indexID,
		"Content-Type": "application/json",
	})
	if rr.Code != http.StatusOK {
		t.Errorf("search failed: %d %s", rr.Code, rr.Body.String())
	}
}

func TestSearchEndpoint_InvalidJSON(t *testing.T) {
	s := newTestServer(t, func(cfg *core.Config) {
		cfg.Registry.Enabled = false
	})

	rr := doRequest(t, s, "POST", "/v1/search", `{"query":`, map[string]string{
		"X-Index-ID":   "search-json-test",
		"Content-Type": "application/json",
	})
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for invalid JSON, got %d: %s", rr.Code, rr.Body.String())
	}
	resp := decodeJSON(t, rr)
	if resp["code"] != "INVALID_JSON" {
		t.Fatalf("expected INVALID_JSON code, got %v", resp["code"])
	}
}

func TestSearchEndpoint_ClampsDepthAndLimit(t *testing.T) {
	s := newTestServer(t, func(cfg *core.Config) {
		cfg.Registry.Enabled = false
	})

	indexID := "search-clamp-test"
	for i := 0; i < maxSearchLimit+20; i++ {
		body := `{"content":"bulk search item ` + strconv.Itoa(i) + `"}`
		rr := doRequest(t, s, "POST", "/v1/write", body, map[string]string{
			"X-Index-ID":   indexID,
			"Content-Type": "application/json",
		})
		if rr.Code >= 400 {
			t.Fatalf("write %d failed: %d %s", i, rr.Code, rr.Body.String())
		}
	}

	rr := doRequest(t, s, "POST", "/v1/search", `{"query":"bulk","depth":999,"limit":9999}`, map[string]string{
		"X-Index-ID":   indexID,
		"Content-Type": "application/json",
	})
	if rr.Code != http.StatusOK {
		t.Fatalf("search failed: %d %s", rr.Code, rr.Body.String())
	}

	resp := decodeJSON(t, rr)
	if gotDepth, ok := resp["depth"].(float64); !ok || int(gotDepth) != maxSearchDepth {
		t.Fatalf("expected clamped depth=%d, got %v", maxSearchDepth, resp["depth"])
	}
	if gotCount, ok := resp["count"].(float64); !ok || int(gotCount) > maxSearchLimit {
		t.Fatalf("expected clamped count <= %d, got %v", maxSearchLimit, resp["count"])
	}
}

func TestContextEndpoint_EmptyCueRejected(t *testing.T) {
	s := newTestServer(t, func(cfg *core.Config) {
		cfg.Registry.Enabled = false
	})

	rr := doRequest(t, s, "POST", "/v1/context", `{"cue":"","maxTokens":256}`, map[string]string{
		"X-Index-ID":   "context-empty-cue",
		"Content-Type": "application/json",
	})
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for empty cue, got %d: %s", rr.Code, rr.Body.String())
	}
	resp := decodeJSON(t, rr)
	if resp["code"] != "QUERY_REQUIRED" {
		t.Fatalf("expected QUERY_REQUIRED code, got %v", resp["code"])
	}
}

// ---------------------------------------------------------------------------
// Metadata write + search (E2E)
// ---------------------------------------------------------------------------

func TestMetadataWrite_PreservedInResponse(t *testing.T) {
	s := newTestServer(t, func(cfg *core.Config) {
		cfg.Registry.Enabled = false
	})

	indexID := "meta-write-test"
	body := `{"content":"the hippocampus encodes episodic memory","metadata":{"thread_id":"conv-001","role":"user"}}`
	rr := doRequest(t, s, "POST", "/v1/write", body, map[string]string{
		"X-Index-ID":   indexID,
		"Content-Type": "application/json",
	})
	if rr.Code != http.StatusOK {
		t.Fatalf("write failed: %d %s", rr.Code, rr.Body.String())
	}

	resp := decodeJSON(t, rr)
	meta, ok := resp["metadata"].(map[string]any)
	if !ok {
		t.Fatalf("expected metadata in response, got: %v", resp["metadata"])
	}
	if meta["thread_id"] != "conv-001" {
		t.Errorf("expected thread_id=conv-001, got %v", meta["thread_id"])
	}
	if meta["role"] != "user" {
		t.Errorf("expected role=user, got %v", meta["role"])
	}
}

func TestMetadataSearch_BoostSoftMode(t *testing.T) {
	s := newTestServer(t, func(cfg *core.Config) {
		cfg.Registry.Enabled = false
	})

	indexID := "meta-search-boost"
	headers := map[string]string{"X-Index-ID": indexID, "Content-Type": "application/json"}

	// Write two neurons — one with thread_id, one without
	doRequest(t, s, "POST", "/v1/write",
		`{"content":"dopamine reward signal in the brain","metadata":{"thread_id":"conv-boost"}}`, headers)
	doRequest(t, s, "POST", "/v1/write",
		`{"content":"dopamine reward signal in the brain extra context"}`, headers)

	// Search with metadata boost (strict=false)
	rr := doRequest(t, s, "POST", "/v1/search",
		`{"query":"dopamine reward","metadata":{"thread_id":"conv-boost"},"strict":false}`, headers)
	if rr.Code != http.StatusOK {
		t.Fatalf("search failed: %d %s", rr.Code, rr.Body.String())
	}

	resp := decodeJSON(t, rr)
	results, ok := resp["results"].([]any)
	if !ok || len(results) == 0 {
		t.Fatalf("expected results, got: %v", resp)
	}
	// Both neurons should be returned (soft mode)
	if len(results) < 2 {
		t.Errorf("soft mode: expected both neurons, got %d", len(results))
	}
	// First result should have thread_id metadata
	first := results[0].(map[string]any)
	meta, _ := first["metadata"].(map[string]any)
	if meta == nil || meta["thread_id"] != "conv-boost" {
		t.Errorf("expected first result to have thread_id=conv-boost, got %v", meta)
	}
}

func TestMetadataSearch_StrictMode(t *testing.T) {
	s := newTestServer(t, func(cfg *core.Config) {
		cfg.Registry.Enabled = false
	})

	indexID := "meta-search-strict"
	headers := map[string]string{"X-Index-ID": indexID, "Content-Type": "application/json"}

	doRequest(t, s, "POST", "/v1/write",
		`{"content":"prefrontal cortex executive function","metadata":{"thread_id":"conv-strict"}}`, headers)
	doRequest(t, s, "POST", "/v1/write",
		`{"content":"prefrontal cortex executive function other thread","metadata":{"thread_id":"conv-other"}}`, headers)

	// Search with strict=true — only conv-strict thread
	rr := doRequest(t, s, "POST", "/v1/search",
		`{"query":"prefrontal cortex","metadata":{"thread_id":"conv-strict"},"strict":true}`, headers)
	if rr.Code != http.StatusOK {
		t.Fatalf("search failed: %d %s", rr.Code, rr.Body.String())
	}

	resp := decodeJSON(t, rr)
	results, ok := resp["results"].([]any)
	if !ok {
		t.Fatalf("expected results array, got: %v", resp)
	}
	if len(results) != 1 {
		t.Fatalf("strict mode: expected 1 result, got %d", len(results))
	}
	first := results[0].(map[string]any)
	meta, _ := first["metadata"].(map[string]any)
	if meta == nil || meta["thread_id"] != "conv-strict" {
		t.Errorf("strict mode: expected thread_id=conv-strict, got %v", meta)
	}
}

func TestMetadataSearch_GETQueryParam(t *testing.T) {
	s := newTestServer(t, func(cfg *core.Config) {
		cfg.Registry.Enabled = false
	})

	indexID := "meta-get-param"
	headers := map[string]string{"X-Index-ID": indexID, "Content-Type": "application/json"}

	doRequest(t, s, "POST", "/v1/write",
		`{"content":"sleep consolidates memory during REM","metadata":{"thread_id":"t-get"}}`, headers)
	doRequest(t, s, "POST", "/v1/write",
		`{"content":"sleep consolidates memory during REM other"}`, headers)

	// GET with metadata_thread_id query param + strict=true
	rr := doRequest(t, s, "GET",
		"/v1/search?q=sleep+memory&metadata_thread_id=t-get&strict=true", "", map[string]string{
			"X-Index-ID": indexID,
		})
	if rr.Code != http.StatusOK {
		t.Fatalf("GET search failed: %d %s", rr.Code, rr.Body.String())
	}

	resp := decodeJSON(t, rr)
	results, ok := resp["results"].([]any)
	if !ok {
		t.Fatalf("expected results, got: %v", resp)
	}
	if len(results) != 1 {
		t.Fatalf("GET strict: expected 1 result, got %d", len(results))
	}
}
