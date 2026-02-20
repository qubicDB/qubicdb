package core

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"

	"gopkg.in/yaml.v3"
)

const (
	// DefaultVectorModelPath is the default GGUF embedding model path used by local runtime configuration.
	DefaultVectorModelPath = "./dist/MiniLM-L6-v2.Q8_0.gguf"

	// DefaultVectorEnabled keeps semantic/hybrid search on by default for internal usage.
	DefaultVectorEnabled = true
)

var builtInMCPTools = map[string]struct{}{
	"qubicdb_write":                   {},
	"qubicdb_read":                    {},
	"qubicdb_search":                  {},
	"qubicdb_recall":                  {},
	"qubicdb_context":                 {},
	"qubicdb_registry_find_or_create": {},
}

// ---------------------------------------------------------------------------
// Config — Central configuration for a QubicDB server instance.
//
// The configuration is resolved through a four-level hierarchy where each
// layer overrides values set by the layer beneath it:
//
//	Priority (highest → lowest):
//	  1. Programmatic overrides (e.g. CLI flags applied after loading)
//	  2. YAML configuration file
//	  3. Environment variables (QUBICDB_* prefix)
//	  4. Built-in defaults
//
// All duration fields accept standard Go duration strings when supplied
// through the YAML file or environment variables (e.g. "30s", "5m", "1h").
// ---------------------------------------------------------------------------

// ServerConfig groups network listener settings.
type ServerConfig struct {
	// HTTPAddr is the TCP address the HTTP/REST API binds to.
	HTTPAddr string `yaml:"httpAddr"`
}

// StorageConfig groups persistence-related settings.
type StorageConfig struct {
	// DataPath is the directory where .nrdb brain files are stored.
	DataPath string `yaml:"dataPath"`

	// Compress enables msgpack-level compression for persistence.
	Compress bool `yaml:"compress"`

	// WALEnabled controls write-ahead logging for crash recovery.
	WALEnabled bool `yaml:"walEnabled"`

	// FsyncPolicy controls persistence fsync behavior: always | interval | off.
	FsyncPolicy string `yaml:"fsyncPolicy"`

	// FsyncInterval controls fsync cadence when fsyncPolicy is interval.
	FsyncInterval time.Duration `yaml:"fsyncInterval"`

	// ChecksumValidationInterval controls periodic on-disk .nrdb checksum scans.
	// 0 disables periodic background validation.
	ChecksumValidationInterval time.Duration `yaml:"checksumValidationInterval"`

	// StartupRepair enables startup integrity repair for corrupt/missing persisted data files.
	StartupRepair bool `yaml:"startupRepair"`
}

// MatrixConfig groups organic memory matrix bounds.
type MatrixConfig struct {
	// MinDimension is the initial dimensionality of a new brain matrix.
	MinDimension int `yaml:"minDimension"`

	// MaxDimension is the upper limit the matrix can grow to.
	MaxDimension int `yaml:"maxDimension"`

	// MaxNeurons is the hard cap on the number of neurons per brain.
	MaxNeurons int `yaml:"maxNeurons"`
}

// LifecycleConfig groups brain state transition thresholds.
type LifecycleConfig struct {
	// IdleThreshold is how long a brain must be inactive before
	// transitioning from Active → Idle.
	IdleThreshold time.Duration `yaml:"idleThreshold"`

	// SleepThreshold is how long a brain must be idle before
	// transitioning from Idle → Sleeping (consolidation begins).
	SleepThreshold time.Duration `yaml:"sleepThreshold"`

	// DormantThreshold is how long a brain must be sleeping before
	// transitioning from Sleeping → Dormant (eligible for eviction).
	DormantThreshold time.Duration `yaml:"dormantThreshold"`
}

// DaemonConfig groups background daemon interval settings.
type DaemonConfig struct {
	// DecayInterval controls how often the decay daemon runs.
	// Decay reduces energy of unused neurons over time.
	DecayInterval time.Duration `yaml:"decayInterval"`

	// ConsolidateInterval controls how often the consolidation daemon runs.
	// Consolidation moves important neurons to deeper, permanent layers.
	ConsolidateInterval time.Duration `yaml:"consolidateInterval"`

	// PruneInterval controls how often the pruning daemon runs.
	// Pruning removes dead neurons and weak synapses.
	PruneInterval time.Duration `yaml:"pruneInterval"`

	// PersistInterval controls how often in-memory state is flushed to disk.
	PersistInterval time.Duration `yaml:"persistInterval"`

	// ReorgInterval controls how often the matrix reorganisation daemon runs.
	// Reorg optimises spatial locality for frequently co-accessed neurons.
	ReorgInterval time.Duration `yaml:"reorgInterval"`
}

// WorkerConfig groups worker pool settings.
type WorkerConfig struct {
	// MaxIdleTime is the maximum duration a brain worker may remain idle
	// before being evicted from the in-memory pool.
	MaxIdleTime time.Duration `yaml:"maxIdleTime"`
}

