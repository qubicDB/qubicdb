package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

const replHelp = `
QubicDB Interactive Shell — available commands:

  Brain ops:
    ping                              Check server health
    stats                             Global statistics
    write <content>                   Write a memory (neuron)
      write <content> --metadata key=val,key2=val2
      write <content> --parent-id <neuron-id>
    search <query>                    Associative search
      search <query> --depth N --limit N
      search <query> --metadata key=val --strict
    recall                            List all neurons for active index
    read <neuron-id>                  Read a specific neuron
    context <cue>                     Assemble LLM context

  Index:
    \index                            Show active index
    \index <id>                       Switch active index

  Admin (requires credentials in connection string):
    indexes                           List all active indexes
    detail <index-id>                 Show index stats + brain state
    reset <index-id>                  Wipe neurons (keep index registered)
    delete <index-id>                 Delete index completely
    export <index-id>                 Export brain data
    wake <index-id>                   Force brain to Active state
    sleep <index-id>                  Force brain to Sleeping state
    daemons                           Show daemon status
    pause-daemons                     Pause all background daemons
    resume-daemons                    Resume all background daemons
    gc                                Force garbage collection
    persist                           Flush all brains to disk

  Config (requires credentials):
    config                            Show full runtime config
    config get <section>              Show one section
    config set <key> <value>          Set a runtime parameter
      e.g. config set daemons.decayInterval 30s

  Registry:
    registry                          List registered UUIDs
    registry create <uuid>            Register a new UUID
    registry delete <uuid>            Unregister a UUID

  Shell:
    \help                             Show this help
    \index [id]                       Show/switch active index
    \status                           Show connection info
    \quit  (or exit, quit, Ctrl-D)    Exit
`

// runREPL starts the interactive shell. conn and httpClient are already
// initialised by the cobra PersistentPreRunE.
func runREPL(c *cli) {
	indexInfo := ""
	if c.conn.IndexID != "" {
		indexInfo = fmt.Sprintf(", index: %s", c.conn.IndexID)
	}

	// Step 1: verify server is reachable (silent — no output).
	if err := c.silentGet("/health"); err != nil {
		fmt.Fprintf(os.Stderr, "error: cannot reach %s — %v\n", c.conn.BaseURL(), err)
		os.Exit(1)
	}

	// Step 2: if credentials provided, verify them (silent check).
	if c.conn.User != "" {
		if err := c.silentAdminGet("/admin/daemons"); err != nil {
			fmt.Fprintf(os.Stderr, "error: authentication failed for user %q — check your credentials\n", c.conn.User)
			os.Exit(1)
		}
	}

	fmt.Printf("Connected to QubicDB at %s%s\nType \\help for commands, \\quit to exit.\n\n",
		c.conn.BaseURL(), indexInfo)

	activeIndex := c.conn.IndexID
	scanner := bufio.NewScanner(os.Stdin)

	for {
		prompt := "qubicdb"
		if activeIndex != "" {
			prompt = fmt.Sprintf("qubicdb[%s]", activeIndex)
		}
		fmt.Printf("%s> ", prompt)

		if !scanner.Scan() {
			fmt.Println()
			break
		}
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		if done := dispatchREPL(c, line, &activeIndex); done {
			fmt.Println("Bye.")
			break
		}
	}
}

