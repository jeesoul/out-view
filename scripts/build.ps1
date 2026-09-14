[CmdletBinding()]
param(
    [ValidateSet('Cli', 'Gui', 'Both')][string]$ClientMode = 'Cli',
    [switch]$SkipServer,
    [switch]$SkipClient,
    [switch]$SkipSidecar,
    [switch]$Release,
    [string]$OutputDirectory
)

$ErrorActionPreference = 'Stop'
Set-StrictMode -Version Latest
. (Join-Path $PSScriptRoot 'common.ps1')

$projectRoot = Get-OutViewProjectRoot
$version = Get-OutViewVersion -ProjectRoot $projectRoot
if ([string]::IsNullOrWhiteSpace($OutputDirectory)) {
    $baseDirectory = if ($Release) { 'release' } else { 'artifacts' }
    $OutputDirectory = Join-Path $projectRoot (Join-Path $baseDirectory "outview-$version")
}
elseif (-not [System.IO.Path]::IsPathRooted($OutputDirectory)) {
    $OutputDirectory = Join-Path $projectRoot $OutputDirectory
}
$destination = [System.IO.Path]::GetFullPath($OutputDirectory)
if (Test-Path -LiteralPath $destination) {
    throw "Output directory already exists; refusing to overwrite it: $destination"
}

$destinationParent = Split-Path -Parent $destination
New-Item -ItemType Directory -Path $destinationParent -Force | Out-Null
$destinationParent = (Resolve-Path -LiteralPath $destinationParent).Path
Assert-OutViewChildPath -Path $destination -Parent $destinationParent -Purpose 'build destination'
$staging = Join-Path $destinationParent ('.outview-staging-{0}-{1}' -f $PID, [guid]::NewGuid().ToString('N'))
Assert-OutViewChildPath -Path $staging -Parent $destinationParent -Purpose 'staging directory'
New-Item -ItemType Directory -Path $staging | Out-Null

$sourceDateEpoch = $env:SOURCE_DATE_EPOCH
if ([string]::IsNullOrWhiteSpace($sourceDateEpoch)) {
    $buildDate = [DateTime]::UtcNow.ToString('yyyy-MM-ddTHH:mm:ssZ')
}
else {
    $buildDate = [DateTimeOffset]::FromUnixTimeSeconds([long]$sourceDateEpoch).UtcDateTime.ToString('yyyy-MM-ddTHH:mm:ssZ')
}