// RegistryConfig groups UUID registry settings.
type RegistryConfig struct {
	// Enabled controls whether the UUID registry guard is active.
	// When true, only registered UUIDs may access brain operations.
	Enabled bool `yaml:"enabled"`
}

// VectorConfig groups embedding / vector search settings.
type VectorConfig struct {
	// Enabled activates the vector embedding layer.
	// When false, search falls back to pure lexical matching.
	Enabled bool `yaml:"enabled"`

	// ModelPath is the path to a GGUF BERT embedding model file.
	ModelPath string `yaml:"modelPath"`

	// GPULayers controls how many model layers are offloaded to GPU.
	// 0 = CPU only.
	GPULayers int `yaml:"gpuLayers"`

	// Alpha is the weight of the vector score in hybrid search.
	// finalScore = Alpha*vectorScore + (1-Alpha)*stringScore
	// Range: 0.0 (pure string) to 1.0 (pure vector). Default: 0.6
	Alpha float64 `yaml:"alpha"`

	// QueryRepeat controls how many times the query is repeated before embedding.
	// Repeating the query improves embedding quality for short queries by allowing
	// the model to attend to all query tokens bidirectionally (Springer et al. 2024).
	// 1 = no repeat (baseline), 2 = repeat once (recommended), 3 = repeat twice.
	// Default: 2
	QueryRepeat int `yaml:"queryRepeat"`

	// EmbedContextSize is the llama.cpp context window size used for embedding.
	// Must be >= 512 for MiniLM. Increase if QueryRepeat×queryTokens > 512.
	// Default: 512
	EmbedContextSize uint32 `yaml:"embedContextSize"`
}

// AdminConfig groups server administration settings.
type AdminConfig struct {
	// Enabled controls whether admin endpoints are active.
	// When false, all /admin/* routes return 404.
	Enabled bool `yaml:"enabled"`

	// User is the admin username for /admin/login authentication.
	User string `yaml:"user"`

	// Password is the admin password for /admin/login authentication.
	// WARNING: Change the default before deploying to production.
	Password string `yaml:"password"`
}

// MCPConfig groups Model Context Protocol endpoint settings.
type MCPConfig struct {
	// Enabled controls whether /mcp endpoint is exposed.
	Enabled bool `yaml:"enabled"`

	// Path is the HTTP route for MCP transport.
	Path string `yaml:"path"`

	// APIKey is optional shared secret validated from X-API-Key or Bearer token.
	APIKey string `yaml:"apiKey"`

	// Stateless enables stateless session-id handling for streamable HTTP.
	Stateless bool `yaml:"stateless"`

	// RateLimitRPS controls per-client rate limiting in requests/second.
	// Set to 0 to disable MCP-specific rate limiting.
	RateLimitRPS float64 `yaml:"rateLimitRPS"`

	// RateLimitBurst controls burst capacity for MCP-specific rate limiting.
	RateLimitBurst int `yaml:"rateLimitBurst"`

	// EnablePrompts toggles MCP prompt registration.
	EnablePrompts bool `yaml:"enablePrompts"`

	// AllowedTools is an optional allowlist; empty means all built-in MCP tools.
	AllowedTools []string `yaml:"allowedTools"`
}

// SecurityConfig groups network security and request-limiting settings.
type SecurityConfig struct {
	// AllowedOrigins controls the CORS Access-Control-Allow-Origin header.
	// Use "*" to allow all origins (development only) or a comma-separated
	// list of allowed origins for production.
	AllowedOrigins string `yaml:"allowedOrigins"`

	// MaxRequestBody is the maximum allowed HTTP request body size in bytes.
	// Requests exceeding this limit are rejected with 413 Payload Too Large.
	// Default: 1048576 (1 MB). Set to 0 to disable the limit (not recommended).
	MaxRequestBody int64 `yaml:"maxRequestBody"`

	// MaxNeuronContentBytes is the maximum allowed neuron content payload size in bytes.
	// Requests that try to write larger neuron content are rejected.
	// Default: 65536 (64 KB).
	MaxNeuronContentBytes int64 `yaml:"maxNeuronContentBytes"`

	// TLSCert is the path to a TLS certificate file for HTTPS.
	// Leave empty to disable TLS (plain HTTP). Requires TLSKey.
	TLSCert string `yaml:"tlsCert"`

	// TLSKey is the path to the TLS private key file.
	// Leave empty to disable TLS. Requires TLSCert.
	TLSKey string `yaml:"tlsKey"`

	// ReadTimeout is the maximum duration for reading the entire request.
	ReadTimeout time.Duration `yaml:"readTimeout"`

	// WriteTimeout is the maximum duration before timing out writes of the response.
	WriteTimeout time.Duration `yaml:"writeTimeout"`
}

// Config is the root configuration object for a QubicDB server.
type Config struct {
	Server    ServerConfig    `yaml:"server"`
	Storage   StorageConfig   `yaml:"storage"`
	Matrix    MatrixConfig    `yaml:"matrix"`
	Lifecycle LifecycleConfig `yaml:"lifecycle"`
	Daemons   DaemonConfig    `yaml:"daemons"`
	Worker    WorkerConfig    `yaml:"worker"`
	Registry  RegistryConfig  `yaml:"registry"`
	Vector    VectorConfig    `yaml:"vector"`
	Admin     AdminConfig     `yaml:"admin"`
	MCP       MCPConfig       `yaml:"mcp"`
	Security  SecurityConfig  `yaml:"security"`
}