// dispatchREPL parses and executes one REPL line.
// Returns true when the user wants to quit.
func dispatchREPL(c *cli, line string, activeIndex *string) bool {
	parts := tokenize(line)
	if len(parts) == 0 {
		return false
	}
	cmd := strings.ToLower(parts[0])

	switch cmd {
	// ── Quit ────────────────────────────────────────────────
	case `\quit`, `\q`, "exit", "quit":
		return true

	// ── Help ────────────────────────────────────────────────
	case `\help`, `\h`, "help":
		fmt.Print(replHelp)

	// ── Index switch ────────────────────────────────────────
	case `\index`:
		if len(parts) < 2 {
			if *activeIndex == "" {
				fmt.Println("no active index (use \\index <id> to set one)")
			} else {
				fmt.Printf("active index: %s\n", *activeIndex)
			}
		} else {
			*activeIndex = parts[1]
			fmt.Printf("switched to index: %s\n", *activeIndex)
		}

	// ── Status ──────────────────────────────────────────────
	case `\status`:
		fmt.Printf("server:  %s\n", c.conn.BaseURL())
		if c.conn.User != "" {
			fmt.Printf("user:    %s\n", c.conn.User)
		}
		fmt.Printf("index:   %s\n", func() string {
			if *activeIndex == "" {
				return "(none)"
			}
			return *activeIndex
		}())

	// ── Brain ops ───────────────────────────────────────────
	case "ping":
		if err := c.getJSON("/health"); err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
		}

	case "stats":
		c.getJSON("/v1/stats") //nolint:errcheck

	case "write":
		replWrite(c, parts[1:], activeIndex)

	case "search":
		replSearch(c, parts[1:], activeIndex)

	case "recall":
		idx := replResolveIndex(parts[1:], activeIndex)
		c.getJSONWithIndex("/v1/recall", idx) //nolint:errcheck

	case "read":
		if len(parts) < 2 {
			fmt.Fprintln(os.Stderr, "usage: read <neuron-id>")
		} else {
			c.getJSONWithIndex("/v1/read/"+parts[1], *activeIndex) //nolint:errcheck
		}

	case "context":
		replContext(c, parts[1:], activeIndex)

	// ── Admin ───────────────────────────────────────────────
	case "indexes":
		c.adminGet("/admin/indexes") //nolint:errcheck

	case "detail":
		if len(parts) < 2 {
			fmt.Fprintln(os.Stderr, "usage: detail <index-id>")
		} else {
			c.adminGet("/admin/indexes/" + parts[1]) //nolint:errcheck
		}

	case "reset":
		if len(parts) < 2 {
			fmt.Fprintln(os.Stderr, "usage: reset <index-id>")
		} else {
			c.adminPost("/admin/indexes/"+parts[1]+"/reset", "") //nolint:errcheck
		}

	case "delete":
		if len(parts) < 2 {
			fmt.Fprintln(os.Stderr, "usage: delete <index-id>")
		} else {
			c.adminDelete("/admin/indexes/" + parts[1]) //nolint:errcheck
		}

	case "export":
		if len(parts) < 2 {
			fmt.Fprintln(os.Stderr, "usage: export <index-id>")
		} else {
			c.adminGet("/admin/indexes/" + parts[1] + "/export") //nolint:errcheck
		}

	case "wake":
		if len(parts) < 2 {
			fmt.Fprintln(os.Stderr, "usage: wake <index-id>")
		} else {
			c.adminPost("/admin/indexes/"+parts[1]+"/wake", "") //nolint:errcheck
		}

	case "sleep":
		if len(parts) < 2 {
			fmt.Fprintln(os.Stderr, "usage: sleep <index-id>")
		} else {
			c.adminPost("/admin/indexes/"+parts[1]+"/sleep", "") //nolint:errcheck
		}

	case "daemons":
		c.adminGet("/admin/daemons") //nolint:errcheck

	case "pause-daemons":
		c.adminPost("/admin/daemons/pause", "") //nolint:errcheck

	case "resume-daemons":
		c.adminPost("/admin/daemons/resume", "") //nolint:errcheck

	case "gc":
		c.adminPost("/admin/gc", "") //nolint:errcheck

	case "persist":
		c.adminPost("/admin/persist", "") //nolint:errcheck

	// ── Config ──────────────────────────────────────────────
	case "config":
		if len(parts) < 2 {
			c.adminGet("/v1/config") //nolint:errcheck
		} else {
			switch parts[1] {
			case "show":
				c.adminGet("/v1/config") //nolint:errcheck
			case "get":
				if len(parts) < 3 {
					fmt.Fprintln(os.Stderr, "usage: config get <section>")
				} else {
					c.configGetSection(parts[2]) //nolint:errcheck
				}
			case "set":
				if len(parts) < 4 {
					fmt.Fprintln(os.Stderr, "usage: config set <key> <value>")
				} else {
					c.configSet(parts[2], parts[3]) //nolint:errcheck
				}
			default:
				fmt.Fprintf(os.Stderr, "unknown config subcommand %q — use show/get/set\n", parts[1])
			}
		}

	// ── Registry ────────────────────────────────────────────
	case "registry":
		if len(parts) < 2 {
			c.getJSON("/v1/registry") //nolint:errcheck
		} else {
			switch parts[1] {
			case "list":
				c.getJSON("/v1/registry") //nolint:errcheck
			case "create":
				if len(parts) < 3 {
					fmt.Fprintln(os.Stderr, "usage: registry create <uuid>")
				} else {
					body := fmt.Sprintf(`{"uuid":%q}`, parts[2])
					c.postJSON("/v1/registry", body, "") //nolint:errcheck
				}
			case "delete":
				if len(parts) < 3 {
					fmt.Fprintln(os.Stderr, "usage: registry delete <uuid>")
				} else {
					c.deleteJSON("/v1/registry/"+parts[2], "") //nolint:errcheck
				}
			default:
				fmt.Fprintf(os.Stderr, "unknown registry subcommand %q — use list/create/delete\n", parts[1])
			}
		}

	default:
		fmt.Fprintf(os.Stderr, "unknown command %q — type \\help for available commands\n", cmd)
	}

	return false
}

