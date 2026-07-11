$ErrorActionPreference = "Stop"
$repo = if ($env:AISH_REPO) { $env:AISH_REPO } else { "khashino/AISH" }
$version = if ($env:AISH_VERSION) { $env:AISH_VERSION } else { "latest" }
$base = "https://github.com/$repo/releases"
$url = if ($version -eq "latest") { "$base/latest/download/aish-windows-amd64.exe" } else { "$base/download/$version/aish-windows-amd64.exe" }
$dest = if ($env:AISH_INSTALL_DIR) { $env:AISH_INSTALL_DIR } else { "$env:LOCALAPPDATA\AISH\bin" }
New-Item -ItemType Directory -Force -Path $dest | Out-Null
Invoke-WebRequest $url -OutFile "$dest\aish.exe"
Write-Host "Installed AISH to $dest\aish.exe"