// ---------------------------------------------------------------------------
// Factory functions
// ---------------------------------------------------------------------------

// DefaultConfig returns a Config populated with production-safe defaults.
func DefaultConfig() *Config {
	return &Config{
		Server: ServerConfig{
			HTTPAddr: ":6060",
		},
		Storage: StorageConfig{
			DataPath:                   "./data",
			Compress:                   true,
			WALEnabled:                 true,
			FsyncPolicy:                "interval",
			FsyncInterval:              1 * time.Second,
			ChecksumValidationInterval: 0,
			StartupRepair:              true,
		},
		Matrix: MatrixConfig{
			MinDimension: 3,
			MaxDimension: 1000,
			MaxNeurons:   1000000,
		},
		Lifecycle: LifecycleConfig{
			IdleThreshold:    30 * time.Second,
			SleepThreshold:   5 * time.Minute,
			DormantThreshold: 30 * time.Minute,
		},
		Daemons: DaemonConfig{
			DecayInterval:       1 * time.Minute,
			ConsolidateInterval: 5 * time.Minute,
			PruneInterval:       10 * time.Minute,
			PersistInterval:     1 * time.Minute,
			ReorgInterval:       15 * time.Minute,
		},
		Worker: WorkerConfig{
			MaxIdleTime: 30 * time.Minute,
		},
		Registry: RegistryConfig{
			Enabled: false,
		},
		Vector: VectorConfig{
			Enabled:          DefaultVectorEnabled,
			ModelPath:        DefaultVectorModelPath,
			GPULayers:        0,
			Alpha:            0.6,
			QueryRepeat:      2,
			EmbedContextSize: 512,
		},
		Admin: AdminConfig{
			Enabled:  true,
			User:     "admin",
			Password: "qubicdb",
		},
		MCP: MCPConfig{
			Enabled:        false,
			Path:           "/mcp",
			APIKey:         "",
			Stateless:      true,
			RateLimitRPS:   30,
			RateLimitBurst: 60,
			EnablePrompts:  true,
			AllowedTools:   nil,
		},
		Security: SecurityConfig{
			AllowedOrigins:        "http://localhost:6060",
			MaxRequestBody:        1 << 20, // 1 MB
			MaxNeuronContentBytes: DefaultMaxNeuronContentBytes,
			ReadTimeout:           30 * time.Second,
			WriteTimeout:          30 * time.Second,
		},
	}
}

// ConfigFromFile reads a YAML configuration file and merges it on top of
// the built-in defaults. Fields absent from the file retain their defaults.
func ConfigFromFile(path string) (*Config, error) {
	cfg := DefaultConfig()

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading config file %s: %w", path, err)
	}

	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("parsing config file %s: %w", path, err)
	}

	return cfg, nil
}

