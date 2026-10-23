$ErrorActionPreference = 'Continue'
$exe = 'C:\Users\Bennett\Documents\PROJECT\agent-quota\bloom\src-tauri\target\debug\bloom.exe'
# stop only this checkout's bloom instance if one is running
Get-Process bloom -ErrorAction SilentlyContinue | Where-Object { $_.Path -eq $exe } | Stop-Process -Force
Start-Sleep -Seconds 2
$env:WEBVIEW2_ADDITIONAL_BROWSER_ARGUMENTS = '--remote-debugging-port=9222'
$p = Start-Process -FilePath $exe -PassThru
Write-Output "bloom pid=$($p.Id) path=$exe started=$($p.StartTime.ToString('o'))"
Start-Sleep -Seconds 6
