package core

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// DefaultConfig tests
// ---------------------------------------------------------------------------

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	if cfg == nil {
		t.Fatal("DefaultConfig must not return nil")
	}

	// Server defaults
	if cfg.Server.HTTPAddr != ":6060" {
		t.Errorf("expected Server.HTTPAddr ':6060', got %q", cfg.Server.HTTPAddr)
	}

	// Storage defaults
	if cfg.Storage.DataPath != "./data" {
		t.Errorf("expected Storage.DataPath './data', got %q", cfg.Storage.DataPath)
	}
	if !cfg.Storage.Compress {
		t.Error("expected Storage.Compress to be true by default")
	}
	if !cfg.Storage.WALEnabled {
		t.Error("expected Storage.WALEnabled to be true by default")
	}
	if cfg.Storage.FsyncPolicy != "interval" {
		t.Errorf("expected Storage.FsyncPolicy 'interval', got %q", cfg.Storage.FsyncPolicy)
	}
	if cfg.Storage.FsyncInterval != 1*time.Second {
		t.Errorf("expected Storage.FsyncInterval 1s, got %v", cfg.Storage.FsyncInterval)
	}
	if cfg.Storage.ChecksumValidationInterval != 0 {
		t.Errorf("expected Storage.ChecksumValidationInterval 0, got %v", cfg.Storage.ChecksumValidationInterval)
	}
	if !cfg.Storage.StartupRepair {
		t.Error("expected Storage.StartupRepair true by default")
	}

	// Matrix defaults
	if cfg.Matrix.MinDimension != 3 {
		t.Errorf("expected Matrix.MinDimension 3, got %d", cfg.Matrix.MinDimension)
	}
	if cfg.Matrix.MaxDimension != 1000 {
		t.Errorf("expected Matrix.MaxDimension 1000, got %d", cfg.Matrix.MaxDimension)
	}
	if cfg.Matrix.MaxNeurons != 1000000 {
		t.Errorf("expected Matrix.MaxNeurons 1000000, got %d", cfg.Matrix.MaxNeurons)
	}

	// Lifecycle defaults
	if cfg.Lifecycle.IdleThreshold != 30*time.Second {
		t.Errorf("expected Lifecycle.IdleThreshold 30s, got %v", cfg.Lifecycle.IdleThreshold)
	}
	if cfg.Lifecycle.SleepThreshold != 5*time.Minute {
		t.Errorf("expected Lifecycle.SleepThreshold 5m, got %v", cfg.Lifecycle.SleepThreshold)
	}
	if cfg.Lifecycle.DormantThreshold != 30*time.Minute {
		t.Errorf("expected Lifecycle.DormantThreshold 30m, got %v", cfg.Lifecycle.DormantThreshold)
	}

	// Daemon defaults
	if cfg.Daemons.DecayInterval != 1*time.Minute {
		t.Errorf("expected Daemons.DecayInterval 1m, got %v", cfg.Daemons.DecayInterval)
	}
	if cfg.Daemons.ConsolidateInterval != 5*time.Minute {
		t.Errorf("expected Daemons.ConsolidateInterval 5m, got %v", cfg.Daemons.ConsolidateInterval)
	}
	if cfg.Daemons.PruneInterval != 10*time.Minute {
		t.Errorf("expected Daemons.PruneInterval 10m, got %v", cfg.Daemons.PruneInterval)
	}
	if cfg.Daemons.PersistInterval != 1*time.Minute {
		t.Errorf("expected Daemons.PersistInterval 1m, got %v", cfg.Daemons.PersistInterval)
	}
	if cfg.Daemons.ReorgInterval != 15*time.Minute {
		t.Errorf("expected Daemons.ReorgInterval 15m, got %v", cfg.Daemons.ReorgInterval)
	}

	// Worker defaults
	if cfg.Worker.MaxIdleTime != 30*time.Minute {
		t.Errorf("expected Worker.MaxIdleTime 30m, got %v", cfg.Worker.MaxIdleTime)
	}

	// Registry defaults
	if cfg.Registry.Enabled {
		t.Error("expected Registry.Enabled to be false by default")
	}

	// Security defaults
	if cfg.Security.AllowedOrigins != "http://localhost:6060" {
		t.Errorf("expected Security.AllowedOrigins 'http://localhost:6060', got %q", cfg.Security.AllowedOrigins)
	}
	if cfg.Security.MaxRequestBody != 1<<20 {
		t.Errorf("expected Security.MaxRequestBody 1MB, got %d", cfg.Security.MaxRequestBody)
	}
	if cfg.Security.MaxNeuronContentBytes != DefaultMaxNeuronContentBytes {
		t.Errorf("expected Security.MaxNeuronContentBytes %d, got %d", DefaultMaxNeuronContentBytes, cfg.Security.MaxNeuronContentBytes)
	}
	if cfg.Security.ReadTimeout != 30*time.Second {
		t.Errorf("expected Security.ReadTimeout 30s, got %v", cfg.Security.ReadTimeout)
	}
	if cfg.Security.WriteTimeout != 30*time.Second {
		t.Errorf("expected Security.WriteTimeout 30s, got %v", cfg.Security.WriteTimeout)
	}
	if cfg.Security.TLSCert != "" {
		t.Errorf("expected empty TLSCert by default, got %q", cfg.Security.TLSCert)
	}
	if cfg.Security.TLSKey != "" {
		t.Errorf("expected empty TLSKey by default, got %q", cfg.Security.TLSKey)
	}

	// Vector defaults
	if !cfg.Vector.Enabled {
		t.Error("expected Vector.Enabled to be true by default")
	}
	if cfg.Vector.ModelPath != "./dist/MiniLM-L6-v2.Q8_0.gguf" {
		t.Errorf("expected Vector.ModelPath './dist/MiniLM-L6-v2.Q8_0.gguf', got %q", cfg.Vector.ModelPath)
	}
	if cfg.Vector.Alpha != 0.6 {
		t.Errorf("expected Vector.Alpha 0.6, got %f", cfg.Vector.Alpha)
	}

	// Admin defaults
	if !cfg.Admin.Enabled {
		t.Error("expected Admin.Enabled to be true by default")
	}
	if cfg.Admin.User != "admin" {
		t.Errorf("expected Admin.User 'admin', got %q", cfg.Admin.User)
	}
	if cfg.Admin.Password != "qubicdb" {
		t.Errorf("expected Admin.Password 'qubicdb', got %q", cfg.Admin.Password)
	}

	// MCP defaults
	if cfg.MCP.Enabled {
		t.Error("expected MCP.Enabled to be false by default")
	}
	if cfg.MCP.Path != "/mcp" {
		t.Errorf("expected MCP.Path '/mcp', got %q", cfg.MCP.Path)
	}
	if !cfg.MCP.Stateless {
		t.Error("expected MCP.Stateless to be true by default")
	}
	if cfg.MCP.RateLimitRPS != 30 {
		t.Errorf("expected MCP.RateLimitRPS 30, got %f", cfg.MCP.RateLimitRPS)
	}
	if cfg.MCP.RateLimitBurst != 60 {
		t.Errorf("expected MCP.RateLimitBurst 60, got %d", cfg.MCP.RateLimitBurst)
	}
	if !cfg.MCP.EnablePrompts {
		t.Error("expected MCP.EnablePrompts to be true by default")
	}

	// Security defaults
	if cfg.Security.AllowedOrigins != "http://localhost:6060" {
		t.Errorf("expected Security.AllowedOrigins 'http://localhost:6060', got %q", cfg.Security.AllowedOrigins)
	}
	if cfg.Security.MaxRequestBody != 1<<20 {
		t.Errorf("expected Security.MaxRequestBody 1MB, got %d", cfg.Security.MaxRequestBody)
	}
	if cfg.Security.MaxNeuronContentBytes != DefaultMaxNeuronContentBytes {
		t.Errorf("expected Security.MaxNeuronContentBytes %d, got %d", DefaultMaxNeuronContentBytes, cfg.Security.MaxNeuronContentBytes)
	}
	if cfg.Security.ReadTimeout != 30*time.Second {
		t.Errorf("expected Security.ReadTimeout 30s, got %v", cfg.Security.ReadTimeout)
	}
	if cfg.Security.WriteTimeout != 30*time.Second {
		t.Errorf("expected Security.WriteTimeout 30s, got %v", cfg.Security.WriteTimeout)
	}
	if cfg.Security.TLSCert != "" {
		t.Errorf("expected empty TLSCert by default, got %q", cfg.Security.TLSCert)
	}
	if cfg.Security.TLSKey != "" {
		t.Errorf("expected empty TLSKey by default, got %q", cfg.Security.TLSKey)
	}
}