// ConfigFromEnv applies environment variable overrides to the given Config.
// If cfg is nil a new default Config is created first.
//
// Environment variable mapping (all optional, prefix QUBICDB_):
//
//	QUBICDB_HTTP_ADDR           → Server.HTTPAddr
//	QUBICDB_DATA_PATH           → Storage.DataPath
//	QUBICDB_COMPRESS            → Storage.Compress          ("true"/"false")
//	QUBICDB_WAL_ENABLED         → Storage.WALEnabled        ("true"/"false")
//	QUBICDB_FSYNC_POLICY        → Storage.FsyncPolicy       (always|interval|off)
//	QUBICDB_FSYNC_INTERVAL      → Storage.FsyncInterval     (duration string)
//	QUBICDB_CHECKSUM_VALIDATION_INTERVAL → Storage.ChecksumValidationInterval (duration string, 0=off)
//	QUBICDB_STARTUP_REPAIR      → Storage.StartupRepair     ("true"/"false")
//	QUBICDB_MIN_DIMENSION       → Matrix.MinDimension
//	QUBICDB_MAX_DIMENSION       → Matrix.MaxDimension
//	QUBICDB_MAX_NEURONS         → Matrix.MaxNeurons
//	QUBICDB_IDLE_THRESHOLD      → Lifecycle.IdleThreshold   (duration string)
//	QUBICDB_SLEEP_THRESHOLD     → Lifecycle.SleepThreshold  (duration string)
//	QUBICDB_DORMANT_THRESHOLD   → Lifecycle.DormantThreshold(duration string)
//	QUBICDB_DECAY_INTERVAL      → Daemons.DecayInterval     (duration string)
//	QUBICDB_CONSOLIDATE_INTERVAL→ Daemons.ConsolidateInterval
//	QUBICDB_PRUNE_INTERVAL      → Daemons.PruneInterval
//	QUBICDB_PERSIST_INTERVAL    → Daemons.PersistInterval
//	QUBICDB_REORG_INTERVAL      → Daemons.ReorgInterval
//	QUBICDB_MAX_IDLE_TIME       → Worker.MaxIdleTime
//	QUBICDB_REGISTRY_ENABLED    → Registry.Enabled          ("true"/"false")
//	QUBICDB_ADMIN_ENABLED       → Admin.Enabled             ("true"/"false")
//	QUBICDB_ADMIN_USER          → Admin.User
//	QUBICDB_ADMIN_PASSWORD      → Admin.Password
//	QUBICDB_MCP_ENABLED         → MCP.Enabled               ("true"/"false")
//	QUBICDB_MCP_PATH            → MCP.Path
//	QUBICDB_MCP_API_KEY         → MCP.APIKey
//	QUBICDB_MCP_STATELESS       → MCP.Stateless             ("true"/"false")
//	QUBICDB_MCP_RATE_LIMIT_RPS  → MCP.RateLimitRPS          (float)
//	QUBICDB_MCP_RATE_LIMIT_BURST→ MCP.RateLimitBurst        (integer)
//	QUBICDB_MCP_ENABLE_PROMPTS  → MCP.EnablePrompts         ("true"/"false")
//	QUBICDB_MCP_ALLOWED_TOOLS   → MCP.AllowedTools          (comma-separated)
//	QUBICDB_ALLOWED_ORIGINS     → Security.AllowedOrigins
//	QUBICDB_MAX_REQUEST_BODY    → Security.MaxRequestBody   (bytes, integer)
//	QUBICDB_MAX_NEURON_CONTENT_BYTES → Security.MaxNeuronContentBytes (bytes, integer)
//	QUBICDB_TLS_CERT            → Security.TLSCert
//	QUBICDB_TLS_KEY             → Security.TLSKey
//	QUBICDB_READ_TIMEOUT        → Security.ReadTimeout      (duration string)
//	QUBICDB_WRITE_TIMEOUT       → Security.WriteTimeout     (duration string)
func ConfigFromEnv(cfg *Config) *Config {
	if cfg == nil {
		cfg = DefaultConfig()
	}

	// -- Server --
	setEnvStr("QUBICDB_HTTP_ADDR", &cfg.Server.HTTPAddr)

	// -- Storage --
	setEnvStr("QUBICDB_DATA_PATH", &cfg.Storage.DataPath)
	setEnvBool("QUBICDB_COMPRESS", &cfg.Storage.Compress)
	setEnvBool("QUBICDB_WAL_ENABLED", &cfg.Storage.WALEnabled)
	setEnvStr("QUBICDB_FSYNC_POLICY", &cfg.Storage.FsyncPolicy)
	setEnvDuration("QUBICDB_FSYNC_INTERVAL", &cfg.Storage.FsyncInterval)
	setEnvDuration("QUBICDB_CHECKSUM_VALIDATION_INTERVAL", &cfg.Storage.ChecksumValidationInterval)
	setEnvBool("QUBICDB_STARTUP_REPAIR", &cfg.Storage.StartupRepair)

	// -- Matrix --
	setEnvInt("QUBICDB_MIN_DIMENSION", &cfg.Matrix.MinDimension)
	setEnvInt("QUBICDB_MAX_DIMENSION", &cfg.Matrix.MaxDimension)
	setEnvInt("QUBICDB_MAX_NEURONS", &cfg.Matrix.MaxNeurons)

	// -- Lifecycle --
	setEnvDuration("QUBICDB_IDLE_THRESHOLD", &cfg.Lifecycle.IdleThreshold)
	setEnvDuration("QUBICDB_SLEEP_THRESHOLD", &cfg.Lifecycle.SleepThreshold)
	setEnvDuration("QUBICDB_DORMANT_THRESHOLD", &cfg.Lifecycle.DormantThreshold)

	// -- Daemons --
	setEnvDuration("QUBICDB_DECAY_INTERVAL", &cfg.Daemons.DecayInterval)
	setEnvDuration("QUBICDB_CONSOLIDATE_INTERVAL", &cfg.Daemons.ConsolidateInterval)
	setEnvDuration("QUBICDB_PRUNE_INTERVAL", &cfg.Daemons.PruneInterval)
	setEnvDuration("QUBICDB_PERSIST_INTERVAL", &cfg.Daemons.PersistInterval)
	setEnvDuration("QUBICDB_REORG_INTERVAL", &cfg.Daemons.ReorgInterval)

	// -- Worker --
	setEnvDuration("QUBICDB_MAX_IDLE_TIME", &cfg.Worker.MaxIdleTime)

	// -- Registry --
	setEnvBool("QUBICDB_REGISTRY_ENABLED", &cfg.Registry.Enabled)

	// -- Vector --
	setEnvBool("QUBICDB_VECTOR_ENABLED", &cfg.Vector.Enabled)
	setEnvStr("QUBICDB_VECTOR_MODEL_PATH", &cfg.Vector.ModelPath)
	setEnvInt("QUBICDB_VECTOR_GPU_LAYERS", &cfg.Vector.GPULayers)
	setEnvFloat("QUBICDB_VECTOR_ALPHA", &cfg.Vector.Alpha)
	setEnvInt("QUBICDB_VECTOR_QUERY_REPEAT", &cfg.Vector.QueryRepeat)
	setEnvUint32("QUBICDB_VECTOR_EMBED_CONTEXT_SIZE", &cfg.Vector.EmbedContextSize)

	// -- Admin --
	setEnvBool("QUBICDB_ADMIN_ENABLED", &cfg.Admin.Enabled)
	setEnvStr("QUBICDB_ADMIN_USER", &cfg.Admin.User)
	setEnvStr("QUBICDB_ADMIN_PASSWORD", &cfg.Admin.Password)

	// -- MCP --
	setEnvBool("QUBICDB_MCP_ENABLED", &cfg.MCP.Enabled)
	setEnvStr("QUBICDB_MCP_PATH", &cfg.MCP.Path)
	setEnvStr("QUBICDB_MCP_API_KEY", &cfg.MCP.APIKey)
	setEnvBool("QUBICDB_MCP_STATELESS", &cfg.MCP.Stateless)
	setEnvFloat("QUBICDB_MCP_RATE_LIMIT_RPS", &cfg.MCP.RateLimitRPS)
	setEnvInt("QUBICDB_MCP_RATE_LIMIT_BURST", &cfg.MCP.RateLimitBurst)
	setEnvBool("QUBICDB_MCP_ENABLE_PROMPTS", &cfg.MCP.EnablePrompts)
	setEnvCSV("QUBICDB_MCP_ALLOWED_TOOLS", &cfg.MCP.AllowedTools)

	// -- Security --
	setEnvStr("QUBICDB_ALLOWED_ORIGINS", &cfg.Security.AllowedOrigins)
	setEnvInt64("QUBICDB_MAX_REQUEST_BODY", &cfg.Security.MaxRequestBody)
	setEnvInt64("QUBICDB_MAX_NEURON_CONTENT_BYTES", &cfg.Security.MaxNeuronContentBytes)
	setEnvStr("QUBICDB_TLS_CERT", &cfg.Security.TLSCert)
	setEnvStr("QUBICDB_TLS_KEY", &cfg.Security.TLSKey)
	setEnvDuration("QUBICDB_READ_TIMEOUT", &cfg.Security.ReadTimeout)
	setEnvDuration("QUBICDB_WRITE_TIMEOUT", &cfg.Security.WriteTimeout)

	return cfg
}

