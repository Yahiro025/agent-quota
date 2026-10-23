param([int]$X = 960, [int]$Y = 14)
# Park inside the revealed notch via real input, then enumerate top-level
# windows for bloom/Dock_64: exstyle transparency, visibility, rects.
Add-Type -TypeDefinition @'
using System;
using System.Runtime.InteropServices;
public static class WC {
  [StructLayout(LayoutKind.Sequential)] public struct RECT { public int l,t,r,b; }
  [StructLayout(LayoutKind.Sequential)] public struct POINT { public int x,y; }
  [StructLayout(LayoutKind.Sequential)] public struct INPUT { public uint type; public MOUSEINPUT mi; }
  [StructLayout(LayoutKind.Sequential)] public struct MOUSEINPUT { public int dx,dy; public uint data,flags,time; public IntPtr extra; }
  [DllImport("user32.dll")] public static extern IntPtr SetThreadDpiAwarenessContext(IntPtr v);
  [DllImport("user32.dll")] public static extern uint SendInput(uint n, INPUT[] i, int sz);
  [DllImport("user32.dll")] public static extern int GetSystemMetrics(int n);
  [DllImport("user32.dll")] public static extern bool EnumWindows(EnumWindowsProc cb, IntPtr lp);
  public delegate bool EnumWindowsProc(IntPtr h, IntPtr l);
  [DllImport("user32.dll")] public static extern uint GetWindowThreadProcessId(IntPtr w, out uint p);
  [DllImport("user32.dll")] public static extern int GetWindowLong(IntPtr w, int i);
  [DllImport("user32.dll")] public static extern bool GetWindowRect(IntPtr w, out RECT r);
  [DllImport("user32.dll")] public static extern bool IsWindowVisible(IntPtr w);
  [DllImport("user32.dll")] public static extern IntPtr WindowFromPoint(POINT p);
  [DllImport("user32.dll")] public static extern IntPtr GetAncestor(IntPtr w, uint f);
  public static void MoveAbs(int x, int y) {
    var i = new INPUT[1];
    i[0].type = 0;
    i[0].mi.dx = (int)Math.Round(x * 65535.0 / (GetSystemMetrics(0) - 1));
    i[0].mi.dy = (int)Math.Round(y * 65535.0 / (GetSystemMetrics(1) - 1));
    i[0].mi.flags = 0x8000 | 0x0001;
    SendInput(1, i, Marshal.SizeOf(typeof(INPUT)));
  }
}
'@
[void][WC]::SetThreadDpiAwarenessContext([IntPtr]::new(-4))
# Dwell in the band to arm the reveal, then step down into the notch.
[WC]::MoveAbs($X, 0); Start-Sleep -Milliseconds 900
foreach ($y in 2,6,10,$Y) { [WC]::MoveAbs($X,$y); Start-Sleep -Milliseconds 120 }
Start-Sleep -Milliseconds 400
$targets = @("bloom","Dock_64")
$cb = [WC+EnumWindowsProc]{ param($h,$l)
  [uint32]$p=0; [void][WC]::GetWindowThreadProcessId($h,[ref]$p)
  $proc = Get-Process -Id $p -ErrorAction SilentlyContinue
  if ($proc -and $targets -contains $proc.ProcessName) {
    $ex = [WC]::GetWindowLong($h,-20)
    $r = New-Object WC+RECT; [void][WC]::GetWindowRect($h,[ref]$r)
    $vis = [WC]::IsWindowVisible($h)
    Write-Output ("{0} hwnd={1} ex=0x{2:x8} transparent={3} vis={4} rect=({5},{6})-({7},{8})" -f $proc.ProcessName,$h,$ex,(($ex -band 0x20) -ne 0),$vis,$r.l,$r.t,$r.r,$r.b)
  }
  return $true
}
[void][WC]::EnumWindows($cb,[IntPtr]::Zero)
$pt = New-Object WC+POINT; $pt.x=$X; $pt.y=$Y
$root = [WC]::GetAncestor([WC]::WindowFromPoint($pt),2)
[uint32]$hitPid=0; [void][WC]::GetWindowThreadProcessId($root,[ref]$hitPid)
$hp = Get-Process -Id $hitPid -ErrorAction SilentlyContinue
Write-Output ("hit-at($X,$Y) -> {0}|{1}" -f $hitPid, ($hp.ProcessName))