func TestDefaultConfigPassesValidation(t *testing.T) {
	cfg := DefaultConfig()
	if err := cfg.Validate(); err != nil {
		t.Errorf("DefaultConfig should pass validation, got: %v", err)
	}
}

// ---------------------------------------------------------------------------
// YAML config file tests
// ---------------------------------------------------------------------------

func TestConfigFromFile_FullOverride(t *testing.T) {
	yaml := `
server:
  httpAddr: ":9090"
storage:
  dataPath: "/tmp/qubicdb-test"
  compress: false
  walEnabled: false
  fsyncPolicy: "always"
  fsyncInterval: "2s"
  checksumValidationInterval: "45s"
  startupRepair: false
matrix:
  minDimension: 5
  maxDimension: 500
  maxNeurons: 50000
lifecycle:
  idleThreshold: 1m
  sleepThreshold: 10m
  dormantThreshold: 1h
daemons:
  decayInterval: 2m
  consolidateInterval: 10m
  pruneInterval: 20m
  persistInterval: 3m
  reorgInterval: 30m
worker:
  maxIdleTime: 1h
registry:
  enabled: true
mcp:
  enabled: true
  path: "/ai-mcp"
  apiKey: "token-123"
  stateless: false
  rateLimitRPS: 12.5
  rateLimitBurst: 25
  enablePrompts: false
  allowedTools: ["qubicdb_search", "qubicdb_context"]
`
	path := writeTempYAML(t, yaml)

	cfg, err := ConfigFromFile(path)
	if err != nil {
		t.Fatalf("ConfigFromFile failed: %v", err)
	}

	if cfg.Server.HTTPAddr != ":9090" {
		t.Errorf("expected ':9090', got %q", cfg.Server.HTTPAddr)
	}
	if cfg.Storage.DataPath != "/tmp/qubicdb-test" {
		t.Errorf("expected '/tmp/qubicdb-test', got %q", cfg.Storage.DataPath)
	}
	if cfg.Storage.Compress {
		t.Error("expected Compress false")
	}
	if cfg.Storage.WALEnabled {
		t.Error("expected WALEnabled false")
	}
	if cfg.Storage.FsyncPolicy != "always" {
		t.Errorf("expected FsyncPolicy always, got %q", cfg.Storage.FsyncPolicy)
	}
	if cfg.Storage.FsyncInterval != 2*time.Second {
		t.Errorf("expected FsyncInterval 2s, got %v", cfg.Storage.FsyncInterval)
	}
	if cfg.Storage.ChecksumValidationInterval != 45*time.Second {
		t.Errorf("expected ChecksumValidationInterval 45s, got %v", cfg.Storage.ChecksumValidationInterval)
	}
	if cfg.Storage.StartupRepair {
		t.Error("expected StartupRepair false")
	}
	if cfg.Matrix.MinDimension != 5 {
		t.Errorf("expected MinDimension 5, got %d", cfg.Matrix.MinDimension)
	}
	if cfg.Matrix.MaxDimension != 500 {
		t.Errorf("expected MaxDimension 500, got %d", cfg.Matrix.MaxDimension)
	}
	if cfg.Matrix.MaxNeurons != 50000 {
		t.Errorf("expected MaxNeurons 50000, got %d", cfg.Matrix.MaxNeurons)
	}
	if cfg.Lifecycle.IdleThreshold != 1*time.Minute {
		t.Errorf("expected IdleThreshold 1m, got %v", cfg.Lifecycle.IdleThreshold)
	}
	if cfg.Lifecycle.SleepThreshold != 10*time.Minute {
		t.Errorf("expected SleepThreshold 10m, got %v", cfg.Lifecycle.SleepThreshold)
	}
	if cfg.Lifecycle.DormantThreshold != 1*time.Hour {
		t.Errorf("expected DormantThreshold 1h, got %v", cfg.Lifecycle.DormantThreshold)
	}
	if cfg.Daemons.DecayInterval != 2*time.Minute {
		t.Errorf("expected DecayInterval 2m, got %v", cfg.Daemons.DecayInterval)
	}
	if cfg.Daemons.ConsolidateInterval != 10*time.Minute {
		t.Errorf("expected ConsolidateInterval 10m, got %v", cfg.Daemons.ConsolidateInterval)
	}
	if cfg.Daemons.PruneInterval != 20*time.Minute {
		t.Errorf("expected PruneInterval 20m, got %v", cfg.Daemons.PruneInterval)
	}
	if cfg.Daemons.PersistInterval != 3*time.Minute {
		t.Errorf("expected PersistInterval 3m, got %v", cfg.Daemons.PersistInterval)
	}
	if cfg.Daemons.ReorgInterval != 30*time.Minute {
		t.Errorf("expected ReorgInterval 30m, got %v", cfg.Daemons.ReorgInterval)
	}
	if cfg.Worker.MaxIdleTime != 1*time.Hour {
		t.Errorf("expected MaxIdleTime 1h, got %v", cfg.Worker.MaxIdleTime)
	}
	if !cfg.Registry.Enabled {
		t.Error("expected Registry.Enabled true")
	}
	if !cfg.MCP.Enabled {
		t.Error("expected MCP.Enabled true")
	}
	if cfg.MCP.Path != "/ai-mcp" {
		t.Errorf("expected MCP.Path /ai-mcp, got %q", cfg.MCP.Path)
	}
	if cfg.MCP.APIKey != "token-123" {
		t.Errorf("expected MCP.APIKey token-123, got %q", cfg.MCP.APIKey)
	}
	if cfg.MCP.Stateless {
		t.Error("expected MCP.Stateless false")
	}
	if cfg.MCP.RateLimitRPS != 12.5 {
		t.Errorf("expected MCP.RateLimitRPS 12.5, got %v", cfg.MCP.RateLimitRPS)
	}
	if cfg.MCP.RateLimitBurst != 25 {
		t.Errorf("expected MCP.RateLimitBurst 25, got %d", cfg.MCP.RateLimitBurst)
	}
	if cfg.MCP.EnablePrompts {
		t.Error("expected MCP.EnablePrompts false")
	}
	if len(cfg.MCP.AllowedTools) != 2 {
		t.Fatalf("expected 2 allowed MCP tools, got %d", len(cfg.MCP.AllowedTools))
	}
	if cfg.MCP.AllowedTools[0] != "qubicdb_search" || cfg.MCP.AllowedTools[1] != "qubicdb_context" {
		t.Errorf("unexpected MCP.AllowedTools: %#v", cfg.MCP.AllowedTools)
	}
}

