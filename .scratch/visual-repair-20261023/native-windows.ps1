Add-Type -TypeDefinition @'
using System;
using System.Runtime.InteropServices;
public static class WCheck {
  [StructLayout(LayoutKind.Sequential)] public struct RECT { public int l,t,r,b; }
  [DllImport("user32.dll")] public static extern IntPtr SetThreadDpiAwarenessContext(IntPtr v);
  [DllImport("user32.dll")] public static extern bool EnumWindows(EnumWindowsProc cb, IntPtr lp);
  public delegate bool EnumWindowsProc(IntPtr h, IntPtr l);
  [DllImport("user32.dll")] public static extern uint GetWindowThreadProcessId(IntPtr w, out uint p);
  [DllImport("user32.dll")] public static extern int GetWindowLong(IntPtr w, int i);
  [DllImport("user32.dll")] public static extern bool GetWindowRect(IntPtr w, out RECT r);
  [DllImport("user32.dll")] public static extern bool IsWindowVisible(IntPtr w);
}
'@
[void][WCheck]::SetThreadDpiAwarenessContext([IntPtr]::new(-4))
$targets = @("bloom","Dock_64")
$cb = [WCheck+EnumWindowsProc]{ param($h,$l)
  [uint32]$p=0; [void][WCheck]::GetWindowThreadProcessId($h,[ref]$p)
  $proc = Get-Process -Id $p -ErrorAction SilentlyContinue
  if ($proc -and $targets -contains $proc.ProcessName) {
    $ex = [WCheck]::GetWindowLong($h,-20)
    $r = New-Object WCheck+RECT; [void][WCheck]::GetWindowRect($h,[ref]$r)
    $vis = [WCheck]::IsWindowVisible($h)
    Write-Output ("{0} hwnd={1} exstyle=0x{2:x8} transparent={3} vis={4} rect=({5},{6})-({7},{8})" -f $proc.ProcessName,$h,$ex,(($ex -band 0x20) -ne 0),$vis,$r.l,$r.t,$r.r,$r.b)
  }
  return $true
}
[void][WCheck]::EnumWindows($cb,[IntPtr]::Zero)
