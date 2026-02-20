cmake_minimum_required(VERSION 3.14)
project(llama_go_wrapper VERSION 1.0 LANGUAGES CXX)

set(CMAKE_CXX_STANDARD 17)
set(CMAKE_CXX_STANDARD_REQUIRED ON)
set(CMAKE_POSITION_INDEPENDENT_CODE ON)

# ── LLAMA_DIR: path to a llama.cpp source tree (required) ────────────────────
# Pass via: cmake -DLLAMA_DIR=/path/to/llama.cpp ..
if(NOT DEFINED LLAMA_DIR)
    message(FATAL_ERROR
        "LLAMA_DIR is not set.\n"
        "Run cmake with: -DLLAMA_DIR=/path/to/llama.cpp\n"
        "Example: cmake -B build -DLLAMA_DIR=../../llama.cpp")
endif()

if(NOT EXISTS "${LLAMA_DIR}/CMakeLists.txt")
    message(FATAL_ERROR "LLAMA_DIR does not look like a llama.cpp source tree: ${LLAMA_DIR}")
endif()

# ── Build llama.cpp as static libs (no shared, no examples) ──────────────────
set(BUILD_SHARED_LIBS       OFF CACHE BOOL "" FORCE)
set(LLAMA_BUILD_TESTS       OFF CACHE BOOL "" FORCE)
set(LLAMA_BUILD_EXAMPLES    OFF CACHE BOOL "" FORCE)
set(LLAMA_BUILD_SERVER      OFF CACHE BOOL "" FORCE)
set(LLAMA_BUILD_COMMON      ON  CACHE BOOL "" FORCE)

add_subdirectory(${LLAMA_DIR} llama.cpp EXCLUDE_FROM_ALL)

# ── llama_go shared library ───────────────────────────────────────────────────
add_library(llama_go SHARED llama-go.cpp)

target_include_directories(llama_go PRIVATE
    ${LLAMA_DIR}/include
    ${LLAMA_DIR}/common
    ${LLAMA_DIR}/ggml/include
)

target_compile_definitions(llama_go PRIVATE
    LLAMA_SHARED
    LLAMA_BUILD
)

target_link_libraries(llama_go PRIVATE
    llama
    common
)

set_target_properties(llama_go PROPERTIES
    VERSION   ${PROJECT_VERSION}
    SOVERSION 1
    LIBRARY_OUTPUT_DIRECTORY ${CMAKE_BINARY_DIR}/lib
    RUNTIME_OUTPUT_DIRECTORY ${CMAKE_BINARY_DIR}/bin
)