// LoadConfig implements the full four-level configuration hierarchy:
//
//  1. Start with built-in defaults.
//  2. If configPath is non-empty, overlay the YAML file.
//  3. Apply environment variable overrides.
//  4. The caller may then apply programmatic overrides (e.g. CLI flags).
//
// Returns the merged Config or an error if the file cannot be read/parsed.
func LoadConfig(configPath string) (*Config, error) {
	var cfg *Config

	if configPath != "" {
		var err error
		cfg, err = ConfigFromFile(configPath)
		if err != nil {
			return nil, err
		}
	} else {
		cfg = DefaultConfig()
	}

	cfg = ConfigFromEnv(cfg)
	return cfg, nil
}

// ---------------------------------------------------------------------------
// Validation
// ---------------------------------------------------------------------------

// Validate performs structural validation of the entire configuration.
// Returns a descriptive error for the first invalid field encountered.
func (c *Config) Validate() error {
	// Server
	if c.Server.HTTPAddr == "" {
		return fmt.Errorf("server.httpAddr must not be empty")
	}

	// Storage
	if c.Storage.DataPath == "" {
		return fmt.Errorf("storage.dataPath must not be empty")
	}
	policy := strings.ToLower(strings.TrimSpace(c.Storage.FsyncPolicy))
	if policy != "always" && policy != "interval" && policy != "off" {
		return fmt.Errorf("storage.fsyncPolicy must be one of always|interval|off")
	}
	c.Storage.FsyncPolicy = policy
	if c.Storage.FsyncPolicy == "interval" && c.Storage.FsyncInterval <= 0 {
		return fmt.Errorf("storage.fsyncInterval must be > 0 when storage.fsyncPolicy is interval")
	}
	if c.Storage.ChecksumValidationInterval < 0 {
		return fmt.Errorf("storage.checksumValidationInterval must be >= 0")
	}

	// Matrix
	if c.Matrix.MinDimension < 1 {
		return fmt.Errorf("matrix.minDimension must be >= 1, got %d", c.Matrix.MinDimension)
	}
	if c.Matrix.MaxDimension < c.Matrix.MinDimension {
		return fmt.Errorf("matrix.maxDimension (%d) must be >= matrix.minDimension (%d)",
			c.Matrix.MaxDimension, c.Matrix.MinDimension)
	}
	if c.Matrix.MaxNeurons < 1 {
		return fmt.Errorf("matrix.maxNeurons must be >= 1, got %d", c.Matrix.MaxNeurons)
	}

	// Lifecycle — ensure ordering makes sense
	if c.Lifecycle.IdleThreshold <= 0 {
		return fmt.Errorf("lifecycle.idleThreshold must be > 0")
	}
	if c.Lifecycle.SleepThreshold <= c.Lifecycle.IdleThreshold {
		return fmt.Errorf("lifecycle.sleepThreshold (%v) must be > lifecycle.idleThreshold (%v)",
			c.Lifecycle.SleepThreshold, c.Lifecycle.IdleThreshold)
	}
	if c.Lifecycle.DormantThreshold <= c.Lifecycle.SleepThreshold {
		return fmt.Errorf("lifecycle.dormantThreshold (%v) must be > lifecycle.sleepThreshold (%v)",
			c.Lifecycle.DormantThreshold, c.Lifecycle.SleepThreshold)
	}

	// Daemons — all intervals must be positive
	for name, d := range map[string]time.Duration{
		"daemons.decayInterval":       c.Daemons.DecayInterval,
		"daemons.consolidateInterval": c.Daemons.ConsolidateInterval,
		"daemons.pruneInterval":       c.Daemons.PruneInterval,
		"daemons.persistInterval":     c.Daemons.PersistInterval,
		"daemons.reorgInterval":       c.Daemons.ReorgInterval,
	} {
		if d <= 0 {
			return fmt.Errorf("%s must be > 0", name)
		}
	}

	// Worker
	if c.Worker.MaxIdleTime <= 0 {
		return fmt.Errorf("worker.maxIdleTime must be > 0")
	}

	// Matrix — boundary guards (unless you know what you are doing)
	if c.Matrix.MaxNeurons > 10_000_000 {
		log.Printf("⚠ WARNING: matrix.maxNeurons=%d is extremely high; memory usage will be significant — proceed only if you know what you are doing", c.Matrix.MaxNeurons)
	}
	if c.Matrix.MaxDimension > 10_000 {
		log.Printf("⚠ WARNING: matrix.maxDimension=%d is very high; this may degrade spatial operations — proceed only if you know what you are doing", c.Matrix.MaxDimension)
	}

	// Admin
	if c.Admin.Enabled {
		if c.Admin.User == "" || c.Admin.Password == "" {
			return fmt.Errorf("admin.user and admin.password must not be empty when admin is enabled")
		}
		if c.Admin.Password == "qubicdb" {
			if isProductionMode() {
				return fmt.Errorf("admin.password must not use default value in production")
			}
			log.Printf("⚠ WARNING: admin.password is set to the default value — change it before deploying to production")
		}
	}

	// MCP
	mcpPath := strings.TrimSpace(c.MCP.Path)
	if mcpPath == "" {
		mcpPath = "/mcp"
	}
	if !strings.HasPrefix(mcpPath, "/") {
		return fmt.Errorf("mcp.path must start with '/'")
	}
	if len(mcpPath) > 1 {
		mcpPath = strings.TrimRight(mcpPath, "/")
	}
	c.MCP.Path = mcpPath
	if c.MCP.RateLimitRPS < 0 {
		return fmt.Errorf("mcp.rateLimitRPS must be >= 0")
	}
	if c.MCP.RateLimitBurst < 0 {
		return fmt.Errorf("mcp.rateLimitBurst must be >= 0")
	}
	if len(c.MCP.AllowedTools) > 0 {
		dedup := make(map[string]struct{}, len(c.MCP.AllowedTools))
		invalid := make(map[string]struct{})
		tools := make([]string, 0, len(c.MCP.AllowedTools))
		for _, name := range c.MCP.AllowedTools {
			name = strings.TrimSpace(name)
			if name == "" {
				continue
			}
			if _, ok := dedup[name]; ok {
				continue
			}
			if _, ok := builtInMCPTools[name]; !ok {
				invalid[name] = struct{}{}
				continue
			}
			dedup[name] = struct{}{}
			tools = append(tools, name)
		}
		if len(invalid) > 0 {
			invalidTools := make([]string, 0, len(invalid))
			for name := range invalid {
				invalidTools = append(invalidTools, name)
			}
			sort.Strings(invalidTools)
			return fmt.Errorf("mcp.allowedTools contains unsupported tools: %s", strings.Join(invalidTools, ", "))
		}
		c.MCP.AllowedTools = tools
	}

	// Security
	if c.Security.MaxRequestBody < 0 {
		return fmt.Errorf("security.maxRequestBody must be >= 0 (0 = unlimited, not recommended)")
	}
	if c.Security.MaxNeuronContentBytes <= 0 {
		return fmt.Errorf("security.maxNeuronContentBytes must be > 0")
	}
	if c.Security.ReadTimeout <= 0 {
		return fmt.Errorf("security.readTimeout must be > 0")
	}
	if c.Security.WriteTimeout <= 0 {
		return fmt.Errorf("security.writeTimeout must be > 0")
	}
	if c.Admin.Enabled && c.Security.AllowedOrigins == "*" {
		return fmt.Errorf("security.allowedOrigins must not be '*' when admin is enabled")
	}
	if c.Security.AllowedOrigins == "*" {
		log.Printf("⚠ WARNING: security.allowedOrigins is set to \"*\" (allow all) — restrict for production use")
	}
	if c.Security.TLSCert != "" && c.Security.TLSKey == "" {
		return fmt.Errorf("security.tlsKey is required when security.tlsCert is set")
	}
	if c.Security.TLSKey != "" && c.Security.TLSCert == "" {
		return fmt.Errorf("security.tlsCert is required when security.tlsKey is set")
	}

	// Vector
	if c.Vector.Enabled {
		if c.Vector.Alpha < 0 || c.Vector.Alpha > 1 {
			return fmt.Errorf("vector.alpha must be between 0.0 and 1.0, got %f", c.Vector.Alpha)
		}
		if c.Vector.GPULayers < 0 {
			return fmt.Errorf("vector.gpuLayers must be >= 0, got %d", c.Vector.GPULayers)
		}
		if c.Vector.QueryRepeat < 1 || c.Vector.QueryRepeat > 3 {
			return fmt.Errorf("vector.queryRepeat must be 1, 2, or 3, got %d", c.Vector.QueryRepeat)
		}
		if c.Vector.EmbedContextSize < 512 {
			return fmt.Errorf("vector.embedContextSize must be >= 512, got %d", c.Vector.EmbedContextSize)
		}
	}

	// Daemon boundary guards
	if c.Daemons.DecayInterval < 5*time.Second {
		log.Printf("⚠ WARNING: daemons.decayInterval=%v is very aggressive — this will increase CPU usage", c.Daemons.DecayInterval)
	}
	if c.Daemons.PersistInterval < 5*time.Second {
		log.Printf("⚠ WARNING: daemons.persistInterval=%v is very aggressive — this will increase disk I/O", c.Daemons.PersistInterval)
	}

	return nil
}