func TestConfigFromFile_PartialOverride(t *testing.T) {
	yaml := `
server:
  httpAddr: ":7070"
matrix:
  maxNeurons: 999
`
	path := writeTempYAML(t, yaml)

	cfg, err := ConfigFromFile(path)
	if err != nil {
		t.Fatalf("ConfigFromFile failed: %v", err)
	}

	// Overridden values
	if cfg.Server.HTTPAddr != ":7070" {
		t.Errorf("expected ':7070', got %q", cfg.Server.HTTPAddr)
	}
	if cfg.Matrix.MaxNeurons != 999 {
		t.Errorf("expected MaxNeurons 999, got %d", cfg.Matrix.MaxNeurons)
	}

	// Non-overridden values should retain defaults
	if cfg.Storage.DataPath != "./data" {
		t.Errorf("DataPath should retain default './data', got %q", cfg.Storage.DataPath)
	}
	if cfg.Lifecycle.IdleThreshold != 30*time.Second {
		t.Errorf("IdleThreshold should retain default 30s, got %v", cfg.Lifecycle.IdleThreshold)
	}
}

func TestConfigFromFile_NotFound(t *testing.T) {
	_, err := ConfigFromFile("/nonexistent/path/qubicdb.yaml")
	if err == nil {
		t.Error("expected error for nonexistent file")
	}
}

func TestConfigFromFile_InvalidYAML(t *testing.T) {
	path := writeTempYAML(t, `{{{invalid yaml`)

	_, err := ConfigFromFile(path)
	if err == nil {
		t.Error("expected error for invalid YAML")
	}
}

// ---------------------------------------------------------------------------
// Environment variable override tests
// ---------------------------------------------------------------------------

func TestConfigFromEnv_AllVars(t *testing.T) {
	envs := map[string]string{
		"QUBICDB_HTTP_ADDR":                    ":8080",
		"QUBICDB_DATA_PATH":                    "/var/lib/qubicdb",
		"QUBICDB_COMPRESS":                     "false",
		"QUBICDB_WAL_ENABLED":                  "false",
		"QUBICDB_FSYNC_POLICY":                 "off",
		"QUBICDB_FSYNC_INTERVAL":               "3s",
		"QUBICDB_CHECKSUM_VALIDATION_INTERVAL": "90s",
		"QUBICDB_STARTUP_REPAIR":               "false",
		"QUBICDB_MIN_DIMENSION":                "10",
		"QUBICDB_MAX_DIMENSION":                "200",
		"QUBICDB_MAX_NEURONS":                  "5000",
		"QUBICDB_IDLE_THRESHOLD":               "1m",
		"QUBICDB_SLEEP_THRESHOLD":              "15m",
		"QUBICDB_DORMANT_THRESHOLD":            "2h",
		"QUBICDB_DECAY_INTERVAL":               "30s",
		"QUBICDB_CONSOLIDATE_INTERVAL":         "7m",
		"QUBICDB_PRUNE_INTERVAL":               "12m",
		"QUBICDB_PERSIST_INTERVAL":             "45s",
		"QUBICDB_REORG_INTERVAL":               "25m",
		"QUBICDB_MAX_IDLE_TIME":                "45m",
		"QUBICDB_REGISTRY_ENABLED":             "true",
		"QUBICDB_MCP_ENABLED":                  "true",
		"QUBICDB_MCP_PATH":                     "/mcp-custom",
		"QUBICDB_MCP_API_KEY":                  "mcp-secret",
		"QUBICDB_MCP_STATELESS":                "false",
		"QUBICDB_MCP_RATE_LIMIT_RPS":           "8.5",
		"QUBICDB_MCP_RATE_LIMIT_BURST":         "17",
		"QUBICDB_MCP_ENABLE_PROMPTS":           "false",
		"QUBICDB_MCP_ALLOWED_TOOLS":            "qubicdb_search, qubicdb_context",
		"QUBICDB_MAX_NEURON_CONTENT_BYTES":     "131072",
	}
	setEnvs(t, envs)

	cfg := ConfigFromEnv(nil)

	if cfg.Server.HTTPAddr != ":8080" {
		t.Errorf("expected ':8080', got %q", cfg.Server.HTTPAddr)
	}
	if cfg.Storage.DataPath != "/var/lib/qubicdb" {
		t.Errorf("expected '/var/lib/qubicdb', got %q", cfg.Storage.DataPath)
	}
	if cfg.Storage.Compress {
		t.Error("expected Compress false")
	}
	if cfg.Storage.WALEnabled {
		t.Error("expected WALEnabled false")
	}
	if cfg.Storage.FsyncPolicy != "off" {
		t.Errorf("expected FsyncPolicy off, got %q", cfg.Storage.FsyncPolicy)
	}
	if cfg.Storage.FsyncInterval != 3*time.Second {
		t.Errorf("expected FsyncInterval 3s, got %v", cfg.Storage.FsyncInterval)
	}
	if cfg.Storage.ChecksumValidationInterval != 90*time.Second {
		t.Errorf("expected ChecksumValidationInterval 90s, got %v", cfg.Storage.ChecksumValidationInterval)
	}
	if cfg.Storage.StartupRepair {
		t.Error("expected StartupRepair false")
	}
	if cfg.Matrix.MinDimension != 10 {
		t.Errorf("expected MinDimension 10, got %d", cfg.Matrix.MinDimension)
	}
	if cfg.Matrix.MaxDimension != 200 {
		t.Errorf("expected MaxDimension 200, got %d", cfg.Matrix.MaxDimension)
	}
	if cfg.Matrix.MaxNeurons != 5000 {
		t.Errorf("expected MaxNeurons 5000, got %d", cfg.Matrix.MaxNeurons)
	}
	if cfg.Lifecycle.IdleThreshold != 1*time.Minute {
		t.Errorf("expected IdleThreshold 1m, got %v", cfg.Lifecycle.IdleThreshold)
	}
	if cfg.Lifecycle.SleepThreshold != 15*time.Minute {
		t.Errorf("expected SleepThreshold 15m, got %v", cfg.Lifecycle.SleepThreshold)
	}
	if cfg.Lifecycle.DormantThreshold != 2*time.Hour {
		t.Errorf("expected DormantThreshold 2h, got %v", cfg.Lifecycle.DormantThreshold)
	}
	if cfg.Daemons.DecayInterval != 30*time.Second {
		t.Errorf("expected DecayInterval 30s, got %v", cfg.Daemons.DecayInterval)
	}
	if cfg.Daemons.ConsolidateInterval != 7*time.Minute {
		t.Errorf("expected ConsolidateInterval 7m, got %v", cfg.Daemons.ConsolidateInterval)
	}
	if cfg.Daemons.PruneInterval != 12*time.Minute {
		t.Errorf("expected PruneInterval 12m, got %v", cfg.Daemons.PruneInterval)
	}
	if cfg.Daemons.PersistInterval != 45*time.Second {
		t.Errorf("expected PersistInterval 45s, got %v", cfg.Daemons.PersistInterval)
	}
	if cfg.Daemons.ReorgInterval != 25*time.Minute {
		t.Errorf("expected ReorgInterval 25m, got %v", cfg.Daemons.ReorgInterval)
	}
	if cfg.Worker.MaxIdleTime != 45*time.Minute {
		t.Errorf("expected MaxIdleTime 45m, got %v", cfg.Worker.MaxIdleTime)
	}
	if !cfg.Registry.Enabled {
		t.Error("expected Registry.Enabled true")
	}
	if !cfg.MCP.Enabled {
		t.Error("expected MCP.Enabled true")
	}
	if cfg.MCP.Path != "/mcp-custom" {
		t.Errorf("expected MCP.Path /mcp-custom, got %q", cfg.MCP.Path)
	}
	if cfg.MCP.APIKey != "mcp-secret" {
		t.Errorf("expected MCP.APIKey mcp-secret, got %q", cfg.MCP.APIKey)
	}
	if cfg.MCP.Stateless {
		t.Error("expected MCP.Stateless false")
	}
	if cfg.MCP.RateLimitRPS != 8.5 {
		t.Errorf("expected MCP.RateLimitRPS 8.5, got %v", cfg.MCP.RateLimitRPS)
	}
	if cfg.MCP.RateLimitBurst != 17 {
		t.Errorf("expected MCP.RateLimitBurst 17, got %d", cfg.MCP.RateLimitBurst)
	}
	if cfg.MCP.EnablePrompts {
		t.Error("expected MCP.EnablePrompts false")
	}
	if len(cfg.MCP.AllowedTools) != 2 {
		t.Fatalf("expected 2 MCP allowed tools, got %d", len(cfg.MCP.AllowedTools))
	}
	if cfg.MCP.AllowedTools[0] != "qubicdb_search" || cfg.MCP.AllowedTools[1] != "qubicdb_context" {
		t.Errorf("unexpected MCP.AllowedTools: %#v", cfg.MCP.AllowedTools)
	}
}

