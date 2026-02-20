# Linux Local Vector Patch (Dev Only)

Builds `libllama_go.so` from source and copies it to the qubicdb working directory.
Delegates to `patches/vector-wrapper/` — see that directory for the full build system.

## Usage

```bash
bash ./patches/linux-local-vector/apply.sh --llama-dir /path/to/llama.cpp
```

Optional system-wide install + smoke test:

```bash
bash ./patches/linux-local-vector/apply.sh --llama-dir /path/to/llama.cpp --system-install --smoke-test
```

## Prerequisites

- CMake 3.14+, gcc/clang, build-essential
- llama.cpp source: `git clone https://github.com/ggerganov/llama.cpp`

## What it does

1. Builds `libllama_go.so` via `patches/vector-wrapper/build.sh`
2. Copies the library to the qubicdb working directory
3. With `--system-install`: copies to `/usr/local/lib/` and runs `ldconfig`

## Configure QubicDB

```bash
export QUBICDB_VECTOR_ENABLED=true
export QUBICDB_VECTOR_MODEL_PATH=/path/to/your-model.gguf
```

Or via CLI flags:

```bash
./qubicdb --vector --vector-model /path/to/your-model.gguf
```

Any GGUF embedding model works. See `patches/vector-wrapper/README.md` for full details.

## Manual verify

From `qubicdb/qubicdb`:

```bash
# If library is in working directory (not system-installed):
LD_LIBRARY_PATH=$(pwd) go test ./pkg/e2e -run ^$ -bench BenchmarkVectorizerEmbedTextLive -benchmem -benchtime=1x -v
```
