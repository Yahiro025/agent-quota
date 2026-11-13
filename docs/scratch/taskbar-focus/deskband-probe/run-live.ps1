# Native probe only. All temporary shell changes are reversed in finally.
$ErrorActionPreference = 'Stop'
$repo = (Resolve-Path (Join-Path $PSScriptRoot '../../../..')).Path
$output = Join-Path $repo '.debug-launch/deskband-probe'
$evidence = Join-Path $repo 'docs/verification/taskbar-focus/deskband-probe'
$probe = Join-Path $output 'probe.exe'
$finish = Join-Path $output 'finish.flag'
if (Test-Path -LiteralPath $finish) { throw 'Old finish flag exists; preserve its previous run before starting another.' }
$script:commandNumber = 0
function Invoke-Probe([string[]] $Arguments, [string] $Record) {
    $script:commandNumber++
    $stdout = Join-Path $output "command-$($script:commandNumber).out"
    $stderr = Join-Path $output "command-$($script:commandNumber).err"
    if ($Arguments | Where-Object { $_ -notmatch '^[a-z0-9-]+$' }) { throw 'Unexpected probe argument' }
    $start = New-Object System.Diagnostics.ProcessStartInfo
    $start.FileName = $probe
    $start.Arguments = $Arguments -join ' '
    $start.UseShellExecute = $false
    $start.CreateNoWindow = $true
    $start.RedirectStandardOutput = $true
    $start.RedirectStandardError = $true
    $child = New-Object System.Diagnostics.Process
    $child.StartInfo = $start
    [void]$child.Start()
    $stdoutTask = $child.StandardOutput.ReadToEndAsync()
    $stderrTask = $child.StandardError.ReadToEndAsync()
    if (-not $child.WaitForExit(12000)) {
        Stop-Process -Id $child.Id -Force
        throw "Probe command timed out: $Arguments"
    }
    $text = $stdoutTask.GetAwaiter().GetResult()
    $text | Set-Content -LiteralPath $stdout -Encoding utf8
    $stderrTask.GetAwaiter().GetResult() | Set-Content -LiteralPath $stderr -Encoding utf8
    if ($Record) { $text | Set-Content -LiteralPath (Join-Path $evidence $Record) -Encoding utf8 }
    if ($child.ExitCode) { throw "Probe command failed ($($child.ExitCode)): $Arguments $text" }
    return $text
}
$registered = $false
$originalState = $null
try {
    $before = Invoke-Probe @('status') 'before-geometry.json' | ConvertFrom-Json
    $originalState = $before.appbar_state
    if (Test-Path -LiteralPath 'HKCU:\Software\Classes\CLSID\{4F66CA94-A701-4FC2-8EA5-C2152F376B90}') { throw 'Probe key already exists' }
    $registered = $true
    Invoke-Probe @('register') 'register.txt' | Write-Output
    Invoke-Probe @('activate') 'activate.txt' | Write-Output
    Invoke-Probe @('categories') 'categories.txt' | Write-Output
    Invoke-Probe @('show') 'show.txt' | Write-Output
    Start-Sleep -Milliseconds 300
    Invoke-Probe @('status') 'shown-geometry.json' | Write-Output
    Write-Output 'LIVE_PROBE_READY — automatic cleanup after five minutes or finish.flag'
    $deadline = (Get-Date).AddMinutes(5)
    while ((Get-Date) -lt $deadline -and -not (Test-Path -LiteralPath $finish)) { Start-Sleep -Seconds 1 }
} finally {
    # Restoration has an independent process and deadline, even if the tested command stalls.
    if ($null -ne $originalState) {
        try { Invoke-Probe @('restore-state', [string]$originalState) 'restore-state.txt' | Write-Output } catch { Write-Warning $_ }
    }
    try { Invoke-Probe @('quit') 'controller-quit.txt' | Write-Output } catch { Write-Warning $_ }
    if ($registered) {
        try { Invoke-Probe @('hide') 'hide.txt' | Write-Output } catch { Write-Warning $_ }
        try { Invoke-Probe @('status') 'hidden-geometry.json' | Write-Output } catch { Write-Warning $_ }
        try { Invoke-Probe @('unregister') 'unregister.txt' | Write-Output } catch { Write-Warning $_ }
    }
    try { Invoke-Probe @('status') 'after-geometry.json' | Write-Output } catch { Write-Warning $_ }
    $baseline = Get-Content -LiteralPath (Join-Path $evidence 'before.json') -Raw | ConvertFrom-Json
    $appProcess = Get-Process agentquota -ErrorAction SilentlyContinue
    [pscustomobject]@{
        Time=(Get-Date -Format o)
        InstalledPath=$appProcess.Path
        InstalledPID=$appProcess.Id
        InstalledSHA256=(Get-FileHash -LiteralPath $baseline.InstalledPath).Hash
        SettingsSHA256=(Get-FileHash -LiteralPath (Join-Path $env:APPDATA 'agent-quota/settings.json')).Hash
        ExplorerPID=(Get-Process explorer).Id
        ProbeRegistryKeyAbsent=(-not (Test-Path -LiteralPath 'HKCU:\Software\Classes\CLSID\{4F66CA94-A701-4FC2-8EA5-C2152F376B90}'))
    } | ConvertTo-Json | Set-Content -LiteralPath (Join-Path $evidence 'after.json') -Encoding utf8
    if (Test-Path -LiteralPath (Join-Path $output 'events.log')) {
        Copy-Item -LiteralPath (Join-Path $output 'events.log') -Destination (Join-Path $evidence 'events.log') -Force
    }
    Write-Output 'LIVE_PROBE_CLEANUP_FINISHED'
}