// ── REPL command helpers ─────────────────────────────────────

func replWrite(c *cli, args []string, activeIndex *string) {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "usage: write <content> [--index <id>] [--metadata key=val,...] [--parent-id <id>]")
		return
	}
	content := args[0]
	idx := *activeIndex
	var parentID string
	meta := map[string]string{}

	for i := 1; i < len(args); i++ {
		switch args[i] {
		case "--index", "-i":
			if i+1 < len(args) {
				i++
				idx = args[i]
			}
		case "--parent-id":
			if i+1 < len(args) {
				i++
				parentID = args[i]
			}
		case "--metadata", "-m":
			if i+1 < len(args) {
				i++
				for _, kv := range strings.Split(args[i], ",") {
					if eq := strings.IndexByte(kv, '='); eq > 0 {
						meta[strings.TrimSpace(kv[:eq])] = strings.TrimSpace(kv[eq+1:])
					}
				}
			}
		}
	}

	payload := map[string]any{"content": content}
	if parentID != "" {
		payload["parent_id"] = parentID
	}
	if len(meta) > 0 {
		payload["metadata"] = meta
	}
	body, _ := json.Marshal(payload)
	c.postJSON("/v1/write", string(body), idx) //nolint:errcheck
}

func replSearch(c *cli, args []string, activeIndex *string) {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "usage: search <query> [--index <id>] [--depth N] [--limit N] [--metadata key=val,...] [--strict]")
		return
	}
	query := args[0]
	idx := *activeIndex
	depth := 2
	limit := 20
	strict := false
	meta := map[string]string{}

	for i := 1; i < len(args); i++ {
		switch args[i] {
		case "--index", "-i":
			if i+1 < len(args) {
				i++
				idx = args[i]
			}
		case "--depth":
			if i+1 < len(args) {
				i++
				fmt.Sscanf(args[i], "%d", &depth)
			}
		case "--limit":
			if i+1 < len(args) {
				i++
				fmt.Sscanf(args[i], "%d", &limit)
			}
		case "--strict":
			strict = true
		case "--metadata", "-m":
			if i+1 < len(args) {
				i++
				for _, kv := range strings.Split(args[i], ",") {
					if eq := strings.IndexByte(kv, '='); eq > 0 {
						meta[strings.TrimSpace(kv[:eq])] = strings.TrimSpace(kv[eq+1:])
					}
				}
			}
		}
	}

	payload := map[string]any{
		"query": query,
		"depth": depth,
		"limit": limit,
	}
	if len(meta) > 0 {
		payload["metadata"] = meta
	}
	if strict {
		payload["strict"] = true
	}
	body, _ := json.Marshal(payload)
	c.postJSON("/v1/search", string(body), idx) //nolint:errcheck
}

func replContext(c *cli, args []string, activeIndex *string) {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "usage: context <cue> [--max-tokens N] [--depth N]")
		return
	}
	cue := args[0]
	idx := *activeIndex
	maxTokens := 2000
	depth := 2

	for i := 1; i < len(args); i++ {
		switch args[i] {
		case "--max-tokens":
			if i+1 < len(args) {
				i++
				fmt.Sscanf(args[i], "%d", &maxTokens)
			}
		case "--depth":
			if i+1 < len(args) {
				i++
				fmt.Sscanf(args[i], "%d", &depth)
			}
		case "--index", "-i":
			if i+1 < len(args) {
				i++
				idx = args[i]
			}
		}
	}

	payload := map[string]any{"cue": cue, "maxTokens": maxTokens, "depth": depth}
	body, _ := json.Marshal(payload)
	c.postJSON("/v1/context", string(body), idx) //nolint:errcheck
}

func replResolveIndex(args []string, activeIndex *string) string {
	for i := 0; i < len(args); i++ {
		if (args[i] == "--index" || args[i] == "-i") && i+1 < len(args) {
			return args[i+1]
		}
	}
	return *activeIndex
}

// tokenize splits a line into tokens respecting quoted strings.
func tokenize(line string) []string {
	var tokens []string
	var cur strings.Builder
	inQuote := false
	quoteChar := rune(0)

	for _, ch := range line {
		switch {
		case inQuote:
			if ch == quoteChar {
				inQuote = false
			} else {
				cur.WriteRune(ch)
			}
		case ch == '"' || ch == '\'':
			inQuote = true
			quoteChar = ch
		case ch == ' ' || ch == '\t':
			if cur.Len() > 0 {
				tokens = append(tokens, cur.String())
				cur.Reset()
			}
		default:
			cur.WriteRune(ch)
		}
	}
	if cur.Len() > 0 {
		tokens = append(tokens, cur.String())
	}
	return tokens
}
