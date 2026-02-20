#!/usr/bin/env bash
# build.sh — Build libllama_go shared library for QubicDB vector support.
#
# Usage:
#   bash build.sh --llama-dir /path/to/llama.cpp [--dest /path/to/output] [--cmake-args "..."] [--jobs N]
#
# The resulting library will be placed in --dest (default: current directory).
# Copy it to the qubicdb working directory or a standard lib path.
#
# Platform output:
#   macOS  → libllama_go.dylib
#   Linux  → libllama_go.so
#   Windows → use build.ps1 instead

set -euo pipefail

LLAMA_DIR=""
DEST_DIR="$(pwd)"
EXTRA_CMAKE_ARGS=""
JOBS="$(getconf _NPROCESSORS_ONLN 2>/dev/null || echo 4)"

while [[ $# -gt 0 ]]; do
    case "$1" in
        --llama-dir)   LLAMA_DIR="$2";        shift 2 ;;
        --dest)        DEST_DIR="$2";         shift 2 ;;
        --cmake-args)  EXTRA_CMAKE_ARGS="$2"; shift 2 ;;
        --jobs)        JOBS="$2";             shift 2 ;;
        *) echo "Unknown argument: $1" >&2; exit 1 ;;
    esac
done

if [[ -z "$LLAMA_DIR" ]]; then
    echo "Error: --llama-dir is required." >&2
    echo "  Example: bash build.sh --llama-dir /path/to/llama.cpp" >&2
    echo "  The llama.cpp source can be cloned from: https://github.com/ggerganov/llama.cpp" >&2
    exit 1
fi

LLAMA_DIR="$(cd "$LLAMA_DIR" && pwd)"
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
BUILD_DIR="$SCRIPT_DIR/_build"

if [[ ! -f "$LLAMA_DIR/CMakeLists.txt" ]]; then
    echo "Error: $LLAMA_DIR does not look like a llama.cpp source tree." >&2
    exit 1
fi

echo "llama.cpp : $LLAMA_DIR"
echo "output    : $DEST_DIR"
echo "jobs      : $JOBS"
echo ""

mkdir -p "$BUILD_DIR"

# shellcheck disable=SC2086
cmake -B "$BUILD_DIR" \
    -DLLAMA_DIR="$LLAMA_DIR" \
    -DCMAKE_BUILD_TYPE=Release \
    $EXTRA_CMAKE_ARGS \
    "$SCRIPT_DIR"

cmake --build "$BUILD_DIR" --config Release --target llama_go -j "$JOBS"

# Locate the built library
LIB_PATH=""
for candidate in \
    "$BUILD_DIR/lib/libllama_go.dylib" \
    "$BUILD_DIR/lib/libllama_go.so" \
    "$BUILD_DIR/libllama_go.dylib" \
    "$BUILD_DIR/libllama_go.so"; do
    if [[ -f "$candidate" ]]; then
        LIB_PATH="$candidate"
        break
    fi
done

if [[ -z "$LIB_PATH" ]]; then
    echo "Error: built library not found under $BUILD_DIR" >&2
    exit 1
fi

mkdir -p "$DEST_DIR"
cp -f "$LIB_PATH" "$DEST_DIR/"
echo ""
echo "Built: $(basename "$LIB_PATH")"
echo "Copied to: $DEST_DIR/$(basename "$LIB_PATH")"
echo ""
echo "Next steps:"
echo "  1. Copy the library to your qubicdb working directory, or"
echo "  2. Install system-wide:"
if [[ "$(uname)" == "Darwin" ]]; then
    echo "       sudo cp $DEST_DIR/libllama_go.dylib /usr/local/lib/"
else
    echo "       sudo cp $DEST_DIR/libllama_go.so /usr/local/lib/ && sudo ldconfig"
fi
echo "  3. Or set the library path at runtime:"
if [[ "$(uname)" == "Darwin" ]]; then
    echo "       export DYLD_LIBRARY_PATH=$DEST_DIR:\$DYLD_LIBRARY_PATH"
else
    echo "       export LD_LIBRARY_PATH=$DEST_DIR:\$LD_LIBRARY_PATH"
fi
echo ""
echo "  Then configure QubicDB:"
echo "       --vector --vector-model /path/to/your-model.gguf"
echo "  or via env:"
echo "       QUBICDB_VECTOR_ENABLED=true QUBICDB_VECTOR_MODEL_PATH=/path/to/your-model.gguf"
