[CmdletBinding()]
param(
    [switch]$SkipJava,
    [switch]$SkipClient,
    [switch]$SkipSidecar,
    [switch]$SkipScriptContracts
)

$ErrorActionPreference = 'Stop'
Set-StrictMode -Version Latest
. (Join-Path $PSScriptRoot 'common.ps1')

$projectRoot = Get-OutViewProjectRoot
if (-not $SkipJava) {
    $null = Resolve-OutViewJava
    $maven = Resolve-OutViewMaven -ProjectRoot $projectRoot
    Invoke-OutViewProcess -FilePath $maven -Arguments @('test') -WorkingDirectory $projectRoot
}
if (-not $SkipClient) {
    $go = Resolve-OutViewGo
    $clientRoot = Join-Path $projectRoot 'client'
    Invoke-OutViewProcess -FilePath $go -Arguments @('test', '-count=1', '-timeout=120s', '-tags=headless_test', './...') -WorkingDirectory $clientRoot
    Invoke-OutViewProcess -FilePath $go -Arguments @('test', '-count=1', '-timeout=120s', '-tags=integration', './test/integration') -WorkingDirectory $clientRoot
    Invoke-OutViewProcess -FilePath $go -Arguments @('test', '-count=1', '-timeout=120s', '-tags=ci', './cmd/outview-gui') -WorkingDirectory $clientRoot
}
if (-not $SkipSidecar) {
    $go = Resolve-OutViewGo
    Invoke-OutViewProcess -FilePath $go -Arguments @('test', '-count=1', '-timeout=120s', './...') -WorkingDirectory (Join-Path $projectRoot 'webrtc-sidecar')
}
if (-not $SkipScriptContracts) {
    & (Join-Path $PSScriptRoot 'tests\build-contract.Tests.ps1')
    if ($LASTEXITCODE -ne 0) {
        throw "Script contract tests failed with exit code $LASTEXITCODE"
    }
}
Write-Host 'All selected test suites passed.'