func TestConfigFromEnv_NilInput(t *testing.T) {
	clearQubicDBEnvs(t)

	cfg := ConfigFromEnv(nil)
	if cfg == nil {
		t.Fatal("ConfigFromEnv(nil) must return a valid Config")
	}
	// Should match defaults when no env vars are set
	if cfg.Server.HTTPAddr != ":6060" {
		t.Errorf("expected default HTTPAddr, got %q", cfg.Server.HTTPAddr)
	}
}

func TestConfigFromEnv_IgnoresInvalidValues(t *testing.T) {
	clearQubicDBEnvs(t)
	t.Setenv("QUBICDB_MIN_DIMENSION", "not-a-number")
	t.Setenv("QUBICDB_IDLE_THRESHOLD", "bad-duration")
	t.Setenv("QUBICDB_COMPRESS", "maybe")

	cfg := ConfigFromEnv(nil)

	// All should retain defaults because the env values are unparseable
	if cfg.Matrix.MinDimension != 3 {
		t.Errorf("expected default MinDimension 3, got %d", cfg.Matrix.MinDimension)
	}
	if cfg.Lifecycle.IdleThreshold != 30*time.Second {
		t.Errorf("expected default IdleThreshold, got %v", cfg.Lifecycle.IdleThreshold)
	}
	if !cfg.Storage.Compress {
		t.Error("expected default Compress true")
	}
}

// ---------------------------------------------------------------------------
// LoadConfig hierarchy tests
// ---------------------------------------------------------------------------

func TestLoadConfig_DefaultsOnly(t *testing.T) {
	clearQubicDBEnvs(t)

	cfg, err := LoadConfig("")
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}
	if cfg.Server.HTTPAddr != ":6060" {
		t.Errorf("expected default ':6060', got %q", cfg.Server.HTTPAddr)
	}
}

func TestLoadConfig_YAMLThenEnv(t *testing.T) {
	// YAML sets HTTPAddr to :7070
	yaml := `
server:
  httpAddr: ":7070"
`
	path := writeTempYAML(t, yaml)

	// Env overrides HTTPAddr to :8080
	clearQubicDBEnvs(t)
	t.Setenv("QUBICDB_HTTP_ADDR", ":8080")

	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}

	// Env should win over YAML
	if cfg.Server.HTTPAddr != ":8080" {
		t.Errorf("env should override YAML: expected ':8080', got %q", cfg.Server.HTTPAddr)
	}
	// Untouched fields should retain defaults
	if cfg.Storage.DataPath != "./data" {
		t.Errorf("expected default DataPath, got %q", cfg.Storage.DataPath)
	}
}

func TestLoadConfig_InvalidFile(t *testing.T) {
	_, err := LoadConfig("/nonexistent/file.yaml")
	if err == nil {
		t.Error("expected error for nonexistent config file")
	}
}

// ---------------------------------------------------------------------------
// Validation tests
// ---------------------------------------------------------------------------

func TestValidate_ValidConfig(t *testing.T) {
	cfg := DefaultConfig()
	if err := cfg.Validate(); err != nil {
		t.Errorf("valid config should pass: %v", err)
	}
}

func TestValidate_EmptyHTTPAddr(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Server.HTTPAddr = ""
	if err := cfg.Validate(); err == nil {
		t.Error("empty HTTPAddr should fail validation")
	}
}

func TestValidate_EmptyDataPath(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Storage.DataPath = ""
	if err := cfg.Validate(); err == nil {
		t.Error("empty DataPath should fail validation")
	}
}

func TestValidate_InvalidFsyncPolicy(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Storage.FsyncPolicy = "invalid"
	if err := cfg.Validate(); err == nil {
		t.Error("invalid fsync policy should fail validation")
	}
}

func TestValidate_IntervalFsyncRequiresPositiveDuration(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Storage.FsyncPolicy = "interval"
	cfg.Storage.FsyncInterval = 0
	if err := cfg.Validate(); err == nil {
		t.Error("interval fsync with zero duration should fail validation")
	}
}

func TestValidate_MinDimensionZero(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Matrix.MinDimension = 0
	if err := cfg.Validate(); err == nil {
		t.Error("MinDimension 0 should fail validation")
	}
}

func TestValidate_MaxDimensionLessThanMin(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Matrix.MinDimension = 10
	cfg.Matrix.MaxDimension = 5
	if err := cfg.Validate(); err == nil {
		t.Error("MaxDimension < MinDimension should fail validation")
	}
}

func TestValidate_MaxNeuronsZero(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Matrix.MaxNeurons = 0
	if err := cfg.Validate(); err == nil {
		t.Error("MaxNeurons 0 should fail validation")
	}
}

func TestValidate_LifecycleOrdering(t *testing.T) {
	// IdleThreshold <= 0
	cfg := DefaultConfig()
	cfg.Lifecycle.IdleThreshold = 0
	if err := cfg.Validate(); err == nil {
		t.Error("IdleThreshold 0 should fail")
	}

	// SleepThreshold <= IdleThreshold
	cfg = DefaultConfig()
	cfg.Lifecycle.SleepThreshold = cfg.Lifecycle.IdleThreshold
	if err := cfg.Validate(); err == nil {
		t.Error("SleepThreshold == IdleThreshold should fail")
	}

	// DormantThreshold <= SleepThreshold
	cfg = DefaultConfig()
	cfg.Lifecycle.DormantThreshold = cfg.Lifecycle.SleepThreshold
	if err := cfg.Validate(); err == nil {
		t.Error("DormantThreshold == SleepThreshold should fail")
	}
}

func TestValidate_DaemonIntervalsPositive(t *testing.T) {
	fields := []struct {
		name string
		set  func(*Config)
	}{
		{"DecayInterval", func(c *Config) { c.Daemons.DecayInterval = 0 }},
		{"ConsolidateInterval", func(c *Config) { c.Daemons.ConsolidateInterval = 0 }},
		{"PruneInterval", func(c *Config) { c.Daemons.PruneInterval = 0 }},
		{"PersistInterval", func(c *Config) { c.Daemons.PersistInterval = 0 }},
		{"ReorgInterval", func(c *Config) { c.Daemons.ReorgInterval = 0 }},
	}

	for _, f := range fields {
		t.Run(f.name, func(t *testing.T) {
			cfg := DefaultConfig()
			f.set(cfg)
			if err := cfg.Validate(); err == nil {
				t.Errorf("%s = 0 should fail validation", f.name)
			}
		})
	}
}

func TestValidate_WorkerMaxIdleTimePositive(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Worker.MaxIdleTime = 0
	if err := cfg.Validate(); err == nil {
		t.Error("MaxIdleTime 0 should fail validation")
	}
}

