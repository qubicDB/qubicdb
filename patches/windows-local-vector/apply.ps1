# Windows Local Vector Patch
#
# Builds llama_go.dll from source and copies it to the qubicdb working directory.
# Delegates to patches/vector-wrapper/build.ps1 — see that file for full options.
#
# Usage:
#   pwsh apply.ps1 -LlamaDir C:\path\to\llama.cpp [-Dest C:\path\to\qubicdb\qubicdb] [-RunSmokeTest]

param(
    [Parameter(Mandatory = $true)]
    [string]$LlamaDir,

    [string]$Dest = (Join-Path $PSScriptRoot "..\.."),

    [string]$CmakeArgs = "",

    [int]$Jobs = $env:NUMBER_OF_PROCESSORS,

    [switch]$RunSmokeTest
)

$ErrorActionPreference = "Stop"

$WrapperScript = Join-Path $PSScriptRoot "..\vector-wrapper\build.ps1"

if (!(Test-Path $WrapperScript)) {
    throw "vector-wrapper/build.ps1 not found at: $WrapperScript"
}

$Dest = (Resolve-Path $Dest -ErrorAction SilentlyContinue)?.Path ?? $Dest

& pwsh $WrapperScript -LlamaDir $LlamaDir -Dest $Dest -CmakeArgs $CmakeArgs -Jobs $Jobs

if ($RunSmokeTest) {
    Write-Host "`nRunning smoke benchmark..." -ForegroundColor Cyan
    Push-Location $Dest
    try {
        go test ./pkg/e2e -run "^$" -bench BenchmarkVectorizerEmbedTextLive -benchmem -benchtime=1x -v
    }
    finally {
        Pop-Location
    }
}
