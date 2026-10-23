$ErrorActionPreference = 'Stop'
$env:PATH = 'C:\Users\Bennett\AppData\Local\Kiro-Cli;C:\Users\Bennett\.cargo\bin;C:\Program Files\nodejs;' + $env:PATH
Set-Location 'C:\Users\Bennett\Documents\PROJECT\agent-quota\bloom'
& 'C:\Users\Bennett\AppData\Local\Kiro-Cli\bun.exe' run tauri build --debug --no-bundle
exit $LASTEXITCODE
