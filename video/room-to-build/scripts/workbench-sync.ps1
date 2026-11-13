$ErrorActionPreference = 'Stop'
$VideoRoot = [System.IO.Path]::GetFullPath((Join-Path $PSScriptRoot '..'))
$SkillRoot = 'C:\Users\Bennett\.agents\skills\video-shotcraft'
$WorkbenchRoot = Join-Path $SkillRoot 'workbench'
$DemoPath = Join-Path $WorkbenchRoot 'demosrc'
if (Test-Path -LiteralPath $DemoPath -PathType Leaf) {
    $LinkText = (Get-Content -LiteralPath $DemoPath -Raw).Trim()
    if ($LinkText -ne '../demos') { throw 'Unexpected demosrc file; preserve it.' }
    Copy-Item -LiteralPath $DemoPath -Destination (Join-Path $WorkbenchRoot 'demosrc.symlink-reference')
    Remove-Item -LiteralPath $DemoPath
    Copy-Item -LiteralPath (Join-Path $SkillRoot 'demos') -Destination $DemoPath -Recurse
}
New-Item -ItemType Directory -Force -Path (Join-Path $WorkbenchRoot 'proj') | Out-Null
Copy-Item -Path (Join-Path $VideoRoot 'src\*') -Destination (Join-Path $WorkbenchRoot 'proj') -Recurse -Force
Copy-Item -Path (Join-Path $VideoRoot 'public\*') -Destination (Join-Path $WorkbenchRoot 'public') -Recurse -Force
foreach ($Name in @('cardpreviews','sfxlib','bgmlib','legacy-textures')) {
    New-Item -ItemType Directory -Force -Path (Join-Path $WorkbenchRoot "public\$Name") | Out-Null
}
New-Item -ItemType Directory -Force -Path (Join-Path $WorkbenchRoot 'public\textures\live') | Out-Null
Write-Output "Windows workbench copy synchronized from $VideoRoot"