// ---------------------------------------------------------------------------
// Env helper function tests
// ---------------------------------------------------------------------------

func TestSetEnvStr(t *testing.T) {
	t.Setenv("TEST_STR", "hello")
	var s string = "default"
	setEnvStr("TEST_STR", &s)
	if s != "hello" {
		t.Errorf("expected 'hello', got %q", s)
	}

	// Empty env should not change value
	t.Setenv("TEST_STR_EMPTY", "")
	s = "keep"
	setEnvStr("TEST_STR_EMPTY", &s)
	if s != "keep" {
		t.Errorf("empty env should not change value, got %q", s)
	}
}

func TestSetEnvBool(t *testing.T) {
	tests := []struct {
		envVal   string
		expected bool
		changed  bool
	}{
		{"true", true, true},
		{"false", false, true},
		{"1", true, true},
		{"0", false, true},
		{"invalid", false, false}, // should not change
		{"", false, false},        // should not change
	}

	for _, tt := range tests {
		t.Run(tt.envVal, func(t *testing.T) {
			key := "TEST_BOOL_" + tt.envVal
			if tt.envVal != "" {
				t.Setenv(key, tt.envVal)
			}
			b := false
			setEnvBool(key, &b)
			if tt.changed && b != tt.expected {
				t.Errorf("env %q: expected %v, got %v", tt.envVal, tt.expected, b)
			}
		})
	}
}

func TestSetEnvInt(t *testing.T) {
	t.Setenv("TEST_INT", "42")
	n := 0
	setEnvInt("TEST_INT", &n)
	if n != 42 {
		t.Errorf("expected 42, got %d", n)
	}

	// Invalid int should not change value
	t.Setenv("TEST_INT_BAD", "abc")
	n = 99
	setEnvInt("TEST_INT_BAD", &n)
	if n != 99 {
		t.Errorf("invalid int should not change value, got %d", n)
	}
}

func TestSetEnvDuration(t *testing.T) {
	t.Setenv("TEST_DUR", "5m30s")
	d := time.Duration(0)
	setEnvDuration("TEST_DUR", &d)
	if d != 5*time.Minute+30*time.Second {
		t.Errorf("expected 5m30s, got %v", d)
	}

	// Invalid duration should not change value
	t.Setenv("TEST_DUR_BAD", "nope")
	d = 10 * time.Second
	setEnvDuration("TEST_DUR_BAD", &d)
	if d != 10*time.Second {
		t.Errorf("invalid duration should not change value, got %v", d)
	}
}

// ---------------------------------------------------------------------------
// Test helpers
// ---------------------------------------------------------------------------

// writeTempYAML writes content to a temporary YAML file and returns its path.
func writeTempYAML(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "qubicdb.yaml")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write temp YAML: %v", err)
	}
	return path
}

// setEnvs sets multiple environment variables and registers cleanup.
func setEnvs(t *testing.T, envs map[string]string) {
	t.Helper()
	for k, v := range envs {
		t.Setenv(k, v)
	}
}

// clearQubicDBEnvs unsets all QUBICDB_ environment variables.
func clearQubicDBEnvs(t *testing.T) {
	t.Helper()
	keys := []string{
		"QUBICDB_HTTP_ADDR", "QUBICDB_DATA_PATH",
		"QUBICDB_COMPRESS", "QUBICDB_WAL_ENABLED", "QUBICDB_FSYNC_POLICY",
		"QUBICDB_FSYNC_INTERVAL", "QUBICDB_CHECKSUM_VALIDATION_INTERVAL", "QUBICDB_STARTUP_REPAIR",
		"QUBICDB_MIN_DIMENSION", "QUBICDB_MAX_DIMENSION",
		"QUBICDB_MAX_NEURONS", "QUBICDB_IDLE_THRESHOLD", "QUBICDB_SLEEP_THRESHOLD",
		"QUBICDB_DORMANT_THRESHOLD", "QUBICDB_DECAY_INTERVAL",
		"QUBICDB_CONSOLIDATE_INTERVAL", "QUBICDB_PRUNE_INTERVAL",
		"QUBICDB_PERSIST_INTERVAL", "QUBICDB_REORG_INTERVAL",
		"QUBICDB_MAX_IDLE_TIME", "QUBICDB_REGISTRY_ENABLED",
		"QUBICDB_VECTOR_ENABLED", "QUBICDB_VECTOR_MODEL_PATH",
		"QUBICDB_VECTOR_GPU_LAYERS", "QUBICDB_VECTOR_ALPHA",
		"QUBICDB_ADMIN_ENABLED", "QUBICDB_ADMIN_USER", "QUBICDB_ADMIN_PASSWORD",
		"QUBICDB_MCP_ENABLED", "QUBICDB_MCP_PATH", "QUBICDB_MCP_API_KEY",
		"QUBICDB_MCP_STATELESS", "QUBICDB_MCP_RATE_LIMIT_RPS", "QUBICDB_MCP_RATE_LIMIT_BURST",
		"QUBICDB_MCP_ENABLE_PROMPTS", "QUBICDB_MCP_ALLOWED_TOOLS",
		"QUBICDB_ALLOWED_ORIGINS", "QUBICDB_MAX_REQUEST_BODY",
		"QUBICDB_MAX_NEURON_CONTENT_BYTES",
		"QUBICDB_TLS_CERT", "QUBICDB_TLS_KEY",
		"QUBICDB_READ_TIMEOUT", "QUBICDB_WRITE_TIMEOUT",
	}
	for _, k := range keys {
		t.Setenv(k, "")
	}
}

// ---------------------------------------------------------------------------
// CLI overrides tests
// ---------------------------------------------------------------------------

func TestApplyCLIOverrides_NilOverrides(t *testing.T) {
	cfg := DefaultConfig()
	original := cfg.Server.HTTPAddr
	cfg.ApplyCLIOverrides(nil)
	if cfg.Server.HTTPAddr != original {
		t.Error("nil overrides should not change config")
	}
}

func TestApplyCLIOverrides_PartialOverride(t *testing.T) {
	cfg := DefaultConfig()
	addr := ":9999"
	compress := true
	cfg.ApplyCLIOverrides(&CLIOverrides{
		HTTPAddr: &addr,
		Compress: &compress,
	})

	if cfg.Server.HTTPAddr != ":9999" {
		t.Errorf("expected :9999, got %q", cfg.Server.HTTPAddr)
	}
	if !cfg.Storage.Compress {
		t.Error("expected compress=true")
	}
}