func isProductionMode() bool {
	for _, key := range []string{"QUBICDB_ENV", "GO_ENV", "APP_ENV"} {
		v := strings.ToLower(strings.TrimSpace(os.Getenv(key)))
		if v == "production" || v == "prod" {
			return true
		}
	}
	return false
}

// ---------------------------------------------------------------------------
// Environment variable helpers
// ---------------------------------------------------------------------------

// setEnvStr sets *target to the value of the named env var if it is non-empty.
func setEnvStr(key string, target *string) {
	if v := os.Getenv(key); v != "" {
		*target = v
	}
}

// setEnvBool sets *target to the parsed boolean value of the named env var.
// Accepted values: "true", "1" → true; "false", "0" → false.
func setEnvBool(key string, target *bool) {
	if v := os.Getenv(key); v != "" {
		if b, err := strconv.ParseBool(v); err == nil {
			*target = b
		}
	}
}

// setEnvInt sets *target to the parsed integer value of the named env var.
func setEnvInt(key string, target *int) {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			*target = n
		}
	}
}

// setEnvInt64 sets *target to the parsed int64 value of the named env var.
func setEnvInt64(key string, target *int64) {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil {
			*target = n
		}
	}
}

// setEnvDuration sets *target to the parsed duration of the named env var.
// Uses time.ParseDuration, so accepts "30s", "5m", "1h30m", etc.
func setEnvDuration(key string, target *time.Duration) {
	if v := os.Getenv(key); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			*target = d
		}
	}
}

