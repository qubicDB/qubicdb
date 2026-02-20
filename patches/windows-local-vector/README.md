# Windows Local Vector Patch (Dev Only)

Builds `llama_go.dll` from source and copies it to the qubicdb working directory.
Delegates to `patches/vector-wrapper/` — see that directory for the full build system.

## Usage

```powershell
pwsh ./patches/windows-local-vector/apply.ps1 -LlamaDir C:\path\to\llama.cpp
```

Optional smoke test:

```powershell
pwsh ./patches/windows-local-vector/apply.ps1 -LlamaDir C:\path\to\llama.cpp -RunSmokeTest
```

## Prerequisites

- CMake 3.14+
- Visual Studio Build Tools (C++17) or MinGW
- llama.cpp source: `git clone https://github.com/ggerganov/llama.cpp`

## What it does

1. Builds `llama_go.dll` via `patches/vector-wrapper/build.ps1`
2. Copies the DLL to the qubicdb working directory

## Configure QubicDB

```powershell
$env:QUBICDB_VECTOR_ENABLED = "true"
$env:QUBICDB_VECTOR_MODEL_PATH = "C:\path\to\your-model.gguf"
```

Or via CLI flags:

```powershell
.\qubicdb.exe --vector --vector-model C:\path\to\your-model.gguf
```

Any GGUF embedding model works. See `patches/vector-wrapper/README.md` for full details.

## Manual verify

From `qubicdb/qubicdb`:

```powershell
go test ./pkg/e2e -run ^$ -bench BenchmarkVectorizerEmbedTextLive -benchmem -benchtime=1x -v
```
