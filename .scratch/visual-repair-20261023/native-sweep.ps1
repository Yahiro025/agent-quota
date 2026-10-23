param([int]$X = 960, [int]$YTop = 0, [int]$YEnd = 30, [int]$StepMs = 25, [switch]$Click)
# Continuous edge-to-notch pointer sweep through real OS input (SendInput,
# absolute move). Samples WindowFromPoint's root owner at each step so the
# trace shows exactly which process owns the point and when ownership flips.
Add-Type -TypeDefinition @'
using System;
using System.Runtime.InteropServices;
public static class SweepPtr {
  [StructLayout(LayoutKind.Sequential)] public struct POINT { public int x,y; }
  [StructLayout(LayoutKind.Sequential)] public struct INPUT { public uint type; public MOUSEINPUT mi; }
  [StructLayout(LayoutKind.Sequential)] public struct MOUSEINPUT { public int dx,dy; public uint data,flags,time; public IntPtr extra; }
  [DllImport("user32.dll")] public static extern IntPtr SetThreadDpiAwarenessContext(IntPtr value);
  [DllImport("user32.dll")] public static extern uint SendInput(uint n, INPUT[] i, int sz);
  [DllImport("user32.dll")] public static extern bool GetCursorPos(out POINT point);
  [DllImport("user32.dll")] public static extern IntPtr WindowFromPoint(POINT point);
  [DllImport("user32.dll")] public static extern IntPtr GetAncestor(IntPtr window, uint flags);
  [DllImport("user32.dll")] public static extern uint GetWindowThreadProcessId(IntPtr window, out uint process);
  [DllImport("user32.dll")] public static extern IntPtr GetForegroundWindow();
  [DllImport("user32.dll")] public static extern void mouse_event(uint flags, uint dx, uint dy, uint data, UIntPtr extra);
  [DllImport("user32.dll")] public static extern int GetSystemMetrics(int n);
  public static void MoveAbs(int x, int y) {
    var i = new INPUT[1];
    i[0].type = 0;
    i[0].mi.dx = (int)Math.Round(x * 65535.0 / (GetSystemMetrics(0) - 1));
    i[0].mi.dy = (int)Math.Round(y * 65535.0 / (GetSystemMetrics(1) - 1));
    i[0].mi.flags = 0x8000 | 0x0001; // ABSOLUTE | MOVE
    SendInput(1, i, Marshal.SizeOf(typeof(INPUT)));
  }
}
'@
[void][SweepPtr]::SetThreadDpiAwarenessContext([IntPtr]::new(-4))
$BloomPath = 'C:\Users\Bennett\Documents\PROJECT\agent-quota\bloom\src-tauri\target\debug\bloom.exe'
$owner = { param($x,$y)
  $p = New-Object SweepPtr+POINT; $p.x=$x; $p.y=$y
  $root = [SweepPtr]::GetAncestor([SweepPtr]::WindowFromPoint($p),2)
  [uint32]$ownerPid=0; [void][SweepPtr]::GetWindowThreadProcessId($root,[ref]$ownerPid)
  $proc = Get-Process -Id $ownerPid -ErrorAction SilentlyContinue
  return "{0}|{1}|{2}" -f $ownerPid, ($proc.ProcessName), ($proc.Path)
}
Write-Output "sweep start: cursor park at ($X,$YTop)"
[SweepPtr]::MoveAbs($X,$YTop)
Start-Sleep -Milliseconds 900
Write-Output ("parked -> " + (& $owner $X $YTop))
for ($y = $YTop + 2; $y -le $YEnd; $y += 2) {
  [SweepPtr]::MoveAbs($X,$y)
  Start-Sleep -Milliseconds $StepMs
  Write-Output ("y={0} -> " -f $y + (& $owner $X $y))
}
Start-Sleep -Milliseconds 300
Write-Output ("rest($YEnd) -> " + (& $owner $X $YEnd))
if ($Click) {
  $p = New-Object SweepPtr+POINT; $p.x=$X; $p.y=$YEnd
  $root = [SweepPtr]::GetAncestor([SweepPtr]::WindowFromPoint($p),2)
  [uint32]$ownerPid=0; [void][SweepPtr]::GetWindowThreadProcessId($root,[ref]$ownerPid)
  $proc = Get-Process -Id $ownerPid -ErrorAction Stop
  if ($proc.Path -ne $BloomPath) { throw "Refusing click on non-Bloom window owned by $($proc.ProcessName) ($($proc.Path))" }
  $before = [SweepPtr]::GetForegroundWindow()
  [SweepPtr]::mouse_event(2,0,0,0,[UIntPtr]::Zero)
  [SweepPtr]::mouse_event(4,0,0,0,[UIntPtr]::Zero)
  Start-Sleep -Milliseconds 700
  $after = [SweepPtr]::GetForegroundWindow()
  Write-Output "native click delivered to Bloom PID $ownerPid; foreground unchanged=$($before -eq $after)"
}