// setEnvFloat sets *target to the parsed float64 value of the named env var.
func setEnvFloat(key string, target *float64) {
	if v := os.Getenv(key); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			*target = f
		}
	}
}

// setEnvUint32 sets *target to the parsed uint32 value of the named env var.
func setEnvUint32(key string, target *uint32) {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.ParseUint(v, 10, 32); err == nil {
			*target = uint32(n)
		}
	}
}

// setEnvCSV sets *target to a comma-separated env var list.
func setEnvCSV(key string, target *[]string) {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		parts := strings.Split(v, ",")
		out := make([]string, 0, len(parts))
		for _, p := range parts {
			p = strings.TrimSpace(p)
			if p != "" {
				out = append(out, p)
			}
		}
		*target = out
	}
}

// ---------------------------------------------------------------------------
// CLI flag overrides — final layer of the configuration hierarchy.
// ---------------------------------------------------------------------------

// CLIOverrides carries optional values set via command-line flags.
// Pointer fields are nil when the flag was not explicitly provided,
// allowing the caller to distinguish "not set" from the zero value.
type CLIOverrides struct {
	ConfigPath             *string
	HTTPAddr               *string
	DataPath               *string
	Compress               *bool
	MinDimension           *int
	MaxDimension           *int
	MaxNeurons             *int
	IdleThreshold          *time.Duration
	SleepThreshold         *time.Duration
	DormantThreshold       *time.Duration
	DecayInterval          *time.Duration
	ConsolidateInt         *time.Duration
	PruneInterval          *time.Duration
	PersistInterval        *time.Duration
	ReorgInterval          *time.Duration
	MaxIdleTime            *time.Duration
	RegistryEnabled        *bool
	VectorEnabled          *bool
	VectorModelPath        *string
	VectorGPULayers        *int
	VectorAlpha            *float64
	VectorQueryRepeat      *int
	VectorEmbedContextSize *uint32
	AdminEnabled           *bool
	AdminUser              *string
	AdminPassword          *string
	AllowedOrigins         *string
	MaxRequestBody         *int64
	MaxNeuronContentBytes  *int64
	TLSCert                *string
	TLSKey                 *string
}

