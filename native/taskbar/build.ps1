param([string]$Output = '.debug-launch/focus-build', [switch]$ProtocolOnly)
$ErrorActionPreference = 'Stop'
$repo = (Resolve-Path (Join-Path $PSScriptRoot '../..')).Path
$outputPath = [IO.Path]::GetFullPath((Join-Path $repo $Output))
$vswhere = Join-Path ${env:ProgramFiles(x86)} 'Microsoft Visual Studio/Installer/vswhere.exe'
$vs = & $vswhere -latest -products '*' -requires Microsoft.VisualStudio.Component.VC.Tools.x86.x64 -property installationPath
if (-not $vs) { throw 'Visual C++ Build Tools are required to build the native host.' }
$msvc = Get-ChildItem (Join-Path $vs 'VC/Tools/MSVC') -Directory | Sort-Object Name -Descending | Select-Object -First 1
$sdk = Join-Path ${env:ProgramFiles(x86)} 'Windows Kits/10'
$version = Get-ChildItem (Join-Path $sdk 'Include') -Directory | Sort-Object Name -Descending | Select-Object -First 1
$compiler = Join-Path $msvc.FullName 'bin/Hostx64/x64/cl.exe'
$env:INCLUDE = @((Join-Path $msvc.FullName 'include'), (Join-Path $version.FullName 'ucrt'), (Join-Path $version.FullName 'shared'), (Join-Path $version.FullName 'um')) -join ';'
$env:LIB = @((Join-Path $msvc.FullName 'lib/x64'), (Join-Path $sdk "Lib/$($version.Name)/ucrt/x64"), (Join-Path $sdk "Lib/$($version.Name)/um/x64")) -join ';'
New-Item -ItemType Directory -Force -Path $outputPath | Out-Null
Push-Location $outputPath
try {
 $common=@('/nologo','/utf-8','/std:c++17','/EHsc','/W4','/WX','/O2','/MT','/guard:cf','/DUNICODE','/D_UNICODE','/D_WIN32_WINNT=0x0A00','/I',$PSScriptRoot)
 & $compiler @common (Join-Path $PSScriptRoot 'protocol_test.cpp') /Fe:protocol-test.exe
 if ($LASTEXITCODE) {throw 'Native protocol test build failed'}
 & .\protocol-test.exe
 if ($LASTEXITCODE) {throw 'Native protocol tests failed'}
 if ($ProtocolOnly) {return}
 & $compiler @common (Join-Path $PSScriptRoot 'host_test.cpp') /Fe:host-test.exe /link user32.lib gdi32.lib ole32.lib uuid.lib uxtheme.lib msimg32.lib
 if ($LASTEXITCODE) {throw 'Native lifecycle test build failed'}
 & .\host-test.exe
 if ($LASTEXITCODE) {throw 'Native lifecycle tests failed'}
 $identity = ((Get-FileHash (Join-Path $PSScriptRoot 'host.cpp')).Hash + (Get-FileHash (Join-Path $PSScriptRoot 'protocol.h')).Hash)
 $sha = [Security.Cryptography.SHA256]::Create()
 try {$buildHex=([BitConverter]::ToString($sha.ComputeHash([Text.Encoding]::UTF8.GetBytes($identity)))).Replace('-','').Substring(0,8)} finally {$sha.Dispose()}
 $dll="agentquota-taskbar-$($buildHex.ToLower()).dll"
 & $compiler @common "/DAQ_HOST_BUILD=0x$buildHex" /LD (Join-Path $PSScriptRoot 'host.cpp') "/Fe:$dll" /link "/DEF:$(Join-Path $PSScriptRoot 'host.def')" user32.lib gdi32.lib ole32.lib uuid.lib uxtheme.lib msimg32.lib /DYNAMICBASE /NXCOMPAT
 if ($LASTEXITCODE) {throw 'Native host build failed'}
 [pscustomobject]@{build=[Convert]::ToUInt32($buildHex,16);dll=$dll;sha256=(Get-FileHash -LiteralPath $dll).Hash} | ConvertTo-Json | Set-Content taskbar-host.json -Encoding utf8
 Push-Location $repo
 try {& go build -ldflags '-H=windowsgui' -o (Join-Path $outputPath 'agentquota.exe') .; if ($LASTEXITCODE) {throw 'Go application build failed'}} finally {Pop-Location}
 Write-Output "Build ready: $outputPath"
} finally {Pop-Location}
