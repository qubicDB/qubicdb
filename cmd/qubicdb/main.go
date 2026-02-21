package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"github.com/qubicDB/qubicdb/pkg/api"
	"github.com/qubicDB/qubicdb/pkg/concurrency"
	"github.com/qubicDB/qubicdb/pkg/core"
	"github.com/qubicDB/qubicdb/pkg/daemon"
	"github.com/qubicDB/qubicdb/pkg/lifecycle"
	"github.com/qubicDB/qubicdb/pkg/persistence"
	"github.com/qubicDB/qubicdb/pkg/registry"
	"github.com/qubicDB/qubicdb/pkg/sentiment"
	"github.com/qubicDB/qubicdb/pkg/vector"
)

func main() {
	var cliOverrides core.CLIOverrides

	rootCmd := &cobra.Command{
		Use:   "qubicdb",
		Short: "QubicDB - Brain-like Recursive Memory for LLMs",
		Long:  "A persistent, per-user memory database that uses organic neural matrices, Hebbian learning, and sleep/consolidation cycles.",
		RunE: func(cmd *cobra.Command, args []string) error {
			return run(cmd.Flags(), &cliOverrides)
		},
		SilenceUsage: true,
	}

	// CLI flags - highest priority in the config hierarchy.
	f := rootCmd.Flags()

	cliOverrides.ConfigPath = f.StringP("config", "f", "", "Path to YAML config file (overrides QUBICDB_CONFIG env)")
	cliOverrides.HTTPAddr = f.String("http-addr", "", "HTTP listen address")
	cliOverrides.DataPath = f.String("data-path", "", "Data directory for .nrdb files")
	cliOverrides.Compress = f.Bool("compress", false, "Enable msgpack compression")
	cliOverrides.MaxNeurons = f.Int("max-neurons", 0, "Maximum neurons per brain")
	cliOverrides.RegistryEnabled = f.Bool("registry", false, "Enable UUID registry")
	cliOverrides.VectorEnabled = f.Bool("vector", false, "Enable vector embedding layer")
	cliOverrides.VectorModelPath = f.String("vector-model", "", "Path to GGUF embedding model")
	cliOverrides.VectorGPULayers = f.Int("vector-gpu-layers", 0, "GPU layers for embedding model")
	cliOverrides.VectorAlpha = f.Float64("vector-alpha", 0.6, "Vector score weight in hybrid search (0.0-1.0)")
	cliOverrides.VectorQueryRepeat = f.Int("vector-query-repeat", 2, "Query repetition count for embedding (1=off, 2=repeat, 3=repeat×3)")
	cliOverrides.VectorEmbedContextSize = f.Uint32("vector-embed-context-size", 512, "llama.cpp context size for embedding")

	// Admin flags
	cliOverrides.AdminEnabled = f.Bool("admin", false, "Enable admin endpoints")
	cliOverrides.AdminUser = f.String("admin-user", "", "Admin username")
	cliOverrides.AdminPassword = f.String("admin-password", "", "Admin password")

	// Security flags
	cliOverrides.AllowedOrigins = f.String("allowed-origins", "", "CORS allowed origins (comma-separated, \"*\" for all)")
	cliOverrides.MaxNeuronContentBytes = f.Int64("max-neuron-content-bytes", 0, "Maximum neuron content payload size in bytes")
	cliOverrides.TLSCert = f.String("tls-cert", "", "Path to TLS certificate file")
	cliOverrides.TLSKey = f.String("tls-key", "", "Path to TLS private key file")

	// Matrix flags
	cliOverrides.MinDimension = f.Int("min-dimension", 0, "Initial matrix dimensionality")
	cliOverrides.MaxDimension = f.Int("max-dimension", 0, "Maximum matrix dimensionality")

	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

// run implements the server startup sequence after CLI flags are parsed.
func run(flags *pflag.FlagSet, cliOverrides *core.CLIOverrides) error {
	core.PrintBanner()

	// Resolve config path: --config flag > QUBICDB_CONFIG env var
	configPath := ""
	if cliOverrides.ConfigPath != nil && *cliOverrides.ConfigPath != "" {
		configPath = *cliOverrides.ConfigPath
	} else {
		configPath = os.Getenv("QUBICDB_CONFIG")
	}

	// Load config through hierarchy: defaults -> YAML -> env vars
	cfg, err := core.LoadConfig(configPath)
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	// Apply CLI flag overrides (only flags that were explicitly set)
	applyExplicitFlags(flags, cfg, cliOverrides)

	if err := cfg.Validate(); err != nil {
		return fmt.Errorf("invalid config: %w", err)
	}
	if err := core.SetMaxNeuronContentBytes(cfg.Security.MaxNeuronContentBytes); err != nil {
		return fmt.Errorf("invalid neuron content limit: %w", err)
	}

	log.Printf("Data path: %s", cfg.Storage.DataPath)
	log.Printf("HTTP: %s", cfg.Server.HTTPAddr)

	// Initialize persistence store
	store, err := persistence.NewStoreWithDurability(
		cfg.Storage.DataPath,
		cfg.Storage.Compress,
		persistence.DurabilityConfig{
			WALEnabled:                 cfg.Storage.WALEnabled,
			FsyncPolicy:                cfg.Storage.FsyncPolicy,
			FsyncInterval:              cfg.Storage.FsyncInterval,
			ChecksumValidationInterval: cfg.Storage.ChecksumValidationInterval,
			StartupRepair:              cfg.Storage.StartupRepair,
		},
	)
	if err != nil {
		return fmt.Errorf("failed to initialize store: %w", err)
	}
	log.Println("Persistence store initialized")

	// Initialize UUID registry
	reg, err := registry.NewStore(cfg.Storage.DataPath)
	if err != nil {
		return fmt.Errorf("failed to initialize registry: %w", err)
	}
	log.Printf("UUID registry initialized (%d entries)", reg.Count())

	// Initialize matrix bounds from config
	bounds := core.MatrixBounds{
		MinDimension: cfg.Matrix.MinDimension,
		MaxDimension: cfg.Matrix.MaxDimension,
		MinNeurons:   0,
		MaxNeurons:   cfg.Matrix.MaxNeurons,
	}

	// Initialize worker pool
	pool := concurrency.NewWorkerPool(store, bounds)
	log.Println("Worker pool initialized")

	// Initialize vector layer (optional)
	var vectorizer *vector.Vectorizer
	if cfg.Vector.Enabled {
		if cfg.Vector.ModelPath == "" {
			log.Println("⚠ Vector layer enabled but no model path configured, skipping")
		} else if !vector.IsLibraryAvailable() {
			log.Println("⚠ Vector layer enabled but llama.cpp library not found, skipping")
			log.Println(vector.ResolveLibraryError(vector.ErrLibraryNotFound))
		} else {
			v, err := vector.NewVectorizer(cfg.Vector.ModelPath, cfg.Vector.GPULayers, cfg.Vector.EmbedContextSize)
			if err != nil {
				log.Printf("⚠ Vector layer failed to initialize: %v", err)
			} else {
				vectorizer = v
				pool.SetVectorizerWithRepeat(vectorizer, cfg.Vector.Alpha, cfg.Vector.QueryRepeat)
				log.Printf("Vector layer initialized (model=%s, dims=%d, gpu=%d, alpha=%.2f, query_repeat=%d)",
					cfg.Vector.ModelPath, vectorizer.EmbedDim(), cfg.Vector.GPULayers, cfg.Vector.Alpha, cfg.Vector.QueryRepeat)
			}
		}
	} else {
		log.Println("Vector layer disabled (enable with --vector or QUBICDB_VECTOR_ENABLED=true)")
	}

	// Initialize sentiment layer (always on — zero external dependencies)
	sentimentAnalyzer := sentiment.New()
	pool.SetSentimentAnalyzer(sentimentAnalyzer)
	log.Println("Sentiment layer initialized (VADER, 6 basic emotions)")

	// Initialize lifecycle manager
	lm := lifecycle.NewManager()
	lm.SetCallbacks(
		func(indexID core.IndexID) {
			log.Printf("User %s entering sleep state", indexID)
		},
		func(indexID core.IndexID) {
			log.Printf("User %s sleep completed", indexID)
		},
		func(indexID core.IndexID) {
			log.Printf("User %s going dormant, persisting...", indexID)
			pool.Evict(indexID)
		},
		func(indexID core.IndexID) {
			log.Printf("User %s waking up", indexID)
		},
	)
	lm.StartMonitor(10 * time.Second)
	log.Println("Lifecycle manager initialized")

	// Initialize daemon manager with config-driven intervals
	daemons := daemon.NewDaemonManager(pool, lm, store)
	daemons.SetIntervals(
		cfg.Daemons.DecayInterval,
		cfg.Daemons.ConsolidateInterval,
		cfg.Daemons.PruneInterval,
		cfg.Daemons.PersistInterval,
		cfg.Daemons.ReorgInterval,
	)
	daemons.Start()
	log.Println("Background daemons started")

	// Start persistence flush worker
	flushStop := store.StartFlushWorker(cfg.Daemons.PersistInterval)
	checksumStop := store.StartChecksumValidationWorker(cfg.Storage.ChecksumValidationInterval)

	// Initialize HTTP server
	httpServer := api.NewServer(cfg.Server.HTTPAddr, pool, lm, reg, cfg)
	httpServer.SetDaemonManager(daemons)

	// Create context for graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())

	// Start servers
	go func() {
		if err := httpServer.Start(); err != nil {
			log.Printf("HTTP server error: %v", err)
		}
	}()

	log.Println("QubicDB is ready!")
	log.Println("--------------------------------------------")

	// Wait for shutdown signal
	core.WaitForShutdown(ctx, cancel)

	log.Println("Initiating graceful shutdown...")

	// Shutdown sequence
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer shutdownCancel()

	if err := httpServer.Stop(shutdownCtx); err != nil {
		log.Printf("HTTP shutdown error: %v", err)
	}
	daemons.Stop()
	lm.Stop()
	close(flushStop)
	if checksumStop != nil {
		close(checksumStop)
	}

	if err := pool.Shutdown(); err != nil {
		log.Printf("Pool shutdown error: %v", err)
	}

	if err := store.FlushAll(); err != nil {
		log.Printf("Final flush error: %v", err)
	}

	if vectorizer != nil {
		vectorizer.Close()
		log.Println("Vector layer closed")
	}

	log.Println("QubicDB shutdown complete")
	return nil
}

// applyExplicitFlags applies only the CLI flags that were explicitly set
// by the user on the command line. Unset flags are ignored so they do not
// override values resolved from YAML or environment variables.
func applyExplicitFlags(flags *pflag.FlagSet, cfg *core.Config, o *core.CLIOverrides) {
	overrides := core.CLIOverrides{}

	if flags.Changed("http-addr") {
		overrides.HTTPAddr = o.HTTPAddr
	}
	if flags.Changed("data-path") {
		overrides.DataPath = o.DataPath
	}
	if flags.Changed("compress") {
		overrides.Compress = o.Compress
	}
	if flags.Changed("max-neurons") {
		overrides.MaxNeurons = o.MaxNeurons
	}
	if flags.Changed("registry") {
		overrides.RegistryEnabled = o.RegistryEnabled
	}
	if flags.Changed("vector") {
		overrides.VectorEnabled = o.VectorEnabled
	}
	if flags.Changed("vector-model") {
		overrides.VectorModelPath = o.VectorModelPath
	}
	if flags.Changed("vector-gpu-layers") {
		overrides.VectorGPULayers = o.VectorGPULayers
	}
	if flags.Changed("vector-alpha") {
		overrides.VectorAlpha = o.VectorAlpha
	}
	if flags.Changed("vector-query-repeat") {
		overrides.VectorQueryRepeat = o.VectorQueryRepeat
	}
	if flags.Changed("vector-embed-context-size") {
		overrides.VectorEmbedContextSize = o.VectorEmbedContextSize
	}
	if flags.Changed("admin") {
		overrides.AdminEnabled = o.AdminEnabled
	}
	if flags.Changed("admin-user") {
		overrides.AdminUser = o.AdminUser
	}
	if flags.Changed("admin-password") {
		overrides.AdminPassword = o.AdminPassword
	}
	if flags.Changed("allowed-origins") {
		overrides.AllowedOrigins = o.AllowedOrigins
	}
	if flags.Changed("max-neuron-content-bytes") {
		overrides.MaxNeuronContentBytes = o.MaxNeuronContentBytes
	}
	if flags.Changed("tls-cert") {
		overrides.TLSCert = o.TLSCert
	}
	if flags.Changed("tls-key") {
		overrides.TLSKey = o.TLSKey
	}
	if flags.Changed("min-dimension") {
		overrides.MinDimension = o.MinDimension
	}
	if flags.Changed("max-dimension") {
		overrides.MaxDimension = o.MaxDimension
	}

	cfg.ApplyCLIOverrides(&overrides)
}