func TestApplyCLIOverrides_AllFields(t *testing.T) {
	cfg := DefaultConfig()

	httpAddr := ":7070"
	dataPath := "/tmp/cli"
	compress := true
	minDim := 5
	maxDim := 50
	maxNeurons := 500
	idle := 2 * time.Minute
	sleep := 5 * time.Minute
	dormant := 10 * time.Minute
	decay := 30 * time.Second
	consolidate := 3 * time.Minute
	prune := 7 * time.Minute
	persist := 45 * time.Second
	reorg := 20 * time.Minute
	maxIdle := 1 * time.Hour
	registry := true

	cfg.ApplyCLIOverrides(&CLIOverrides{
		HTTPAddr:         &httpAddr,
		DataPath:         &dataPath,
		Compress:         &compress,
		MinDimension:     &minDim,
		MaxDimension:     &maxDim,
		MaxNeurons:       &maxNeurons,
		IdleThreshold:    &idle,
		SleepThreshold:   &sleep,
		DormantThreshold: &dormant,
		DecayInterval:    &decay,
		ConsolidateInt:   &consolidate,
		PruneInterval:    &prune,
		PersistInterval:  &persist,
		ReorgInterval:    &reorg,
		MaxIdleTime:      &maxIdle,
		RegistryEnabled:  &registry,
	})

	if cfg.Server.HTTPAddr != ":7070" {
		t.Errorf("HTTPAddr: got %q", cfg.Server.HTTPAddr)
	}
	if cfg.Storage.DataPath != "/tmp/cli" {
		t.Errorf("DataPath: got %q", cfg.Storage.DataPath)
	}
	if !cfg.Storage.Compress {
		t.Error("Compress should be true")
	}
	if cfg.Matrix.MinDimension != 5 {
		t.Errorf("MinDimension: got %d", cfg.Matrix.MinDimension)
	}
	if cfg.Matrix.MaxDimension != 50 {
		t.Errorf("MaxDimension: got %d", cfg.Matrix.MaxDimension)
	}
	if cfg.Matrix.MaxNeurons != 500 {
		t.Errorf("MaxNeurons: got %d", cfg.Matrix.MaxNeurons)
	}
	if cfg.Lifecycle.IdleThreshold != 2*time.Minute {
		t.Errorf("IdleThreshold: got %v", cfg.Lifecycle.IdleThreshold)
	}
	if cfg.Lifecycle.SleepThreshold != 5*time.Minute {
		t.Errorf("SleepThreshold: got %v", cfg.Lifecycle.SleepThreshold)
	}
	if cfg.Lifecycle.DormantThreshold != 10*time.Minute {
		t.Errorf("DormantThreshold: got %v", cfg.Lifecycle.DormantThreshold)
	}
	if cfg.Daemons.DecayInterval != 30*time.Second {
		t.Errorf("DecayInterval: got %v", cfg.Daemons.DecayInterval)
	}
	if cfg.Daemons.ConsolidateInterval != 3*time.Minute {
		t.Errorf("ConsolidateInterval: got %v", cfg.Daemons.ConsolidateInterval)
	}
	if cfg.Daemons.PruneInterval != 7*time.Minute {
		t.Errorf("PruneInterval: got %v", cfg.Daemons.PruneInterval)
	}
	if cfg.Daemons.PersistInterval != 45*time.Second {
		t.Errorf("PersistInterval: got %v", cfg.Daemons.PersistInterval)
	}
	if cfg.Daemons.ReorgInterval != 20*time.Minute {
		t.Errorf("ReorgInterval: got %v", cfg.Daemons.ReorgInterval)
	}
	if cfg.Worker.MaxIdleTime != 1*time.Hour {
		t.Errorf("MaxIdleTime: got %v", cfg.Worker.MaxIdleTime)
	}
	if !cfg.Registry.Enabled {
		t.Error("Registry.Enabled should be true")
	}
}

func TestApplyCLIOverrides_WinsOverEnvAndYAML(t *testing.T) {
	clearQubicDBEnvs(t)
	t.Setenv("QUBICDB_HTTP_ADDR", ":8080")

	cfg, err := LoadConfig("")
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if cfg.Server.HTTPAddr != ":8080" {
		t.Fatalf("env should have set :8080, got %q", cfg.Server.HTTPAddr)
	}

	// CLI override wins
	cliAddr := ":9999"
	cfg.ApplyCLIOverrides(&CLIOverrides{HTTPAddr: &cliAddr})
	if cfg.Server.HTTPAddr != ":9999" {
		t.Errorf("CLI should override env: expected :9999, got %q", cfg.Server.HTTPAddr)
	}
}

// ---------------------------------------------------------------------------
// Admin & Security config tests
// ---------------------------------------------------------------------------

func TestConfigFromFile_AdminAndSecurityOverride(t *testing.T) {
	yaml := `
admin:
  enabled: false
  user: "superadmin"
  password: "s3cret"
security:
  allowedOrigins: "https://example.com"
  maxRequestBody: 2097152
  maxNeuronContentBytes: 131072
  readTimeout: "60s"
  writeTimeout: "60s"
  tlsCert: "/etc/ssl/cert.pem"
  tlsKey: "/etc/ssl/key.pem"
vector:
  enabled: true
  modelPath: "/models/embed.gguf"
  gpuLayers: 4
  alpha: 0.8
`
	path := writeTempYAML(t, yaml)

	cfg, err := ConfigFromFile(path)
	if err != nil {
		t.Fatalf("ConfigFromFile failed: %v", err)
	}

	// Admin
	if cfg.Admin.Enabled {
		t.Error("expected Admin.Enabled false")
	}
	if cfg.Admin.User != "superadmin" {
		t.Errorf("expected Admin.User 'superadmin', got %q", cfg.Admin.User)
	}
	if cfg.Admin.Password != "s3cret" {
		t.Errorf("expected Admin.Password 's3cret', got %q", cfg.Admin.Password)
	}

	// Security
	if cfg.Security.AllowedOrigins != "https://example.com" {
		t.Errorf("AllowedOrigins: got %q", cfg.Security.AllowedOrigins)
	}
	if cfg.Security.MaxRequestBody != 2097152 {
		t.Errorf("MaxRequestBody: got %d", cfg.Security.MaxRequestBody)
	}
	if cfg.Security.MaxNeuronContentBytes != 131072 {
		t.Errorf("MaxNeuronContentBytes: got %d", cfg.Security.MaxNeuronContentBytes)
	}
	if cfg.Security.ReadTimeout != 60*time.Second {
		t.Errorf("ReadTimeout: got %v", cfg.Security.ReadTimeout)
	}
	if cfg.Security.WriteTimeout != 60*time.Second {
		t.Errorf("WriteTimeout: got %v", cfg.Security.WriteTimeout)
	}
	if cfg.Security.TLSCert != "/etc/ssl/cert.pem" {
		t.Errorf("TLSCert: got %q", cfg.Security.TLSCert)
	}
	if cfg.Security.TLSKey != "/etc/ssl/key.pem" {
		t.Errorf("TLSKey: got %q", cfg.Security.TLSKey)
	}

	// Vector
	if !cfg.Vector.Enabled {
		t.Error("expected Vector.Enabled true")
	}
	if cfg.Vector.ModelPath != "/models/embed.gguf" {
		t.Errorf("ModelPath: got %q", cfg.Vector.ModelPath)
	}
	if cfg.Vector.GPULayers != 4 {
		t.Errorf("GPULayers: got %d", cfg.Vector.GPULayers)
	}
	if cfg.Vector.Alpha != 0.8 {
		t.Errorf("Alpha: got %f", cfg.Vector.Alpha)
	}
}

