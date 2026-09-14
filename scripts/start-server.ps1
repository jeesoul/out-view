[CmdletBinding()]
param(
    [string]$JarPath,
    [string]$DataDir,
    [string]$JavaCommand,
    [Parameter(ValueFromRemainingArguments = $true)][string[]]$ServerArguments
)

$ErrorActionPreference = 'Stop'
Set-StrictMode -Version Latest
. (Join-Path $PSScriptRoot 'common.ps1')

$projectRoot = Get-OutViewProjectRoot
if ([string]::IsNullOrWhiteSpace($JarPath)) {
    $packagedJar = Join-Path $projectRoot 'outview-server.jar'
    $JarPath = if (Test-Path -LiteralPath $packagedJar -PathType Leaf) { $packagedJar } else { Join-Path $projectRoot 'target\outview-server.jar' }
}
elseif (-not [System.IO.Path]::IsPathRooted($JarPath)) {
    $JarPath = Join-Path $projectRoot $JarPath
}
$JarPath = [System.IO.Path]::GetFullPath($JarPath)
if (-not (Test-Path -LiteralPath $JarPath -PathType Leaf)) {
    throw "Server JAR not found: $JarPath. Run scripts/build.ps1 first."
}

if ([string]::IsNullOrWhiteSpace($DataDir)) {
    $DataDir = if ([string]::IsNullOrWhiteSpace($env:OUTVIEW_DATA_DIR)) { Join-Path $projectRoot 'data' } else { $env:OUTVIEW_DATA_DIR }
}
if (-not [System.IO.Path]::IsPathRooted($DataDir)) {
    $DataDir = Join-Path $projectRoot $DataDir
}
$DataDir = [System.IO.Path]::GetFullPath($DataDir)
New-Item -ItemType Directory -Path $DataDir -Force | Out-Null
$env:OUTVIEW_DATA_DIR = $DataDir

if ([string]::IsNullOrWhiteSpace($JavaCommand)) {
    $JavaCommand = Resolve-OutViewJava
}

Push-Location $projectRoot
try {
    Write-Host "Starting outView server in $projectRoot"
    Write-Host "Persistent data directory: $DataDir"
    & $JavaCommand '-jar' $JarPath @ServerArguments
    exit $LASTEXITCODE
}
finally {
    Pop-Location
}
