# build.ps1 — Build llama_go.dll for QubicDB vector support on Windows.
#
# Usage:
#   pwsh build.ps1 -LlamaDir C:\path\to\llama.cpp [-Dest C:\output] [-CmakeArgs "..."] [-Jobs 8]
#
# The resulting llama_go.dll will be placed in -Dest (default: current directory).
# Copy it to the qubicdb working directory.

param(
    [Parameter(Mandatory = $true)]
    [string]$LlamaDir,

    [string]$Dest = (Get-Location).Path,

    [string]$CmakeArgs = "",

    [int]$Jobs = $env:NUMBER_OF_PROCESSORS
)

$ErrorActionPreference = "Stop"

$ScriptDir = Split-Path -Parent $MyInvocation.MyCommand.Path
$BuildDir  = Join-Path $ScriptDir "_build"

if (!(Test-Path $LlamaDir)) {
    throw "LlamaDir does not exist: $LlamaDir"
}
if (!(Test-Path (Join-Path $LlamaDir "CMakeLists.txt"))) {
    throw "$LlamaDir does not look like a llama.cpp source tree."
}

$LlamaDir = (Resolve-Path $LlamaDir).Path

Write-Host "llama.cpp : $LlamaDir" -ForegroundColor Cyan
Write-Host "output    : $Dest"     -ForegroundColor Cyan
Write-Host "jobs      : $Jobs"     -ForegroundColor Cyan
Write-Host ""

New-Item -ItemType Directory -Force -Path $BuildDir | Out-Null

$cmakeCmd = "cmake -B `"$BuildDir`" -DLLAMA_DIR=`"$LlamaDir`" -DCMAKE_BUILD_TYPE=Release $CmakeArgs `"$ScriptDir`""
Write-Host "Running: $cmakeCmd" -ForegroundColor Gray
Invoke-Expression $cmakeCmd

cmake --build $BuildDir --config Release --target llama_go -j $Jobs

# Locate the built DLL
$candidates = @(
    (Join-Path $BuildDir "bin\Release\llama_go.dll"),
    (Join-Path $BuildDir "bin\llama_go.dll"),
    (Join-Path $BuildDir "Release\llama_go.dll"),
    (Join-Path $BuildDir "llama_go.dll")
)

$libPath = $null
foreach ($c in $candidates) {
    if (Test-Path $c) {
        $libPath = $c
        break
    }
}

if ($null -eq $libPath) {
    throw "Built library not found under $BuildDir. Check build output above."
}

New-Item -ItemType Directory -Force -Path $Dest | Out-Null
Copy-Item -Path $libPath -Destination (Join-Path $Dest "llama_go.dll") -Force

Write-Host ""
Write-Host "Built: llama_go.dll" -ForegroundColor Green
Write-Host "Copied to: $Dest\llama_go.dll" -ForegroundColor Green
Write-Host ""
Write-Host "Next steps:" -ForegroundColor Cyan
Write-Host "  1. Copy llama_go.dll to your qubicdb working directory."
Write-Host "  2. Configure QubicDB:"
Write-Host "       --vector --vector-model C:\path\to\your-model.gguf"
Write-Host "     or via env:"
Write-Host "       `$env:QUBICDB_VECTOR_ENABLED='true'"
Write-Host "       `$env:QUBICDB_VECTOR_MODEL_PATH='C:\path\to\your-model.gguf'"
