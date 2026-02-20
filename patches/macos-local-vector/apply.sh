#!/usr/bin/env bash
# macOS Local Vector Patch
#
# Builds libllama_go.dylib from source and copies it to the qubicdb working directory.
# Delegates to patches/vector-wrapper/build.sh — see that file for full options.
#
# Usage:
#   bash apply.sh --llama-dir /path/to/llama.cpp [--dest /path/to/qubicdb/qubicdb] [--smoke-test]

set -euo pipefail

LLAMA_DIR=""
DEST_DIR="$(cd "$(dirname "$0")/../.." && pwd)"
RUN_SMOKE_TEST=false
EXTRA_ARGS=()

while [[ $# -gt 0 ]]; do
    case "$1" in
        --llama-dir)   LLAMA_DIR="$2"; shift 2 ;;
        --dest)        DEST_DIR="$2";  shift 2 ;;
        --smoke-test)  RUN_SMOKE_TEST=true; shift ;;
        --cmake-args)  EXTRA_ARGS+=(--cmake-args "$2"); shift 2 ;;
        --jobs)        EXTRA_ARGS+=(--jobs "$2"); shift 2 ;;
        *) echo "Unknown argument: $1" >&2; exit 1 ;;
    esac
done

if [[ -z "$LLAMA_DIR" ]]; then
    echo "Error: --llama-dir is required." >&2
    echo "  Clone llama.cpp: git clone https://github.com/ggerganov/llama.cpp" >&2
    echo "  Then: bash apply.sh --llama-dir /path/to/llama.cpp" >&2
    exit 1
fi

WRAPPER_DIR="$(cd "$(dirname "$0")/../vector-wrapper" && pwd)"
bash "$WRAPPER_DIR/build.sh" --llama-dir "$LLAMA_DIR" --dest "$DEST_DIR" "${EXTRA_ARGS[@]}"

if [[ "$RUN_SMOKE_TEST" == true ]]; then
    echo ""
    echo "Running smoke benchmark..."
    cd "$DEST_DIR"
    DYLD_LIBRARY_PATH="$DEST_DIR${DYLD_LIBRARY_PATH:+:$DYLD_LIBRARY_PATH}" \
        go test ./pkg/e2e -run '^$' -bench BenchmarkVectorizerEmbedTextLive -benchmem -benchtime=1x -v
fi