func TestConfigFromEnv_AdminAndSecurityVars(t *testing.T) {
	clearQubicDBEnvs(t)
	t.Setenv("QUBICDB_ADMIN_ENABLED", "false")
	t.Setenv("QUBICDB_ADMIN_USER", "root")
	t.Setenv("QUBICDB_ADMIN_PASSWORD", "topsecret")
	t.Setenv("QUBICDB_ALLOWED_ORIGINS", "https://app.example.com")
	t.Setenv("QUBICDB_MAX_REQUEST_BODY", "4194304")
	t.Setenv("QUBICDB_MAX_NEURON_CONTENT_BYTES", "262144")
	t.Setenv("QUBICDB_TLS_CERT", "/tls/cert.pem")
	t.Setenv("QUBICDB_TLS_KEY", "/tls/key.pem")
	t.Setenv("QUBICDB_READ_TIMEOUT", "45s")
	t.Setenv("QUBICDB_WRITE_TIMEOUT", "90s")
	t.Setenv("QUBICDB_VECTOR_ENABLED", "true")
	t.Setenv("QUBICDB_VECTOR_MODEL_PATH", "/env/model.gguf")
	t.Setenv("QUBICDB_CHECKSUM_VALIDATION_INTERVAL", "30s")
	t.Setenv("QUBICDB_STARTUP_REPAIR", "false")
	t.Setenv("QUBICDB_VECTOR_GPU_LAYERS", "8")
	t.Setenv("QUBICDB_VECTOR_ALPHA", "0.9")

	cfg := ConfigFromEnv(nil)

	if cfg.Admin.Enabled {
		t.Error("expected Admin.Enabled false from env")
	}
	if cfg.Admin.User != "root" {
		t.Errorf("Admin.User: got %q", cfg.Admin.User)
	}
	if cfg.Admin.Password != "topsecret" {
		t.Errorf("Admin.Password: got %q", cfg.Admin.Password)
	}
	if cfg.Security.AllowedOrigins != "https://app.example.com" {
		t.Errorf("AllowedOrigins: got %q", cfg.Security.AllowedOrigins)
	}
	if cfg.Security.MaxRequestBody != 4194304 {
		t.Errorf("MaxRequestBody: got %d", cfg.Security.MaxRequestBody)
	}
	if cfg.Security.MaxNeuronContentBytes != 262144 {
		t.Errorf("MaxNeuronContentBytes: got %d", cfg.Security.MaxNeuronContentBytes)
	}
	if cfg.Security.TLSCert != "/tls/cert.pem" {
		t.Errorf("TLSCert: got %q", cfg.Security.TLSCert)
	}
	if cfg.Security.TLSKey != "/tls/key.pem" {
		t.Errorf("TLSKey: got %q", cfg.Security.TLSKey)
	}
	if cfg.Security.ReadTimeout != 45*time.Second {
		t.Errorf("ReadTimeout: got %v", cfg.Security.ReadTimeout)
	}
	if cfg.Security.WriteTimeout != 90*time.Second {
		t.Errorf("WriteTimeout: got %v", cfg.Security.WriteTimeout)
	}
	if !cfg.Vector.Enabled {
		t.Error("expected Vector.Enabled true")
	}
	if cfg.Vector.ModelPath != "/env/model.gguf" {
		t.Errorf("Vector.ModelPath: got %q", cfg.Vector.ModelPath)
	}
	if cfg.Vector.GPULayers != 8 {
		t.Errorf("Vector.GPULayers: got %d", cfg.Vector.GPULayers)
	}
	if cfg.Vector.Alpha != 0.9 {
		t.Errorf("Vector.Alpha: got %f", cfg.Vector.Alpha)
	}
	if cfg.Storage.ChecksumValidationInterval != 30*time.Second {
		t.Errorf("Storage.ChecksumValidationInterval: got %v", cfg.Storage.ChecksumValidationInterval)
	}
	if cfg.Storage.StartupRepair {
		t.Error("expected Storage.StartupRepair false from env")
	}
}

func TestValidate_NegativeChecksumValidationInterval(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Storage.ChecksumValidationInterval = -1 * time.Second
	if err := cfg.Validate(); err == nil {
		t.Error("negative checksum validation interval should fail validation")
	}
}

func TestApplyCLIOverrides_AdminAndSecurityFields(t *testing.T) {
	cfg := DefaultConfig()

	adminEnabled := false
	adminUser := "operator"
	adminPass := "op-secret"
	allowedOrigins := "https://prod.example.com"
	maxBody := int64(5 * 1024 * 1024)
	maxNeuronContentBytes := int64(128 * 1024)
	tlsCert := "/cli/cert.pem"
	tlsKey := "/cli/key.pem"

	cfg.ApplyCLIOverrides(&CLIOverrides{
		AdminEnabled:          &adminEnabled,
		AdminUser:             &adminUser,
		AdminPassword:         &adminPass,
		AllowedOrigins:        &allowedOrigins,
		MaxRequestBody:        &maxBody,
		MaxNeuronContentBytes: &maxNeuronContentBytes,
		TLSCert:               &tlsCert,
		TLSKey:                &tlsKey,
	})

	if cfg.Admin.Enabled {
		t.Error("Admin.Enabled should be false")
	}
	if cfg.Admin.User != "operator" {
		t.Errorf("Admin.User: got %q", cfg.Admin.User)
	}
	if cfg.Admin.Password != "op-secret" {
		t.Errorf("Admin.Password: got %q", cfg.Admin.Password)
	}
	if cfg.Security.AllowedOrigins != "https://prod.example.com" {
		t.Errorf("AllowedOrigins: got %q", cfg.Security.AllowedOrigins)
	}
	if cfg.Security.MaxRequestBody != 5*1024*1024 {
		t.Errorf("MaxRequestBody: got %d", cfg.Security.MaxRequestBody)
	}
	if cfg.Security.MaxNeuronContentBytes != 128*1024 {
		t.Errorf("MaxNeuronContentBytes: got %d", cfg.Security.MaxNeuronContentBytes)
	}
	if cfg.Security.TLSCert != "/cli/cert.pem" {
		t.Errorf("TLSCert: got %q", cfg.Security.TLSCert)
	}
	if cfg.Security.TLSKey != "/cli/key.pem" {
		t.Errorf("TLSKey: got %q", cfg.Security.TLSKey)
	}
}

// ---------------------------------------------------------------------------
// Validation — Admin, Security, Vector boundary tests
// ---------------------------------------------------------------------------

func TestValidate_AdminEmptyCredentials(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Admin.Enabled = true
	cfg.Admin.User = ""
	if err := cfg.Validate(); err == nil {
		t.Error("empty admin user should fail validation")
	}

	cfg = DefaultConfig()
	cfg.Admin.Enabled = true
	cfg.Admin.Password = ""
	if err := cfg.Validate(); err == nil {
		t.Error("empty admin password should fail validation")
	}
}

func TestValidate_AdminDisabledSkipsCredentialCheck(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Admin.Enabled = false
	cfg.Admin.User = ""
	cfg.Admin.Password = ""
	if err := cfg.Validate(); err != nil {
		t.Errorf("disabled admin should not require credentials: %v", err)
	}
}

func TestValidate_MCPPathMustStartWithSlash(t *testing.T) {
	cfg := DefaultConfig()
	cfg.MCP.Enabled = true
	cfg.MCP.Path = "mcp"
	if err := cfg.Validate(); err == nil {
		t.Error("MCP path without leading slash should fail validation")
	}
}

func TestValidate_MCPNegativeRateLimitRejected(t *testing.T) {
	cfg := DefaultConfig()
	cfg.MCP.RateLimitRPS = -1
	if err := cfg.Validate(); err == nil {
		t.Error("negative MCP rateLimitRPS should fail validation")
	}

	cfg = DefaultConfig()
	cfg.MCP.RateLimitBurst = -1
	if err := cfg.Validate(); err == nil {
		t.Error("negative MCP rateLimitBurst should fail validation")
	}
}

func TestValidate_MCPAllowedToolsRejectsUnknownValues(t *testing.T) {
	cfg := DefaultConfig()
	cfg.MCP.AllowedTools = []string{"qubicdb_search", "unknown_tool"}
	if err := cfg.Validate(); err == nil {
		t.Error("unknown MCP allowedTools entry should fail validation")
	}
}

func TestValidate_AdminDefaultPasswordRejectedInProduction(t *testing.T) {
	t.Setenv("QUBICDB_ENV", "production")
	cfg := DefaultConfig()
	if err := cfg.Validate(); err == nil {
		t.Error("default admin password should fail validation in production")
	}
}

func TestValidate_AdminNonDefaultPasswordAllowedInProduction(t *testing.T) {
	t.Setenv("QUBICDB_ENV", "production")
	cfg := DefaultConfig()
	cfg.Admin.Password = "change-me-now"
	if err := cfg.Validate(); err != nil {
		t.Errorf("non-default admin password should pass validation in production: %v", err)
	}
}