// ApplyCLIOverrides patches the Config with any explicitly-set CLI flags.
// Only non-nil fields in the CLIOverrides are applied, preserving all
// values resolved from earlier hierarchy layers.
func (c *Config) ApplyCLIOverrides(o *CLIOverrides) {
	if o == nil {
		return
	}
	if o.HTTPAddr != nil {
		c.Server.HTTPAddr = *o.HTTPAddr
	}
	if o.DataPath != nil {
		c.Storage.DataPath = *o.DataPath
	}
	if o.Compress != nil {
		c.Storage.Compress = *o.Compress
	}
	if o.MinDimension != nil {
		c.Matrix.MinDimension = *o.MinDimension
	}
	if o.MaxDimension != nil {
		c.Matrix.MaxDimension = *o.MaxDimension
	}
	if o.MaxNeurons != nil {
		c.Matrix.MaxNeurons = *o.MaxNeurons
	}
	if o.IdleThreshold != nil {
		c.Lifecycle.IdleThreshold = *o.IdleThreshold
	}
	if o.SleepThreshold != nil {
		c.Lifecycle.SleepThreshold = *o.SleepThreshold
	}
	if o.DormantThreshold != nil {
		c.Lifecycle.DormantThreshold = *o.DormantThreshold
	}
	if o.DecayInterval != nil {
		c.Daemons.DecayInterval = *o.DecayInterval
	}
	if o.ConsolidateInt != nil {
		c.Daemons.ConsolidateInterval = *o.ConsolidateInt
	}
	if o.PruneInterval != nil {
		c.Daemons.PruneInterval = *o.PruneInterval
	}
	if o.PersistInterval != nil {
		c.Daemons.PersistInterval = *o.PersistInterval
	}
	if o.ReorgInterval != nil {
		c.Daemons.ReorgInterval = *o.ReorgInterval
	}
	if o.MaxIdleTime != nil {
		c.Worker.MaxIdleTime = *o.MaxIdleTime
	}
	if o.RegistryEnabled != nil {
		c.Registry.Enabled = *o.RegistryEnabled
	}
	if o.VectorEnabled != nil {
		c.Vector.Enabled = *o.VectorEnabled
	}
	if o.VectorModelPath != nil {
		c.Vector.ModelPath = *o.VectorModelPath
	}
	if o.VectorGPULayers != nil {
		c.Vector.GPULayers = *o.VectorGPULayers
	}
	if o.VectorAlpha != nil {
		c.Vector.Alpha = *o.VectorAlpha
	}
	if o.VectorQueryRepeat != nil {
		c.Vector.QueryRepeat = *o.VectorQueryRepeat
	}
	if o.VectorEmbedContextSize != nil {
		c.Vector.EmbedContextSize = *o.VectorEmbedContextSize
	}
	if o.AdminEnabled != nil {
		c.Admin.Enabled = *o.AdminEnabled
	}
	if o.AdminUser != nil {
		c.Admin.User = *o.AdminUser
	}
	if o.AdminPassword != nil {
		c.Admin.Password = *o.AdminPassword
	}
	if o.AllowedOrigins != nil {
		c.Security.AllowedOrigins = *o.AllowedOrigins
	}
	if o.MaxRequestBody != nil {
		c.Security.MaxRequestBody = *o.MaxRequestBody
	}
	if o.MaxNeuronContentBytes != nil {
		c.Security.MaxNeuronContentBytes = *o.MaxNeuronContentBytes
	}
	if o.TLSCert != nil {
		c.Security.TLSCert = *o.TLSCert
	}
	if o.TLSKey != nil {
		c.Security.TLSKey = *o.TLSKey
	}
}

// ---------------------------------------------------------------------------
// Lifecycle helpers
// ---------------------------------------------------------------------------

// WaitForShutdown blocks until an OS interrupt or termination signal is
// received, then cancels the provided context to initiate graceful shutdown.
func WaitForShutdown(ctx context.Context, cancel context.CancelFunc) {
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	select {
	case sig := <-sigChan:
		log.Printf("Received signal %v, initiating shutdown...", sig)
		cancel()
	case <-ctx.Done():
	}
}

// PrintBanner prints the QubicDB ASCII art banner to stdout.
func PrintBanner() {
	banner := `
   ____        __    _      ____  ____ 
  / __ \__  __/ /_  (_)____/ __ \/ __ )
 / / / / / / / __ \/ / ___/ / / / __  |
/ /_/ / /_/ / /_/ / / /__/ /_/ / /_/ / 
\___\_\__,_/_.___/_/\___/_____/_____/  
                                       
    Brain-like Recursive Memory for LLMs
    ────────────────────────────────────
`
	fmt.Print(banner)
}
