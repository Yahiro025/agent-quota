$ErrorActionPreference = 'Stop'
$repo = (Resolve-Path (Join-Path $PSScriptRoot '../../../..')).Path
$output = Join-Path $repo '.debug-launch/deskband-probe'
$vswhere = Join-Path ${env:ProgramFiles(x86)} 'Microsoft Visual Studio/Installer/vswhere.exe'
$vs = & $vswhere -latest -products '*' -requires Microsoft.VisualStudio.Component.VC.Tools.x86.x64 -property installationPath
if (-not $vs) { throw 'Visual C++ Build Tools are required; nothing was installed.' }
$msvc = Get-ChildItem (Join-Path $vs 'VC/Tools/MSVC') -Directory | Sort-Object Name -Descending | Select-Object -First 1
$sdk = Join-Path ${env:ProgramFiles(x86)} 'Windows Kits/10'
$version = Get-ChildItem (Join-Path $sdk 'Include') -Directory | Sort-Object Name -Descending | Select-Object -First 1
$compiler = Join-Path $msvc.FullName 'bin/Hostx64/x64/cl.exe'
$resourceCompiler = Join-Path $sdk "bin/$($version.Name)/x64/rc.exe"
$env:INCLUDE = @((Join-Path $msvc.FullName 'include'), (Join-Path $version.FullName 'ucrt'), (Join-Path $version.FullName 'shared'), (Join-Path $version.FullName 'um')) -join ';'
$env:LIB = @((Join-Path $msvc.FullName 'lib/x64'), (Join-Path $sdk "Lib/$($version.Name)/ucrt/x64"), (Join-Path $sdk "Lib/$($version.Name)/um/x64")) -join ';'
New-Item -ItemType Directory -Force -Path $output | Out-Null
Push-Location $repo
try {
    & go run docs/scratch/taskbar-focus/deskband-probe/frame.go $output
    if ($LASTEXITCODE) { throw 'Frame generation failed' }
} finally { Pop-Location }
$resources = @()
$scales = @(100, 125, 150, 200)
for ($i = 0; $i -lt $scales.Count; $i++) {
    $resources += "$(101+$i) RCDATA `"readout$($scales[$i]).aqf`""
    $resources += "$(111+$i) RCDATA `"popup$($scales[$i]).aqf`""
}
$resources | Set-Content -LiteralPath (Join-Path $output 'frames.rc') -Encoding ascii
Push-Location $output
try {
    & $resourceCompiler /nologo /fo frames.res frames.rc
    if ($LASTEXITCODE) { throw 'Resource compilation failed' }
    $common = @('/nologo', '/utf-8', '/std:c++17', '/EHsc', '/W4', '/WX', '/O2', '/MT', '/DUNICODE', '/D_UNICODE', '/D_WIN32_WINNT=0x0A00', '/I', $PSScriptRoot)
    & $compiler @common (Join-Path $PSScriptRoot 'probe.cpp') frames.res /Fe:probe.exe /link user32.lib gdi32.lib ole32.lib uuid.lib advapi32.lib shell32.lib
    if ($LASTEXITCODE) { throw 'Controller compilation failed' }
    & $compiler @common /LD (Join-Path $PSScriptRoot 'band.cpp') frames.res /Fe:probe-band.dll /link "/DEF:$(Join-Path $PSScriptRoot 'band.def')" user32.lib gdi32.lib ole32.lib uuid.lib
    if ($LASTEXITCODE) { throw 'Deskband compilation failed' }
    Write-Output "Probe binaries: $output"
} finally { Pop-Location }
