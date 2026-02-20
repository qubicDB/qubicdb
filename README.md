# QubicDB

**A brain-like recursive memory system for LLM applications**

[![Docker Hub](https://img.shields.io/docker/v/qubicdb/qubicdb?label=Docker%20Hub&logo=docker&color=0db7ed)](https://hub.docker.com/r/qubicdb/qubicdb)
[![Docker Pulls](https://img.shields.io/docker/pulls/qubicdb/qubicdb?color=0db7ed)](https://hub.docker.com/r/qubicdb/qubicdb)

```
   ____        __    _      ____  ____
  / __ \__  __/ /_  (_)____/ __ \/ __ )
 / / / / / / / __ \/ / ___/ / / / __  |
/ /_/ / /_/ / /_/ / / /__/ /_/ / /_/ /
\___\_\__,_/_.___/_/\___/_____/_____/
```

QubicDB is an organic memory engine inspired by biological brain mechanics. It creates an isolated, self-organizing, Hebbian-strengthened brain instance per index.

---

## Quickstart

```bash
# Pull from Docker Hub
docker pull qubicdb/qubicdb:latest

# Run
docker run -d \
  -p 6060:6060 \
  -v $(pwd)/data:/app/data \
  -e QUBICDB_ADMIN_USER=admin \
  -e QUBICDB_ADMIN_PASSWORD=changeme \
  qubicdb/qubicdb:1.0.0
```

Docker Hub: [hub.docker.com/r/qubicdb/qubicdb](https://hub.docker.com/r/qubicdb/qubicdb)

---

## Features

| Feature | Description |
|---------|-------------|
| **Organic N-Dimensional Matrix** | Dynamically expands/shrinks memory space |
| **Hebbian Learning** | Forms and strengthens synapses between co-activated neurons |
| **Fractal Clustering** | Self-similar topology: dense cores → loose shells → separated clusters |
| **Lifecycle States** | Active → Idle → Sleeping → Dormant transitions |
| **Sleep Consolidation** | Automatic reorganization during inactivity |
| **Per-Index Isolation** | Dedicated goroutine and matrix per index |
| **Hybrid Search** | Lexical + GGUF vector scoring, α-weighted, SIMD cosine similarity |
| **Sentiment Layer** | VADER-based emotion labeling (6 Ekman emotions) with search boost |
| **Neuron Metadata** | Optional `map[string]string` for grouping by `thread_id`, `role`, `source`, etc. |
| **Metadata Search** | Soft boost (default) or strict filter by metadata key-value pairs |
| **Brain-like API** | write / read / search / recall / context |
| **MongoDB-like Query API** | find, count, search, stats operations |
| **LLM Context Assembly** | Token-aware, spread-activation based context |
| **UUID Registry** | Index registration and access guard |
| **MCP Endpoint** | Streamable HTTP MCP with tool allowlist and rate limiter |
| **CLI + Config Hierarchy** | cobra/pflag with CLI > YAML > env > defaults |

---

## Architecture

```
┌──────────────────────────────────────────────────────────────┐
│                       QubicDB Service                        │
├──────────────────────────────────────────────────────────────┤
│                                                              │
│  ┌────────────┐                  ┌────────────┐             │
│  │ HTTP API   │                  │ Protocol   │             │
│  │  :6060     │                  │ (Command)  │             │
│  └─────┬──────┘                  └─────┬──────┘             │
│        └────────────────────────────────┘                     │
│                        ▼                                     │
│  ┌────────────────────────────────────────────────────────┐  │
│  │                   Worker Pool                          │  │
│  │  ┌─────────┐  ┌─────────┐  ┌─────────┐                │  │
│  │  │ Index A │  │ Index B │  │ Index C │  ...           │  │
│  │  │ Worker  │  │ Worker  │  │ Worker  │                │  │
│  │  └────┬────┘  └────┬────┘  └────┬────┘                │  │
│  │       ▼            ▼            ▼                      │  │
│  │  ┌─────────┐  ┌─────────┐  ┌─────────┐                │  │
│  │  │ Matrix  │  │ Matrix  │  │ Matrix  │ (in-memory)    │  │
│  │  │ Engine  │  │ Engine  │  │ Engine  │                │  │
│  │  └─────────┘  └─────────┘  └─────────┘                │  │
│  └────────────────────────────────────────────────────────┘  │
│                        │                                     │
│  ┌─────────────────────┼──────────────────────────────────┐  │
│  │            Background Daemons                          │  │
│  │  [Decay] [Consolidate] [Prune] [Persist] [Reorg]      │  │
│  └─────────────────────┼──────────────────────────────────┘  │
│                        ▼                                     │
│  ┌────────────────────────────────────────────────────────┐  │
│  │               Persistence Layer (.nrdb)                │  │
│  └────────────────────────────────────────────────────────┘  │
└──────────────────────────────────────────────────────────────┘
```

---

## Quick Start

```bash
# Build and run
go mod download
go run ./cmd/qubicdb

# Or with CLI flags
go run ./cmd/qubicdb --http-addr :8080 --data-path ./mydata --registry

# Or with YAML configuration
go run ./cmd/qubicdb --config ./qubicdb.yaml
```

---

## API Reference

Authoritative API docs:

- OpenAPI spec: `./openapi.yaml`
- Detailed operations/config/mechanics reference: `./docs/API.md`

All index-scoped endpoints require `X-Index-ID` header or `index_id` query parameter.

### Brain-like Endpoints (Public)

| Method | Endpoint | Description |
|--------|----------|-------------|
| `POST` | `/v1/write` | Create a new neuron (memory formation) |
| `GET` | `/v1/read/{id}` | Read a neuron by ID |
| `GET` | `/v1/recall` | List neurons |
| `POST` | `/v1/search` | Search with spread activation |
| `POST` | `/v1/context` | Build token-aware LLM context |
| `POST` | `/v1/command` | MongoDB-like query operations |

> Note: Direct low-level neuron mutation is intentionally disabled on external API routes. Mutation is managed by higher-level index/admin flows.

### Brain State

| Method | Endpoint | Description |
|--------|----------|-------------|
| `GET` | `/v1/brain/state` | Get current brain state |
| `POST` | `/v1/brain/wake` | Force wake |
| `POST` | `/v1/brain/sleep` | Force sleep |
| `GET` | `/v1/brain/stats` | Per-index stats |

### UUID Registry

| Method | Endpoint | Description |
|--------|----------|-------------|
| `GET` | `/v1/registry` | List registered UUIDs |
| `POST` | `/v1/registry` | Register a UUID |
| `GET` | `/v1/registry/{uuid}` | Get UUID detail |
| `PUT` | `/v1/registry/{uuid}` | Update UUID metadata |
| `DELETE` | `/v1/registry/{uuid}` | Delete UUID |
| `POST` | `/v1/registry/find-or-create` | Find or create UUID |

### Runtime Configuration

| Method | Endpoint | Description |
|--------|----------|-------------|
| `GET` | `/v1/config` | Get active config (**admin auth required**) |
| `POST` | `/v1/config` | Patch runtime config (**admin auth required**) |

### Utility Endpoints

| Method | Endpoint | Description |
|--------|----------|-------------|
| `GET` | `/health` | Health check |
| `GET` | `/v1/stats` | Global stats |
| `GET` | `/v1/graph` | Neuron/synapse graph data |
| `GET` | `/v1/synapses` | Synapse list |
| `GET` | `/v1/activity` | Recent activity log |

---

## Usage Examples

### Write Memory

```bash
curl -X POST http://localhost:6060/v1/write \
  -H "Content-Type: application/json" \
  -H "X-Index-ID: index-123" \
  -d '{"content": "User prefers TypeScript"}'
```

### Write with Metadata (thread grouping)

```bash
curl -X POST http://localhost:6060/v1/write \
  -H "Content-Type: application/json" \
  -H "X-Index-ID: index-123" \
  -d '{
    "content": "The hippocampus encodes episodic memories during sleep",
    "metadata": {"thread_id": "conv-001", "role": "assistant"}
  }'
```

### Search (Spread Activation)

```bash
curl -X POST http://localhost:6060/v1/search \
  -H "X-Index-ID: index-123" \
  -d '{"query": "programming preferences", "depth": 2, "limit": 10}'
```

### Search with Metadata Boost (soft, default)

```bash
curl -X POST http://localhost:6060/v1/search \
  -H "X-Index-ID: index-123" \
  -d '{
    "query": "memory hippocampus",
    "metadata": {"thread_id": "conv-001"},
    "strict": false
  }'
```

### Search with Strict Metadata Filter

```bash
curl -X POST http://localhost:6060/v1/search \
  -H "X-Index-ID: index-123" \
  -d '{
    "query": "memory hippocampus",
    "metadata": {"thread_id": "conv-001", "role": "assistant"},
    "strict": true
  }'

# GET equivalent
curl "http://localhost:6060/v1/search?q=memory&metadata_thread_id=conv-001&strict=true" \
  -H "X-Index-ID: index-123"
```

### LLM Context Assembly

```bash
curl -X POST http://localhost:6060/v1/context \
  -H "X-Index-ID: index-123" \
  -d '{"cue": "what kinds of projects does this user build?", "maxTokens": 2000}'
```

### MongoDB-like Query

```bash
curl -X POST http://localhost:6060/v1/command \
  -H "X-Index-ID: index-123" \
  -d '{
    "type": "find",
    "collection": "neurons",
    "filter": {"energy": {"$gt": 0.5}},
    "options": {"limit": 10, "sort": {"energy": -1}}
  }'
```

---

## Brain Lifecycle

```
                    activity
  ┌──────────┐  ─────────────►  ┌──────────┐
  │ DORMANT  │                  │  ACTIVE  │
  │ (disk)   │                  │  (RAM)   │
  └──────────┘                  └────┬─────┘
       ▲                             │ inactivity
       │                             ▼
       │                        ┌──────────┐
       │                        │   IDLE   │
       │                        └────┬─────┘
       │                             │ threshold
       │                             ▼
       │                        ┌──────────┐
       └─────── eviction ────── │ SLEEPING │ <- consolidation
                                └──────────┘
```

Lifecycle transitions can trigger callbacks, and thresholds are configurable.

---

## Neuron Mechanics

**Activation:** When a neuron fires through natural retrieval paths, its energy increases and last-fire timestamp is updated.

**Decay:** Background daemons periodically reduce neuron energy. Unused memories weaken over time.

**Hebbian Synapses:** Co-activated neurons form and strengthen synapses. Unused synapses decay and can be pruned.

**Consolidation:** During sleeping phases, frequently accessed mature neurons move to deeper layers, improving long-term memory quality.

---

## Configuration Hierarchy

Priority order (highest to lowest):

1. **CLI flags** (`--http-addr`, `--data-path`, etc.)
2. **YAML file** (`--config` or `QUBICDB_CONFIG`)
3. **Environment variables** (`QUBICDB_*`)
4. **Defaults**

### Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `QUBICDB_CONFIG` | - | YAML config path |
| `QUBICDB_HTTP_ADDR` | `:6060` | HTTP API address |
| `QUBICDB_DATA_PATH` | `./data` | Data directory |
| `QUBICDB_COMPRESS` | `true` | Msgpack compression |
| `QUBICDB_WAL_ENABLED` | `true` | WAL (write-ahead log) enabled |
| `QUBICDB_FSYNC_POLICY` | `interval` | Fsync policy (`always`,`interval`,`off`) |
| `QUBICDB_FSYNC_INTERVAL` | `1s` | Fsync interval for `interval` policy |
| `QUBICDB_MAX_NEURONS` | `1000000` | Max neurons per index |
| `QUBICDB_REGISTRY_ENABLED` | `false` | UUID registry guard |
| `QUBICDB_IDLE_THRESHOLD` | `30s` | Active -> Idle threshold |
| `QUBICDB_SLEEP_THRESHOLD` | `5m` | Idle -> Sleeping threshold |
| `QUBICDB_DORMANT_THRESHOLD` | `30m` | Sleeping -> Dormant threshold |

### CLI Flags

```
--config          YAML config path
--http-addr       HTTP listen address
--data-path       Data directory
--compress        Msgpack compression
--max-neurons     Max neurons per index
--registry        Enable UUID registry guard
--admin           Enable admin API
--admin-user      Admin username
--admin-password  Admin password
--allowed-origins CORS allowed origins
--vector          Enable vector search
--vector-model    Path to GGUF model
--vector-alpha    Hybrid search weight (0.0-1.0)
```

### CLI Client (qubicdb-cli)

```bash
# Write with metadata
qubicdb-cli write "User prefers TypeScript" \
  --index index-123 \
  --metadata thread_id=conv-001,role=user

# Search with metadata boost
qubicdb-cli search "programming" \
  --index index-123 \
  --metadata thread_id=conv-001

# Search with strict filter
qubicdb-cli search "programming" \
  --index index-123 \
  --metadata thread_id=conv-001 \
  --strict
```

---

## Project Structure

```
qubicdb/
├── cmd/qubicdb/           # Main entrypoint (cobra CLI)
├── pkg/
│   ├── core/              # Core types, config, lifecycle helpers
│   ├── engine/            # Matrix operations, search engine
│   ├── synapse/           # Hebbian engine
│   ├── lifecycle/         # Brain state management
│   ├── persistence/       # Binary codec (.nrdb), disk I/O
│   ├── concurrency/       # Worker pool, per-index isolation
│   ├── daemon/            # Background daemons
│   ├── protocol/          # MongoDB-like query executor
│   ├── registry/          # UUID registry store
│   └── api/
│       ├── server.go      # HTTP API server
│       └── apierr/        # Standardized API errors
├── adapters/
│   └── typescript/        # @qubicdb/deepagent-adapter
├── qubicdb.example.yaml   # Example YAML config
├── Dockerfile
├── docker-compose.yml
└── go.mod
```

---

## Error Response Format

All API errors use the same JSON envelope:

```json
{
  "ok": false,
  "error": "descriptive error message",
  "code": "MACHINE_READABLE_CODE",
  "status": 400
}
```

Clients should branch on `code` for stable programmatic handling.

---

## deepagent Integration

```typescript
import { createDeepAgent, CompositeBackend, StateBackend } from "deepagents";
import { createQubicDBBackend } from "@qubicdb/deepagent-adapter";

const agent = createDeepAgent({
  model,
  backend: (config) =>
    new CompositeBackend(new StateBackend(config), {
      "/memories/": createQubicDBBackend({
        baseUrl: "http://localhost:6060",
        userId: config.userId,
      }),
    }),
});
```

---

## Philosophy

QubicDB targets one of the biggest LLM problems: **persistent, personalized memory**.

- **Cold start problem**: LLMs restart from scratch on each invocation. QubicDB keeps user/index memory continuously available.
- **One-size-fits-all memory**: Generic RAG outputs are often identical across users. QubicDB allows each index to evolve differently.
- **Embedding-only dependency**: Vector search is not always sufficient. QubicDB combines pattern matching with Hebbian association.
- **Scalability**: Each index runs in an isolated worker goroutine.

---

## License

MIT

---

Developed by [Deniz Umut Dereli](https://github.com/denizumutdereli)
