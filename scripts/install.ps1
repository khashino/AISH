$ErrorActionPreference = "Stop"

$repo = "khashino/AISH"
$version = if ($env:AISH_VERSION) { $env:AISH_VERSION } else { "v0.6.4" }

if (-not $version.StartsWith("v")) {
    $version = "v$version"
}

$versionNumber = $version.TrimStart("v")
$asset = "aish-v$versionNumber-windows-amd64.exe"
$url = "https://github.com/$repo/releases/download/$version/$asset"

$dest = if ($env:AISH_INSTALL_DIR) {
    $env:AISH_INSTALL_DIR
} else {
    "$env:LOCALAPPDATA\AISH\bin"
}

New-Item -ItemType Directory -Force -Path $dest | Out-Null

$tempFile = Join-Path $env:TEMP $asset
$target = Join-Path $dest "aish.exe"

Write-Host "Downloading AISH $version..."
Write-Host $url

curl.exe -fL $url -o $tempFile

if ($LASTEXITCODE -ne 0) {
    throw "Download failed: $url"
}

Move-Item -Force $tempFile $target

$userPath = [Environment]::GetEnvironmentVariable("Path", "User")

if ($userPath -notlike "*$dest*") {
    $newPath = if ([string]::IsNullOrWhiteSpace($userPath)) {
        $dest
    } else {
        "$userPath;$dest"
    }

    [Environment]::SetEnvironmentVariable("Path", $newPath, "User")
    Write-Host "Added AISH to your user PATH."
}

Write-Host ""
Write-Host "Installed AISH to:"
Write-Host "  $target"
Write-Host ""
Write-Host "Open a new PowerShell window and run:"
Write-Host "  aish version"
Write-Host "  aish setup"