param([string]$out)
Add-Type -AssemblyName System.Windows.Forms
Add-Type -AssemblyName System.Drawing
Add-Type -MemberDefinition '[DllImport("user32.dll")] public static extern IntPtr SetThreadDpiAwarenessContext(IntPtr c);' -Name W -Namespace P
[P.W]::SetThreadDpiAwarenessContext([IntPtr]::New(-4)) | Out-Null
$bounds = [System.Windows.Forms.Screen]::PrimaryScreen.Bounds
$w = [Math]::Min(1200, $bounds.Width); $x = [Math]::Max(0, [int]($bounds.Width / 2 - $w / 2)); $y = 0; $h = [Math]::Min(500, $bounds.Height)
$bmp = New-Object System.Drawing.Bitmap $w, $h
$g = [System.Drawing.Graphics]::FromImage($bmp)
$g.CopyFromScreen($x, $y, 0, 0, $bmp.Size)
$bmp.Save($out, [System.Drawing.Imaging.ImageFormat]::Png)
Write-Output "saved $out region=${x},${y},${w}x${h} screen=$($bounds.Width)x$($bounds.Height)"
