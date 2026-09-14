$ErrorActionPreference = 'Stop'
Set-StrictMode -Version Latest

function Get-OutViewProjectRoot {
    return (Resolve-Path (Join-Path $PSScriptRoot '..')).Path
}

function Get-OutViewVersion {
    param([string]$ProjectRoot = (Get-OutViewProjectRoot))

    $pomPath = Join-Path $ProjectRoot 'pom.xml'
    [xml]$pom = Get-Content -Raw -Encoding UTF8 -LiteralPath $pomPath
    $namespace = New-Object System.Xml.XmlNamespaceManager($pom.NameTable)
    $namespace.AddNamespace('m', 'http://maven.apache.org/POM/4.0.0')
    $node = $pom.SelectSingleNode('/m:project/m:version', $namespace)
    if ($null -eq $node -or [string]::IsNullOrWhiteSpace($node.InnerText)) {
        throw "Cannot read the project version from $pomPath"
    }
    return $node.InnerText.Trim()
}

function Resolve-OutViewCommand {
    param(
        [Parameter(Mandatory = $true)][string]$Name,
        [string[]]$Candidates = @()
    )

    foreach ($candidate in $Candidates) {
        if (-not [string]::IsNullOrWhiteSpace($candidate) -and (Test-Path -LiteralPath $candidate -PathType Leaf)) {
            return (Resolve-Path -LiteralPath $candidate).Path
        }
    }

    $command = Get-Command $Name -ErrorAction SilentlyContinue | Select-Object -First 1
    if ($null -ne $command) {
        return $command.Source
    }
    throw "Required command '$Name' was not found. Configure its standard environment variable or add it to PATH."
}

function Resolve-OutViewJava {
    $candidates = @()
    if (-not [string]::IsNullOrWhiteSpace($env:JAVA_HOME)) {
        $candidates += (Join-Path $env:JAVA_HOME 'bin\java.exe')
        $candidates += (Join-Path $env:JAVA_HOME 'bin\java')
    }
    return Resolve-OutViewCommand -Name 'java' -Candidates $candidates
}

function Resolve-OutViewGo {
    $candidates = @()
    if (-not [string]::IsNullOrWhiteSpace($env:GOROOT)) {
        $candidates += (Join-Path $env:GOROOT 'bin\go.exe')
        $candidates += (Join-Path $env:GOROOT 'bin\go')
    }
    return Resolve-OutViewCommand -Name 'go' -Candidates $candidates
}

function Resolve-OutViewMaven {
    param([string]$ProjectRoot = (Get-OutViewProjectRoot))

    $candidates = @(
        (Join-Path $ProjectRoot 'mvnw.cmd'),
        (Join-Path $ProjectRoot 'mvnw')
    )
    foreach ($variableName in @('MAVEN_HOME', 'M2_HOME')) {
        $value = [Environment]::GetEnvironmentVariable($variableName)
        if (-not [string]::IsNullOrWhiteSpace($value)) {
            $candidates += (Join-Path $value 'bin\mvn.cmd')
            $candidates += (Join-Path $value 'bin\mvn')
        }
    }

    foreach ($name in @('mvn.cmd', 'mvn')) {
        $command = Get-Command $name -ErrorAction SilentlyContinue | Select-Object -First 1
        if ($null -ne $command) {
            return $command.Source
        }
    }
    foreach ($candidate in $candidates) {
        if (Test-Path -LiteralPath $candidate -PathType Leaf) {
            return (Resolve-Path -LiteralPath $candidate).Path
        }
    }

    if ($env:OS -eq 'Windows_NT') {
        $wrapperRoot = Join-Path $env:USERPROFILE '.m2\wrapper\dists'
        if (Test-Path -LiteralPath $wrapperRoot -PathType Container) {
            $fallback = Get-ChildItem -LiteralPath $wrapperRoot -Filter 'mvn.cmd' -File -Recurse -ErrorAction SilentlyContinue |
                Sort-Object LastWriteTime -Descending |
                Select-Object -First 1
            if ($null -ne $fallback) {
                return $fallback.FullName
            }
        }
    }
    throw 'Maven was not found. Set MAVEN_HOME or M2_HOME, add mvn to PATH, or populate the standard Maven wrapper cache.'
}

function Invoke-OutViewProcess {
    param(
        [Parameter(Mandatory = $true)][string]$FilePath,
        [Parameter(Mandatory = $true)][string[]]$Arguments,
        [Parameter(Mandatory = $true)][string]$WorkingDirectory
    )

    Push-Location $WorkingDirectory
    try {
        & $FilePath @Arguments
        if ($LASTEXITCODE -ne 0) {
            throw "Command failed with exit code $LASTEXITCODE`: $FilePath $($Arguments -join ' ')"
        }
    }
    finally {
        Pop-Location
    }
}

function Assert-OutViewChildPath {
    param(
        [Parameter(Mandatory = $true)][string]$Path,
        [Parameter(Mandatory = $true)][string]$Parent,
        [Parameter(Mandatory = $true)][string]$Purpose
    )

    $parentFull = [System.IO.Path]::GetFullPath($Parent).TrimEnd([System.IO.Path]::DirectorySeparatorChar, [System.IO.Path]::AltDirectorySeparatorChar)
    $pathFull = [System.IO.Path]::GetFullPath($Path)
    $prefix = $parentFull + [System.IO.Path]::DirectorySeparatorChar
    if ($pathFull.Equals($parentFull, [System.StringComparison]::OrdinalIgnoreCase) -or
        -not $pathFull.StartsWith($prefix, [System.StringComparison]::OrdinalIgnoreCase)) {
        throw "Unsafe $Purpose path outside its intended parent: $pathFull"
    }
}
