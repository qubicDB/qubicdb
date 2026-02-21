package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/qubicDB/qubicdb/pkg/core"
	"github.com/spf13/cobra"
)

// cli holds the shared state for all subcommands.
type cli struct {
	conn       *core.ConnInfo
	httpClient *http.Client
}

func main() {
	var connectStr string
	var interactive bool

	c := &cli{
		httpClient: &http.Client{Timeout: 30 * time.Second},
	}

	rootCmd := &cobra.Command{
		Use:   "qubicdb-cli",
		Short: "QubicDB CLI — admin client for QubicDB servers",
		Long:  "A command-line client for managing QubicDB instances, similar to redis-cli or psql.",
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			if connectStr == "" {
				connectStr = os.Getenv("QUBICDB_URL")
			}
			if connectStr == "" {
				connectStr = "qubicdb://localhost:6060"
			}
			info, err := core.ParseConnString(connectStr)
			if err != nil {
				return fmt.Errorf("invalid connection string: %w", err)
			}
			c.conn = info
			return nil
		},
		// When called with no subcommand, drop into interactive shell.
		RunE: func(cmd *cobra.Command, args []string) error {
			runREPL(c)
			return nil
		},
	}

	rootCmd.PersistentFlags().StringVar(&connectStr, "connect", "", "Connection string (qubicdb://[user:pass@]host[:port][/index])")
	rootCmd.Flags().BoolVarP(&interactive, "interactive", "i", false, "Start interactive shell (default when no subcommand given)")

	// ── Health ──────────────────────────────────────────────
	rootCmd.AddCommand(&cobra.Command{
		Use:   "ping",
		Short: "Check server health",
		RunE: func(cmd *cobra.Command, args []string) error {
			return c.getJSON("/health")
		},
	})

	// ── Stats ───────────────────────────────────────────────
	rootCmd.AddCommand(&cobra.Command{
		Use:   "stats",
		Short: "Show global server statistics",
		RunE: func(cmd *cobra.Command, args []string) error {
			return c.getJSON("/v1/stats")
		},
	})

	// ── Config commands ─────────────────────────────────────
	configCmd := &cobra.Command{
		Use:   "config",
		Short: "Runtime configuration management",
	}

	configCmd.AddCommand(&cobra.Command{
		Use:   "show",
		Short: "Show active server configuration",
		RunE: func(cmd *cobra.Command, args []string) error {
			return c.adminGet("/v1/config")
		},
	})

	configCmd.AddCommand(&cobra.Command{
		Use:   "get [section]",
		Short: "Show a specific config section (server, matrix, lifecycle, daemons, worker, registry, vector, admin, security)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return c.configGetSection(args[0])
		},
	})

	configSetCmd := &cobra.Command{
		Use:   "set [key] [value]",
		Short: "Set a runtime config parameter",
		Long: `Set a runtime config parameter. Supported keys:
  lifecycle.idleThreshold      (duration, e.g. "30s", "5m")
  lifecycle.sleepThreshold     (duration)
  lifecycle.dormantThreshold   (duration)
  daemons.decayInterval        (duration)
  daemons.consolidateInterval  (duration)
  daemons.pruneInterval        (duration)
  daemons.persistInterval      (duration)
  daemons.reorgInterval        (duration)
  worker.maxIdleTime           (duration)
  registry.enabled             (bool: true/false)
  matrix.maxNeurons            (int)
  security.allowedOrigins      (string)
  security.maxRequestBody      (int64, bytes)
  vector.alpha                 (float 0.0–1.0)`,
		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			return c.configSet(args[0], args[1])
		},
	}
	configCmd.AddCommand(configSetCmd)

	rootCmd.AddCommand(configCmd)

	// ── Write ───────────────────────────────────────────────
	writeCmd := &cobra.Command{
		Use:   "write [content]",
		Short: "Write a new memory (neuron)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			indexID := c.resolveIndex(cmd)
			parentID, _ := cmd.Flags().GetString("parent-id")
			metaKV, _ := cmd.Flags().GetStringToString("metadata")

			payload := map[string]any{"content": args[0]}
			if parentID != "" {
				payload["parent_id"] = parentID
			}
			if len(metaKV) > 0 {
				payload["metadata"] = metaKV
			}
			body, err := json.Marshal(payload)
			if err != nil {
				return err
			}
			return c.postJSON("/v1/write", string(body), indexID)
		},
	}
	writeCmd.Flags().String("index", "", "Index ID (overrides connection string)")
	writeCmd.Flags().String("parent-id", "", "Parent neuron ID (optional)")
	writeCmd.Flags().StringToString("metadata", nil, "Metadata key=value pairs (e.g. --metadata thread_id=conv-1,role=user)")
	rootCmd.AddCommand(writeCmd)

	// ── Search ──────────────────────────────────────────────
	searchCmd := &cobra.Command{
		Use:   "search [query]",
		Short: "Search memories",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			indexID := c.resolveIndex(cmd)
			limit, _ := cmd.Flags().GetInt("limit")
			depth, _ := cmd.Flags().GetInt("depth")
			strict, _ := cmd.Flags().GetBool("strict")
			metaKV, _ := cmd.Flags().GetStringToString("metadata")

			payload := map[string]any{
				"query": args[0],
				"depth": depth,
				"limit": limit,
			}
			if len(metaKV) > 0 {
				payload["metadata"] = metaKV
			}
			if strict {
				payload["strict"] = true
			}
			body, err := json.Marshal(payload)
			if err != nil {
				return err
			}
			return c.postJSON("/v1/search", string(body), indexID)
		},
	}
	searchCmd.Flags().Int("limit", 20, "Max results")
	searchCmd.Flags().Int("depth", 2, "Search depth")
	searchCmd.Flags().String("index", "", "Index ID")
	searchCmd.Flags().StringToString("metadata", nil, "Metadata filter key=value pairs (e.g. --metadata thread_id=conv-1)")
	searchCmd.Flags().Bool("strict", false, "Strict metadata filter — only return neurons matching ALL metadata keys")
	rootCmd.AddCommand(searchCmd)

	// ── Recall ──────────────────────────────────────────────
	recallCmd := &cobra.Command{
		Use:   "recall",
		Short: "List all memories for an index",
		RunE: func(cmd *cobra.Command, args []string) error {
			indexID := c.resolveIndex(cmd)
			return c.getJSONWithIndex("/v1/recall", indexID)
		},
	}
	recallCmd.Flags().String("index", "", "Index ID")
	rootCmd.AddCommand(recallCmd)

	// ── Read ────────────────────────────────────────────────
	readCmd := &cobra.Command{
		Use:   "read [neuron-id]",
		Short: "Read a specific memory by ID",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			indexID := c.resolveIndex(cmd)
			return c.getJSONWithIndex("/v1/read/"+args[0], indexID)
		},
	}
	readCmd.Flags().String("index", "", "Index ID")
	rootCmd.AddCommand(readCmd)

	// ── Admin commands ──────────────────────────────────────
	adminCmd := &cobra.Command{
		Use:   "admin",
		Short: "Server administration commands",
	}

	adminCmd.AddCommand(&cobra.Command{
		Use:   "indexes",
		Short: "List all active indexes",
		RunE: func(cmd *cobra.Command, args []string) error {
			return c.adminGet("/admin/indexes")
		},
	})

	adminCmd.AddCommand(&cobra.Command{
		Use:   "detail [index-id]",
		Short: "Show stats and brain state for an index",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return c.adminGet("/admin/indexes/" + args[0])
		},
	})

	adminCmd.AddCommand(&cobra.Command{
		Use:   "export [index-id]",
		Short: "Export index brain data",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return c.adminGet("/admin/indexes/" + args[0] + "/export")
		},
	})

	adminCmd.AddCommand(&cobra.Command{
		Use:   "reset [index-id]",
		Short: "Reset an index brain (clears all neurons)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return c.adminPost("/admin/indexes/"+args[0]+"/reset", "")
		},
	})

	adminCmd.AddCommand(&cobra.Command{
		Use:   "delete [index-id]",
		Short: "Delete an index completely",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return c.adminDelete("/admin/indexes/" + args[0])
		},
	})

	adminCmd.AddCommand(&cobra.Command{
		Use:   "daemons",
		Short: "Show daemon status",
		RunE: func(cmd *cobra.Command, args []string) error {
			return c.adminGet("/admin/daemons")
		},
	})

	adminCmd.AddCommand(&cobra.Command{
		Use:   "pause-daemons",
		Short: "Pause all background daemons",
		RunE: func(cmd *cobra.Command, args []string) error {
			return c.adminPost("/admin/daemons/pause", "")
		},
	})

	adminCmd.AddCommand(&cobra.Command{
		Use:   "resume-daemons",
		Short: "Resume all background daemons",
		RunE: func(cmd *cobra.Command, args []string) error {
			return c.adminPost("/admin/daemons/resume", "")
		},
	})

	adminCmd.AddCommand(&cobra.Command{
		Use:   "gc",
		Short: "Force garbage collection",
		RunE: func(cmd *cobra.Command, args []string) error {
			return c.adminPost("/admin/gc", "")
		},
	})

	adminCmd.AddCommand(&cobra.Command{
		Use:   "persist",
		Short: "Force flush all brains to disk",
		RunE: func(cmd *cobra.Command, args []string) error {
			return c.adminPost("/admin/persist", "")
		},
	})

	// ── Registry commands ───────────────────────────────────
	registryCmd := &cobra.Command{
		Use:   "registry",
		Short: "UUID registry management",
	}

	registryCmd.AddCommand(&cobra.Command{
		Use:   "list",
		Short: "List registered UUIDs",
		RunE: func(cmd *cobra.Command, args []string) error {
			return c.getJSON("/v1/registry")
		},
	})

	registryCmd.AddCommand(&cobra.Command{
		Use:   "create [uuid]",
		Short: "Register a new UUID",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			body := fmt.Sprintf(`{"uuid":%q}`, args[0])
			return c.postJSON("/v1/registry", body, "")
		},
	})

	registryCmd.AddCommand(&cobra.Command{
		Use:   "delete [uuid]",
		Short: "Unregister a UUID",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return c.deleteJSON("/v1/registry/"+args[0], "")
		},
	})

	adminCmd.AddCommand(registryCmd)
	rootCmd.AddCommand(adminCmd)

	// --interactive flag explicitly requested
	rootCmd.PreRunE = func(cmd *cobra.Command, args []string) error {
		if interactive {
			runREPL(c)
			os.Exit(0)
		}
		return nil
	}

	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

