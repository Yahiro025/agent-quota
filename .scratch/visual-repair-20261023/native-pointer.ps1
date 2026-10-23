param([int]$X = 960, [int]$Y = 2, [switch]$Click, [switch]$Restore)
Add-Type -TypeDefinition @'
using System;
using System.Runtime.InteropServices;
public static class NativePointer {
  [StructLayout(LayoutKind.Sequential)] public struct POINT { public int x,y; }
  [DllImport("user32.dll")] public static extern IntPtr SetThreadDpiAwarenessContext(IntPtr value);
  [DllImport("user32.dll")] public static extern bool GetCursorPos(out POINT point);
  [DllImport("user32.dll")] public static extern bool SetCursorPos(int x, int y);
  [DllImport("user32.dll")] public static extern IntPtr WindowFromPoint(POINT point);
  [DllImport("user32.dll")] public static extern IntPtr GetAncestor(IntPtr window, uint flags);
  [DllImport("user32.dll")] public static extern uint GetWindowThreadProcessId(IntPtr window, out uint process);
  [DllImport("user32.dll")] public static extern IntPtr GetForegroundWindow();
  [DllImport("user32.dll")] public static extern void mouse_event(uint flags, uint dx, uint dy, uint data, UIntPtr extra);
}
'@
[void][NativePointer]::SetThreadDpiAwarenessContext([IntPtr]::new(-4))
$saved = Join-Path $PSScriptRoot 'cursor.json'
if ($Restore) {
  if (Test-Path -LiteralPath $saved) { $point = Get-Content -LiteralPath $saved | ConvertFrom-Json; [void][NativePointer]::SetCursorPos($point.x,$point.y) }
  exit
}
if (!(Test-Path -LiteralPath $saved)) { $point = New-Object NativePointer+POINT; [void][NativePointer]::GetCursorPos([ref]$point); $point | ConvertTo-Json | Set-Content -LiteralPath $saved }
[void][NativePointer]::SetCursorPos($X,$Y)
Start-Sleep -Milliseconds 700
if ($Click) {
  $point = New-Object NativePointer+POINT; $point.x=$X; $point.y=$Y
  $root = [NativePointer]::GetAncestor([NativePointer]::WindowFromPoint($point),2)
  [uint32]$ownerId=0
  [void][NativePointer]::GetWindowThreadProcessId($root,[ref]$ownerId)
  $owner = Get-Process -Id $ownerId -ErrorAction Stop
  if ($owner.Path -ne 'C:\Users\Bennett\Documents\PROJECT\agent-quota\bloom\src-tauri\target\debug\bloom.exe') { throw "Refusing click on non-Bloom window owned by $($owner.ProcessName)" }
  $before = [NativePointer]::GetForegroundWindow()
  [NativePointer]::mouse_event(2,0,0,0,[UIntPtr]::Zero)
  [NativePointer]::mouse_event(4,0,0,0,[UIntPtr]::Zero)
  Start-Sleep -Milliseconds 700
  $after = [NativePointer]::GetForegroundWindow()
  Write-Output "Actual native click delivered to Bloom PID $ownerId; foreground unchanged=$($before -eq $after)"
}
