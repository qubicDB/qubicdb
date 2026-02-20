# macOS Local Vector Patch (Dev Only)

Builds `libllama_go.dylib` from source and copies it to the qubicdb working directory.
Delegates to `patches/vector-wrapper/` — see that directory for the full build system.

## Usage

```bash
bash ./patches/macos-local-vector/apply.sh --llama-dir /path/to/llama.cpp
```

Optional smoke test:

```bash
bash ./patches/macos-local-vector/apply.sh --llama-dir /path/to/llama.cpp --smoke-test
```

## Prerequisites

- CMake 3.14+, Xcode Command Line Tools (`xcode-select --install`)
- llama.cpp source: `git clone https://github.com/ggerganov/llama.cpp`

## What it does

1. Builds `libllama_go.dylib` via `patches/vector-wrapper/build.sh`
2. Copies the library to the qubicdb working directory

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
go test ./pkg/e2e -run ^$ -bench BenchmarkVectorizerEmbedTextLive -benchmem -benchtime=1x -v
```