// ── HTTP helpers ────────────────────────────────────────────

func (c *cli) resolveIndex(cmd *cobra.Command) string {
	if idx, _ := cmd.Flags().GetString("index"); idx != "" {
		return idx
	}
	return c.conn.IndexID
}

func (c *cli) doRequest(method, path, body, indexID string, admin bool) error {
	url := c.conn.BaseURL() + path

	var bodyReader io.Reader
	if body != "" {
		bodyReader = strings.NewReader(body)
	}

	req, err := http.NewRequest(method, url, bodyReader)
	if err != nil {
		return err
	}

	req.Header.Set("Content-Type", "application/json")

	if indexID != "" {
		req.Header.Set("X-Index-ID", indexID)
	}

	if admin && c.conn.User != "" {
		req.SetBasicAuth(c.conn.User, c.conn.Password)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("connection failed: %w", err)
	}
	defer resp.Body.Close()

	data, _ := io.ReadAll(resp.Body)

	if resp.StatusCode >= 400 {
		fmt.Fprintf(os.Stderr, "Error %d: %s\n", resp.StatusCode, string(data))
		return fmt.Errorf("request failed with status %d", resp.StatusCode)
	}

	// Pretty-print JSON
	var prettyJSON map[string]any
	if err := json.Unmarshal(data, &prettyJSON); err == nil {
		out, _ := json.MarshalIndent(prettyJSON, "", "  ")
		fmt.Println(string(out))
	} else {
		// Try as array
		var arr []any
		if err := json.Unmarshal(data, &arr); err == nil {
			out, _ := json.MarshalIndent(arr, "", "  ")
			fmt.Println(string(out))
		} else {
			fmt.Println(string(data))
		}
	}

	return nil
}