func TestValidate_SecurityNegativeMaxRequestBody(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Security.MaxRequestBody = -1
	if err := cfg.Validate(); err == nil {
		t.Error("negative MaxRequestBody should fail validation")
	}
}

func TestValidate_SecurityZeroMaxRequestBodyAllowed(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Security.MaxRequestBody = 0 // unlimited, allowed but warned
	if err := cfg.Validate(); err != nil {
		t.Errorf("zero MaxRequestBody (unlimited) should pass validation: %v", err)
	}
}

func TestValidate_SecurityMaxNeuronContentBytesMustBePositive(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Security.MaxNeuronContentBytes = 0
	if err := cfg.Validate(); err == nil {
		t.Error("MaxNeuronContentBytes 0 should fail validation")
	}

	cfg = DefaultConfig()
	cfg.Security.MaxNeuronContentBytes = -1
	if err := cfg.Validate(); err == nil {
		t.Error("negative MaxNeuronContentBytes should fail validation")
	}
}

func TestValidate_SecurityWildcardOriginsRejectedWhenAdminEnabled(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Admin.Enabled = true
	cfg.Security.AllowedOrigins = "*"
	if err := cfg.Validate(); err == nil {
		t.Error("wildcard allowedOrigins should fail validation when admin is enabled")
	}
}

func TestValidate_SecurityWildcardOriginsAllowedWhenAdminDisabled(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Admin.Enabled = false
	cfg.Security.AllowedOrigins = "*"
	if err := cfg.Validate(); err != nil {
		t.Errorf("wildcard allowedOrigins should pass validation when admin is disabled: %v", err)
	}
}

func TestValidate_SecurityTimeoutsPositive(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Security.ReadTimeout = 0
	if err := cfg.Validate(); err == nil {
		t.Error("ReadTimeout 0 should fail validation")
	}

	cfg = DefaultConfig()
	cfg.Security.WriteTimeout = -1
	if err := cfg.Validate(); err == nil {
		t.Error("WriteTimeout negative should fail validation")
	}
}

func TestValidate_SecurityTLSCertWithoutKey(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Security.TLSCert = "/some/cert.pem"
	cfg.Security.TLSKey = ""
	if err := cfg.Validate(); err == nil {
		t.Error("TLSCert without TLSKey should fail validation")
	}
}

func TestValidate_SecurityTLSKeyWithoutCert(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Security.TLSKey = "/some/key.pem"
	cfg.Security.TLSCert = ""
	if err := cfg.Validate(); err == nil {
		t.Error("TLSKey without TLSCert should fail validation")
	}
}

func TestValidate_SecurityTLSBothSetPasses(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Security.TLSCert = "/some/cert.pem"
	cfg.Security.TLSKey = "/some/key.pem"
	if err := cfg.Validate(); err != nil {
		t.Errorf("TLS with both cert and key should pass: %v", err)
	}
}

func TestValidate_VectorAlphaOutOfRange(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Vector.Enabled = true
	cfg.Vector.Alpha = 1.5
	if err := cfg.Validate(); err == nil {
		t.Error("Alpha > 1.0 should fail validation")
	}

	cfg = DefaultConfig()
	cfg.Vector.Enabled = true
	cfg.Vector.Alpha = -0.1
	if err := cfg.Validate(); err == nil {
		t.Error("Alpha < 0.0 should fail validation")
	}
}

func TestValidate_VectorAlphaValidRange(t *testing.T) {
	for _, alpha := range []float64{0.0, 0.5, 1.0} {
		cfg := DefaultConfig()
		cfg.Vector.Enabled = true
		cfg.Vector.Alpha = alpha
		if err := cfg.Validate(); err != nil {
			t.Errorf("Alpha %f should pass validation: %v", alpha, err)
		}
	}
}

func TestValidate_VectorNegativeGPULayers(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Vector.Enabled = true
	cfg.Vector.GPULayers = -1
	if err := cfg.Validate(); err == nil {
		t.Error("negative GPULayers should fail validation")
	}
}

func TestValidate_VectorDisabledSkipsBoundaryChecks(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Vector.Enabled = false
	cfg.Vector.Alpha = 99.0 // invalid, but vector is disabled
	if err := cfg.Validate(); err != nil {
		t.Errorf("disabled vector should not validate alpha: %v", err)
	}
}

// ---------------------------------------------------------------------------
// setEnvInt64 and setEnvFloat helper tests
// ---------------------------------------------------------------------------

func TestSetEnvInt64(t *testing.T) {
	t.Setenv("TEST_INT64", "1048576")
	var n int64 = 0
	setEnvInt64("TEST_INT64", &n)
	if n != 1048576 {
		t.Errorf("expected 1048576, got %d", n)
	}

	t.Setenv("TEST_INT64_BAD", "not-a-number")
	n = 42
	setEnvInt64("TEST_INT64_BAD", &n)
	if n != 42 {
		t.Errorf("invalid int64 should not change value, got %d", n)
	}

	t.Setenv("TEST_INT64_EMPTY", "")
	n = 99
	setEnvInt64("TEST_INT64_EMPTY", &n)
	if n != 99 {
		t.Errorf("empty env should not change value, got %d", n)
	}
}

func TestSetEnvFloat(t *testing.T) {
	t.Setenv("TEST_FLOAT", "0.75")
	f := 0.0
	setEnvFloat("TEST_FLOAT", &f)
	if f != 0.75 {
		t.Errorf("expected 0.75, got %f", f)
	}

	t.Setenv("TEST_FLOAT_BAD", "xyz")
	f = 0.5
	setEnvFloat("TEST_FLOAT_BAD", &f)
	if f != 0.5 {
		t.Errorf("invalid float should not change value, got %f", f)
	}
}

// ---------------------------------------------------------------------------
// Full hierarchy test with Admin + Security
// ---------------------------------------------------------------------------

func TestFullHierarchy_AdminSecurityOverrides(t *testing.T) {
	// YAML sets admin user and security origins
	yaml := `
admin:
  user: "yaml-admin"
  password: "yaml-pass"
security:
  allowedOrigins: "https://yaml.example.com"
  maxRequestBody: 2097152
`
	path := writeTempYAML(t, yaml)

	// Env overrides admin password
	clearQubicDBEnvs(t)
	t.Setenv("QUBICDB_ADMIN_PASSWORD", "env-pass")

	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}

	// YAML values
	if cfg.Admin.User != "yaml-admin" {
		t.Errorf("Admin.User should be from YAML: got %q", cfg.Admin.User)
	}
	// Env wins over YAML
	if cfg.Admin.Password != "env-pass" {
		t.Errorf("Admin.Password should be from env: got %q", cfg.Admin.Password)
	}
	// YAML untouched by env
	if cfg.Security.AllowedOrigins != "https://yaml.example.com" {
		t.Errorf("AllowedOrigins should be from YAML: got %q", cfg.Security.AllowedOrigins)
	}
	if cfg.Security.MaxRequestBody != 2097152 {
		t.Errorf("MaxRequestBody should be from YAML: got %d", cfg.Security.MaxRequestBody)
	}

	// CLI wins over everything
	cliPass := "cli-pass"
	cliOrigins := "https://cli.example.com"
	cfg.ApplyCLIOverrides(&CLIOverrides{
		AdminPassword:  &cliPass,
		AllowedOrigins: &cliOrigins,
	})
	if cfg.Admin.Password != "cli-pass" {
		t.Errorf("CLI should override env: got %q", cfg.Admin.Password)
	}
	if cfg.Security.AllowedOrigins != "https://cli.example.com" {
		t.Errorf("CLI should override YAML: got %q", cfg.Security.AllowedOrigins)
	}
}