try {
    if (-not $SkipServer) {
        $null = Resolve-OutViewJava
        $maven = Resolve-OutViewMaven -ProjectRoot $projectRoot
        Write-Host "Building Java server v$version"
        Invoke-OutViewProcess -FilePath $maven -Arguments @('clean', 'package', '-DskipTests') -WorkingDirectory $projectRoot
        $serverJar = Join-Path $projectRoot 'target\outview-server.jar'
        if (-not (Test-Path -LiteralPath $serverJar -PathType Leaf)) {
            throw "Expected server artifact was not created: $serverJar"
        }
        Copy-Item -LiteralPath $serverJar -Destination (Join-Path $staging 'outview-server.jar')
    }

    if (-not $SkipClient) {
        $go = Resolve-OutViewGo
        $clientRoot = Join-Path $projectRoot 'client'
        $platforms = if ($Release) {
            @('windows/amd64', 'linux/amd64', 'linux/arm64', 'darwin/amd64', 'darwin/arm64')
        }
        else {
            $hostOs = (& $go env GOOS).Trim()
            $hostArch = (& $go env GOARCH).Trim()
            @("$hostOs/$hostArch")
        }
        foreach ($platform in $platforms) {
            $goos, $goarch = $platform.Split('/')
            $extension = if ($goos -eq 'windows') { '.exe' } else { '' }
            $platformDirectory = Join-Path $staging (Join-Path 'client' $goos)
            New-Item -ItemType Directory -Path $platformDirectory -Force | Out-Null
            if ($ClientMode -in @('Cli', 'Both')) {
                $output = Join-Path $platformDirectory "outview-client-cli-$goos-$goarch$extension"
                Write-Host "Building CLI client for $goos/$goarch"
                $previousGoos, $previousGoarch, $previousCgo = $env:GOOS, $env:GOARCH, $env:CGO_ENABLED
                try {
                    $env:GOOS, $env:GOARCH, $env:CGO_ENABLED = $goos, $goarch, '0'
                    Invoke-OutViewProcess -FilePath $go -Arguments @('build', '-trimpath', '-ldflags', "-s -w -X main.Version=$version -X main.BuildDate=$buildDate", '-o', $output, './cmd/outview-client') -WorkingDirectory $clientRoot
                }
                finally {
                    $env:GOOS, $env:GOARCH, $env:CGO_ENABLED = $previousGoos, $previousGoarch, $previousCgo
                }
            }
            if ($ClientMode -in @('Gui', 'Both')) {
                if ($goos -ne 'windows' -or $goarch -ne 'amd64') {
                    continue
                }
                $output = Join-Path $platformDirectory "outview-client-gui-$goos-$goarch$extension"
                Write-Host 'Building GUI client for windows/amd64; a working CGO C compiler is required'
                $previousGoos, $previousGoarch, $previousCgo = $env:GOOS, $env:GOARCH, $env:CGO_ENABLED
                try {
                    $env:GOOS, $env:GOARCH, $env:CGO_ENABLED = 'windows', 'amd64', '1'
                    Invoke-OutViewProcess -FilePath $go -Arguments @('build', '-trimpath', '-ldflags', "-s -w -H windowsgui -X main.Version=$version -X main.BuildDate=$buildDate", '-o', $output, './cmd/outview-gui') -WorkingDirectory $clientRoot
                }
                finally {
                    $env:GOOS, $env:GOARCH, $env:CGO_ENABLED = $previousGoos, $previousGoarch, $previousCgo
                }
            }
        }
    }

    if (-not $SkipSidecar) {
        $go = Resolve-OutViewGo
        $sidecarRoot = Join-Path $projectRoot 'webrtc-sidecar'
        $platforms = if ($Release) {
            @('windows/amd64', 'linux/amd64', 'linux/arm64', 'darwin/amd64', 'darwin/arm64')
        }
        else {
            @("$((& $go env GOOS).Trim())/$((& $go env GOARCH).Trim())")
        }
        foreach ($platform in $platforms) {
            $goos, $goarch = $platform.Split('/')
            $extension = if ($goos -eq 'windows') { '.exe' } else { '' }
            $platformDirectory = Join-Path $staging (Join-Path 'webrtc-sidecar' $goos)
            New-Item -ItemType Directory -Path $platformDirectory -Force | Out-Null
            $output = Join-Path $platformDirectory "outview-sidecar-$goos-$goarch$extension"
            Write-Host "Building sidecar for $goos/$goarch"
            $previousGoos, $previousGoarch, $previousCgo = $env:GOOS, $env:GOARCH, $env:CGO_ENABLED
            try {
                $env:GOOS, $env:GOARCH, $env:CGO_ENABLED = $goos, $goarch, '0'
                Invoke-OutViewProcess -FilePath $go -Arguments @('build', '-trimpath', '-ldflags', '-s -w', '-o', $output, './cmd/sidecar') -WorkingDirectory $sidecarRoot
            }
            finally {
                $env:GOOS, $env:GOARCH, $env:CGO_ENABLED = $previousGoos, $previousGoarch, $previousCgo
            }
        }
    }

    foreach ($file in @('README.md', 'USER_MANUAL.md', 'RELEASE_SUMMARY.md', 'CHANGELOG.md', 'LICENSE')) {
        $source = Join-Path $projectRoot $file
        if (Test-Path -LiteralPath $source -PathType Leaf) {
            Copy-Item -LiteralPath $source -Destination $staging
        }
    }
    $applicationConfig = Join-Path $projectRoot 'src\main\resources\application.yml'
    $packagedDocs = Join-Path $staging 'docs'
    New-Item -ItemType Directory -Path $packagedDocs | Out-Null
    Get-ChildItem -LiteralPath (Join-Path $projectRoot 'docs') -Filter '*.md' -File | ForEach-Object {
        Copy-Item -LiteralPath $_.FullName -Destination $packagedDocs
    }
    if (Test-Path -LiteralPath $applicationConfig -PathType Leaf) {
        Copy-Item -LiteralPath $applicationConfig -Destination (Join-Path $staging 'application.yml.example')
    }
    $packagedScripts = Join-Path $staging 'scripts'
    New-Item -ItemType Directory -Path $packagedScripts | Out-Null
    foreach ($script in @('common.ps1', 'start-server.ps1', 'start-server.bat', 'common.sh', 'start-server.sh')) {
        Copy-Item -LiteralPath (Join-Path $PSScriptRoot $script) -Destination $packagedScripts
    }
    Assert-OutViewChildPath -Path $staging -Parent $destinationParent -Purpose 'published source'
    Assert-OutViewChildPath -Path $destination -Parent $destinationParent -Purpose 'published destination'
    Move-Item -LiteralPath $staging -Destination $destination
    Write-Host "Build completed: $destination"
}
catch {
    if (Test-Path -LiteralPath $staging) {
        Assert-OutViewChildPath -Path $staging -Parent $destinationParent -Purpose 'staging cleanup'
        if (-not (Split-Path -Leaf $staging).StartsWith('.outview-staging-')) {
            throw "Refusing to clean an unexpected staging path: $staging"
        }
        Remove-Item -LiteralPath $staging -Recurse -Force
    }
    throw
}