func (c *cli) getJSON(path string) error {
	return c.doRequest("GET", path, "", "", false)
}

func (c *cli) getJSONWithIndex(path, indexID string) error {
	return c.doRequest("GET", path, "", indexID, false)
}

func (c *cli) postJSON(path, body, indexID string) error {
	return c.doRequest("POST", path, body, indexID, false)
}

func (c *cli) deleteJSON(path, indexID string) error {
	return c.doRequest("DELETE", path, "", indexID, false)
}

func (c *cli) adminGet(path string) error {
	return c.doRequest("GET", path, "", "", true)
}

func (c *cli) adminPost(path, body string) error {
	return c.doRequest("POST", path, body, "", true)
}

func (c *cli) adminDelete(path string) error {
	return c.doRequest("DELETE", path, "", "", true)
}

// silentGet and silentAdminGet perform a request without printing output —
// used for connection/auth verification in the REPL startup.
func (c *cli) silentGet(path string) error {
	return c.doSilentRequest("GET", path, "", "", false)
}

func (c *cli) silentAdminGet(path string) error {
	return c.doSilentRequest("GET", path, "", "", true)
}

func (c *cli) doSilentRequest(method, path, body, indexID string, admin bool) error {
	url := c.conn.BaseURL() + path
	req, err := http.NewRequest(method, url, strings.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if indexID != "" {
		req.Header.Set("X-Index-ID", indexID)
	}
	if admin && c.conn.User != "" {
		req.SetBasicAuth(c.conn.User, c.conn.Password)
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("connection failed: %w", err)
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, resp.Body)
	if resp.StatusCode >= 400 {
		return fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	return nil
}

// ── Config helpers ──────────────────────────────────────────

func (c *cli) configGetSection(section string) error {
	url := c.conn.BaseURL() + "/v1/config"
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if c.conn.User != "" {
		req.SetBasicAuth(c.conn.User, c.conn.Password)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("connection failed: %w", err)
	}
	defer resp.Body.Close()

	var full map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&full); err != nil {
		return fmt.Errorf("failed to decode config: %w", err)
	}

	val, ok := full[section]
	if !ok {
		valid := make([]string, 0, len(full))
		for k := range full {
			valid = append(valid, k)
		}
		return fmt.Errorf("unknown section %q, valid: %v", section, valid)
	}

	out, _ := json.MarshalIndent(val, "", "  ")
	fmt.Println(string(out))
	return nil
}

func (c *cli) configSet(key, value string) error {
	parts := strings.SplitN(key, ".", 2)
	if len(parts) != 2 {
		return fmt.Errorf("key must be section.field (e.g. daemons.decayInterval)")
	}
	section, field := parts[0], parts[1]

	// Build the JSON patch — type depends on the field
	var fieldJSON string
	switch {
	case field == "enabled": // bool
		if value != "true" && value != "false" {
			return fmt.Errorf("boolean field %q requires 'true' or 'false'", key)
		}
		fieldJSON = fmt.Sprintf(`%q:%s`, field, value)
	case field == "maxNeurons" || field == "maxRequestBody": // numeric
		fieldJSON = fmt.Sprintf(`%q:%s`, field, value)
	case field == "alpha": // float
		fieldJSON = fmt.Sprintf(`%q:%s`, field, value)
	default: // string (durations, origins, etc.)
		fieldJSON = fmt.Sprintf(`%q:%q`, field, value)
	}

	body := fmt.Sprintf(`{%q:{%s}}`, section, fieldJSON)
	return c.adminPost("/v1/config", body)
}
