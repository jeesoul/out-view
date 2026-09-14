$ErrorActionPreference = 'Stop'
Set-StrictMode -Version Latest

$projectRoot = (Resolve-Path (Join-Path $PSScriptRoot '..\..')).Path
$buildScript = Join-Path $projectRoot 'scripts\build.ps1'
$startScript = Join-Path $projectRoot 'scripts\start-server.ps1'
$installerScript = Join-Path $projectRoot 'build-installer.bat'
$failures = [System.Collections.Generic.List[string]]::new()

function Assert-True {
    param([bool]$Condition, [string]$Message)
    if (-not $Condition) {
        $script:failures.Add($Message)
    }
}

function Invoke-CheckedProcess {
    param([string]$FilePath, [string[]]$Arguments)
    & $FilePath @Arguments | Out-Host
    $processExitCode = $LASTEXITCODE
    return [int]$processExitCode
}

$tempRoot = Join-Path ([System.IO.Path]::GetTempPath()) ("outview-script-tests-{0}" -f [guid]::NewGuid())
New-Item -ItemType Directory -Path $tempRoot | Out-Null

try {
    $pom = [xml](Get-Content -Raw -Encoding UTF8 (Join-Path $projectRoot 'pom.xml'))
    Assert-True ($pom.project.version -eq '1.2.1') "pom.xml project version must be 1.2.1; actual: $($pom.project.version)"

    $cliOutput = Join-Path $tempRoot 'cli-build'
    $code = Invoke-CheckedProcess -FilePath 'powershell.exe' -Arguments @(
        '-NoProfile', '-ExecutionPolicy', 'Bypass', '-File', $buildScript,
        '-ClientMode', 'Cli', '-SkipServer', '-SkipSidecar', '-OutputDirectory', $cliOutput
    )
    Assert-True ($code -eq 0) "CLI build entry point must succeed; exit code: $code"
    $cliBinary = Join-Path $cliOutput 'client\windows\outview-client-cli-windows-amd64.exe'
    Assert-True (Test-Path -LiteralPath $cliBinary -PathType Leaf) 'CLI build must create an explicitly named Windows AMD64 artifact'
    if (Test-Path -LiteralPath $cliBinary -PathType Leaf) {
        $versionText = & $cliBinary -version 2>&1 | Out-String
        Assert-True ($LASTEXITCODE -eq 0) 'CLI -version must exit successfully'
        Assert-True ($versionText -match 'v1\.2\.1\b') "CLI version must come from POM; actual output: $versionText"
    }

    $protectedOutput = Join-Path $tempRoot 'existing-release'
    New-Item -ItemType Directory -Path $protectedOutput | Out-Null
    Set-Content -LiteralPath (Join-Path $protectedOutput 'config.txt') -Value 'user-owned=true' -Encoding UTF8
    $code = Invoke-CheckedProcess -FilePath 'powershell.exe' -Arguments @(
        '-NoProfile', '-ExecutionPolicy', 'Bypass', '-File', $buildScript,
        '-ClientMode', 'Cli', '-SkipServer', '-SkipSidecar', '-OutputDirectory', $protectedOutput
    )
    Assert-True ($code -ne 0) 'Build entry point must refuse an existing destination'
    Assert-True ((Get-Content -Raw -LiteralPath (Join-Path $protectedOutput 'config.txt')).Trim() -eq 'user-owned=true') 'Refusing the destination must preserve user configuration'

    $guiOutput = Join-Path $tempRoot 'gui-build'
    $previousCc = $env:CC
    try {
        $env:CC = 'outview-test-missing-c-compiler'
        $code = Invoke-CheckedProcess -FilePath 'powershell.exe' -Arguments @(
            '-NoProfile', '-ExecutionPolicy', 'Bypass', '-File', $buildScript,
            '-ClientMode', 'Gui', '-SkipServer', '-SkipSidecar', '-OutputDirectory', $guiOutput
        )
    }
    finally {
        $env:CC = $previousCc
    }
    Assert-True ($code -ne 0) 'An explicit GUI build must fail when its C compiler is unavailable'
    Assert-True (-not (Test-Path -LiteralPath $guiOutput)) 'A failed GUI build must not publish a CLI fallback or partial release'

    $fakeJava = Join-Path $tempRoot 'fake-java.cmd'
    Set-Content -LiteralPath $fakeJava -Encoding Ascii -Value @(
        '@echo off',
        'echo %CD%^|%OUTVIEW_DATA_DIR%^|%* > "%OUTVIEW_TEST_CAPTURE%"',
        'exit /b 0'
    )
    $fakeJar = Join-Path $tempRoot 'outview-server.jar'
    Set-Content -LiteralPath $fakeJar -Encoding Ascii -Value 'fixture'
    $capture = Join-Path $tempRoot 'start-capture.txt'
    $dataDir = Join-Path $tempRoot 'persistent-data'
    $env:OUTVIEW_TEST_CAPTURE = $capture
    $code = Invoke-CheckedProcess -FilePath 'powershell.exe' -Arguments @(
        '-NoProfile', '-ExecutionPolicy', 'Bypass', '-File', $startScript,
        '-JavaCommand', $fakeJava, '-JarPath', $fakeJar, '-DataDir', $dataDir
    )
    Assert-True ($code -eq 0) "Server start entry point must propagate the Java exit code; actual: $code"
    Assert-True (Test-Path -LiteralPath $capture -PathType Leaf) 'Server start entry point must invoke Java'
    if (Test-Path -LiteralPath $capture -PathType Leaf) {
        $captured = (Get-Content -Raw -LiteralPath $capture).Trim()
        $expectedData = [System.IO.Path]::GetFullPath($dataDir)
        Assert-True ($captured.StartsWith("$projectRoot|$expectedData|")) "Server must start in the project root with an absolute data directory; actual: $captured"
        Assert-True ($captured -match '-jar\s+"?[^|]*outview-server\.jar') "Server entry point must use -jar; actual: $captured"
    }

    $installerSource = Join-Path $tempRoot 'installer-source'
    New-Item -ItemType Directory -Path (Join-Path $installerSource 'client\windows') -Force | Out-Null
    New-Item -ItemType Directory -Path (Join-Path $installerSource 'webrtc-sidecar\windows') -Force | Out-Null
    foreach ($file in @(
        'client\windows\outview-client-gui-windows-amd64.exe',
        'webrtc-sidecar\windows\outview-sidecar-windows-amd64.exe',
        'README.md',
        'USER_MANUAL.md'
    )) {
        Set-Content -LiteralPath (Join-Path $installerSource $file) -Value 'fixture' -Encoding Ascii
    }
    $fakeIscc = Join-Path $tempRoot 'fake-iscc.cmd'
    Set-Content -LiteralPath $fakeIscc -Encoding Ascii -Value @(
        '@echo off',
        'echo %* > "%OUTVIEW_TEST_CAPTURE%"',
        'exit /b 0'
    )
    $installerCapture = Join-Path $tempRoot 'installer-capture.txt'
    $previousIscc, $previousSource, $previousInstallerOutput = $env:ISCC, $env:OUTVIEW_SOURCE_ROOT, $env:OUTVIEW_INSTALLER_OUTPUT_DIR
    try {
        $env:ISCC = $fakeIscc
        $env:OUTVIEW_SOURCE_ROOT = $installerSource
        $env:OUTVIEW_INSTALLER_OUTPUT_DIR = Join-Path $tempRoot 'installer-output'
        $env:OUTVIEW_TEST_CAPTURE = $installerCapture
        $code = Invoke-CheckedProcess -FilePath 'cmd.exe' -Arguments @('/d', '/c', $installerScript)
    }
    finally {
        $env:ISCC, $env:OUTVIEW_SOURCE_ROOT, $env:OUTVIEW_INSTALLER_OUTPUT_DIR = $previousIscc, $previousSource, $previousInstallerOutput
    }
    Assert-True ($code -eq 0) "Installer wrapper must invoke the configured compiler; exit code: $code"
    Assert-True (Test-Path -LiteralPath $installerCapture -PathType Leaf) 'Installer wrapper must invoke ISCC'
    if (Test-Path -LiteralPath $installerCapture -PathType Leaf) {
        $installerArguments = Get-Content -Raw -LiteralPath $installerCapture
        Assert-True ($installerArguments -match '/DAppVersion=1\.2\.1\b') "Installer version must come from POM; actual arguments: $installerArguments"
        Assert-True ($installerArguments -match '/DSourceRoot=') 'Installer wrapper must pass the configurable source root'
    }
}
finally {
    Remove-Item Env:OUTVIEW_TEST_CAPTURE -ErrorAction SilentlyContinue
    if (Test-Path -LiteralPath $tempRoot) {
        $expectedPrefix = [System.IO.Path]::GetFullPath([System.IO.Path]::GetTempPath()).TrimEnd('\') + '\outview-script-tests-'
        if (-not [System.IO.Path]::GetFullPath($tempRoot).StartsWith($expectedPrefix, [System.StringComparison]::OrdinalIgnoreCase)) {
            throw "Refusing to remove unexpected test directory: $tempRoot"
        }
        Remove-Item -LiteralPath $tempRoot -Recurse -Force
    }
}

if ($failures.Count -gt 0) {
    $failures | ForEach-Object { Write-Error $_ -ErrorAction Continue }
    exit 1
}

Write-Host 'Build script contract tests passed.'
