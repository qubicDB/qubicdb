# QubicDB Vector Wrapper

Builds `libllama_go` — the shared library QubicDB's vector layer loads at runtime via purego.

The wrapper exposes a minimal C ABI (`load_library`, `load_model`, `load_context`, `free_model`, `free_context`, `embed_size`, `embed_text`) that `pkg/vector/loader.go` binds to dynamically. No CGO required.

Source adapted from [kelindar/search](https://github.com/kelindar/search) (MIT).

---

## Prerequisites

| Tool | Minimum version |
|------|----------------|
| CMake | 3.14 |
| C++17 compiler | clang / gcc / MSVC |
| llama.cpp source | any recent clone |

You do **not** need a pre-installed llama.cpp system library. The build compiles llama.cpp as static libs and links them into the shared wrapper.

---

## Build

### 1. Get llama.cpp source

```bash
git clone https://github.com/ggerganov/llama.cpp /path/to/llama.cpp
```

Any recent commit works. You do not need to build llama.cpp separately.

### 2. Build the wrapper

**macOS / Linux:**

```bash
bash build.sh --llama-dir /path/to/llama.cpp
```

Output: `libllama_go.dylib` (macOS) or `libllama_go.so` (Linux) in the current directory.

**Windows (PowerShell):**

```powershell
pwsh build.ps1 -LlamaDir C:\path\to\llama.cpp
```

Output: `llama_go.dll` in the current directory.

### Optional flags

| Flag | Description |
|------|-------------|
| `--dest` / `-Dest` | Where to copy the built library (default: current dir) |
| `--jobs` / `-Jobs` | Parallel build jobs (default: CPU count) |
| `--cmake-args` / `-CmakeArgs` | Extra CMake flags (e.g. `-DGGML_METAL=ON`) |

### GPU / Metal / CUDA

Pass extra CMake flags via `--cmake-args`:

```bash
# Apple Metal (macOS)
bash build.sh --llama-dir /path/to/llama.cpp --cmake-args "-DGGML_METAL=ON"

# CUDA (Linux/Windows)
bash build.sh --llama-dir /path/to/llama.cpp --cmake-args "-DGGML_CUDA=ON"

# Vulkan
bash build.sh --llama-dir /path/to/llama.cpp --cmake-args "-DGGML_VULKAN=ON"
```

---

## Install

Copy the library so QubicDB can find it. The loader checks these locations (in order):

1. Current working directory (where you run `qubicdb`)
2. Executable directory
3. `/usr/lib`, `/usr/local/lib` (macOS also: `/opt/homebrew/lib`)
4. `DYLD_LIBRARY_PATH` / `LD_LIBRARY_PATH` entries

**Simplest — copy to working directory:**

```bash
cp libllama_go.dylib /path/to/qubicdb/qubicdb/
```

**System-wide (Linux):**

```bash
sudo cp libllama_go.so /usr/local/lib/
sudo ldconfig
```

**System-wide (macOS):**

```bash
sudo cp libllama_go.dylib /usr/local/lib/
```

**Runtime env (no install):**

```bash
# macOS
export DYLD_LIBRARY_PATH=/path/to/dir:$DYLD_LIBRARY_PATH

# Linux
export LD_LIBRARY_PATH=/path/to/dir:$LD_LIBRARY_PATH
```

---

## Configure QubicDB

Vector model path and all other settings come from QubicDB config — not from this patch. Users choose their own GGUF model.

**CLI flags:**

```bash
./qubicdb \
  --vector \
  --vector-model /path/to/your-model.gguf \
  --vector-alpha 0.6 \
  --vector-query-repeat 2
```

**Environment variables:**

```bash
export QUBICDB_VECTOR_ENABLED=true
export QUBICDB_VECTOR_MODEL_PATH=/path/to/your-model.gguf
export QUBICDB_VECTOR_ALPHA=0.6
export QUBICDB_VECTOR_QUERY_REPEAT=2
export QUBICDB_VECTOR_GPU_LAYERS=0   # set >0 for GPU offload
```

**YAML config:**

```yaml
vector:
  enabled: true
  modelPath: /path/to/your-model.gguf
  alpha: 0.6
  queryRepeat: 2
  gpuLayers: 0
  embedContextSize: 512
```

Any GGUF embedding model works (MiniLM, BGE, nomic-embed, etc.). Embedding dimension is detected automatically from the model file.

---

## Verify

```bash
# From qubicdb/qubicdb directory:
go test ./pkg/e2e -run ^$ -bench BenchmarkVectorizerEmbedTextLive -benchmem -benchtime=1x -v

# Set model path if not using default dist/MiniLM-L6-v2.Q8_0.gguf:
QUBICDB_VECTOR_MODEL_PATH=/path/to/model.gguf \
  go test ./pkg/e2e -run ^$ -bench BenchmarkVectorizerEmbedTextLive -benchmem -benchtime=1x -v
```

---

## Troubleshooting

**"llama.cpp shared library is not available"**
- Library file is missing or in a location the loader doesn't check.
- Run QubicDB with the library in the same directory, or set the env var above.

**`embed_text failed (rc=1)` — tokens exceed batch size**
- The wrapper sets `n_batch` and `n_ubatch` equal to `ctx_size` so the context window and batch size always match. If you still see this error, increase `embedContextSize` in config (default 512). Try 1024 or 2048.
- QubicDB also handles this automatically via chunking: long texts are split at sentence boundaries, each chunk is embedded separately, and the results are averaged.

**`embed_text failed (rc=2)` — last token not SEP**
- Model does not append a SEP token. This is a model compatibility issue; try a different BERT/embedding GGUF.

**Build fails: `common.h not found`**
- Ensure `--llama-dir` points to the llama.cpp **source** root (not a build or install directory).
