#requires -Version 7.0
<#
.SYNOPSIS
  Local build entry point for the codex-eval v0.2.21 fork. Does not use GitHub Actions.
.EXAMPLE
  .\codex-eval\scripts\build.ps1 -Stage source
  .\codex-eval\scripts\build.ps1 -Stage resume -SkipTests -SkipFrontend
#>
param(
  [ValidateSet("all", "source", "resume")]
  [string]$Stage = "all",
  [string]$UpstreamTag = "v0.2.21",
  [string]$Platforms = "linux-amd64",
  [switch]$SkipTests,
  [switch]$SkipFrontend
)

$ErrorActionPreference = "Stop"
$root = (Resolve-Path (Join-Path $PSScriptRoot "..\..")).Path
$buildScript = Join-Path $root "codex-eval\scripts\build.py"
$python = if ($env:PYTHON) { $env:PYTHON } else { "python" }

# Local shims keep the frontend script's nested `pnpm` calls working without corepack.
$shimDir = Join-Path $root "build\bin"
if (Test-Path $shimDir) { $env:PATH = "$shimDir;$env:PATH" }
$goBin = "D:\tools\go1.27.4\go\bin"
if ((Test-Path (Join-Path $goBin "go.exe")) -and -not (Get-Command go -ErrorAction SilentlyContinue)) {
  $env:PATH = "$goBin;$env:PATH"
}

$env:UPSTREAM_TAG = $UpstreamTag
$env:SUB2API_BUILD_PLATFORMS = $Platforms
$env:SUB2API_BUILD_WORKSPACE = Join-Path $root "build\buildpy-work"
$env:NODE_OPTIONS = if ($env:NODE_OPTIONS) { $env:NODE_OPTIONS } else { "--max-old-space-size=8192" }
$env:GOMAXPROCS = if ($env:GOMAXPROCS) { $env:GOMAXPROCS } else { "2" }
$env:SUB2API_GOFLAGS = if ($env:SUB2API_GOFLAGS) { $env:SUB2API_GOFLAGS } else { "-mod=readonly -p=2" }
if ($SkipTests) { $env:SUB2API_SKIP_TESTS = "1" } else { Remove-Item Env:SUB2API_SKIP_TESTS -ErrorAction SilentlyContinue }
if ($SkipFrontend) { $env:SUB2API_SKIP_FRONTEND = "1" } else { Remove-Item Env:SUB2API_SKIP_FRONTEND -ErrorAction SilentlyContinue }

if ($Stage -eq "all") {
  Remove-Item Env:SUB2API_BUILD_STAGE -ErrorAction SilentlyContinue
} else {
  $env:SUB2API_BUILD_STAGE = $Stage
}

Write-Host "codex-eval local build"
Write-Host "  stage:   $Stage"
Write-Host "  upstream:$UpstreamTag"
Write-Host "  platform:$Platforms"
Write-Host "  python:  $python"

& $python $buildScript
exit $LASTEXITCODE
