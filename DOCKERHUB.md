# QubicDB

**A brain-like recursive memory system for LLM applications**

QubicDB is an organic memory engine inspired by biological brain mechanics. Instead of flat key-value storage, it creates an isolated, self-organizing, Hebbian-strengthened brain instance per index — enabling LLMs to form, consolidate, and recall memories the way biological systems do.

---

## Quick Start

```bash
docker pull qubicdb/qubicdb:1.0.1

docker run -d \
  --name qubicdb \
  -p 6060:6060 \
  -v $(pwd)/data:/app/data \
  -e QUBICDB_ADMIN_ENABLED=true \
  -e QUBICDB_ADMIN_USER=admin \
  -e QUBICDB_ADMIN_PASSWORD=changeme \
  qubicdb/qubicdb:1.0.1
```

Server is available at `http://localhost:6060`.

---

## With Admin UI

```bash
docker network create qubicdb-net

docker run -d \
  --name qubicdb \
  --network qubicdb-net \
  -p 6060:6060 \
  -e QUBICDB_ADMIN_ENABLED=true \
  -e QUBICDB_ADMIN_USER=admin \
  -e QUBICDB_ADMIN_PASSWORD=changeme \
  -e QUBICDB_ALLOWED_ORIGINS=http://localhost:8080 \
  qubicdb/qubicdb:1.0.1

docker run -d \
  --name qubicdb-ui \
  --network qubicdb-net \
  -p 8080:80 \
  qubicdb/qubicdb-ui:1.0.1
```

Then open `http://localhost:8080` and log in with your admin credentials.

---

## Key Features

- **Organic N-Dimensional Matrix** — memory space expands and shrinks dynamically
- **Hebbian Learning** — synapses form and strengthen between co-activated neurons
- **Fractal Clustering** — self-similar topology: dense cores → loose shells → separated clusters
- **Lifecycle States** — Active → Idle → Sleeping → Dormant transitions with automatic sleep consolidation
- **Per-Index Isolation** — dedicated goroutine and matrix per index (one brain per user/session)
- **REST API** — write, search, recall, read, forget endpoints
- **MCP Support** — Model Context Protocol for direct LLM tool integration
- **CLI REPL** — interactive admin shell
- **Persistence** — compressed on-disk storage with crash recovery

---

## Environment Variables

| Variable | Default | Description |
|---|---|---|
| `QUBICDB_HTTP_ADDR` | `:6060` | HTTP listen address |
| `QUBICDB_DATA_PATH` | `./data` | Data directory |
| `QUBICDB_ADMIN_ENABLED` | `false` | Enable admin endpoints |
| `QUBICDB_ADMIN_USER` | `admin` | Admin username |
| `QUBICDB_ADMIN_PASSWORD` | — | Admin password (required if enabled) |
| `QUBICDB_ALLOWED_ORIGINS` | `*` | CORS allowed origins (comma-separated) |
| `QUBICDB_MCP_ENABLED` | `false` | Enable MCP endpoint |

---

## API Overview

```bash
# Write a memory
curl -X POST http://localhost:6060/v1/write \
  -H "Content-Type: application/json" \
  -H "X-Index-ID: user-alice" \
  -d '{"content": "Alice prefers Python for data science"}'

# Search memories
curl -X POST http://localhost:6060/v1/search \
  -H "Content-Type: application/json" \
  -H "X-Index-ID: user-alice" \
  -d '{"query": "programming language", "limit": 5}'

# Health check
curl http://localhost:6060/health
```

Full API reference: [qubicdb.github.io/docs](https://qubicdb.github.io/docs/)

---

## Tags

| Tag | Description |
|---|---|
| `latest` | Latest stable release |
| `1.0.1` | Current stable release |
| `1.0.0` | Initial release |

---

## Links

- **Website:** [qubicdb.github.io/qubicdb-web](https://qubicdb.github.io/qubicdb-web/)
- **Source:** [github.com/qubicDB/qubicdb](https://github.com/qubicDB/qubicdb)
- **API Docs:** [qubicdb.github.io/docs](https://qubicdb.github.io/docs/)
- **Admin UI:** [hub.docker.com/r/qubicdb/qubicdb-ui](https://hub.docker.com/r/qubicdb/qubicdb-ui)

---

MIT License · Developed by [Deniz Umut Dereli](https://github.com/denizumutdereli)
