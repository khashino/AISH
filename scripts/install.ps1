$ErrorActionPreference = "Stop"

$repo = if ($env:AISH_REPO) {
    $env:AISH_REPO
} else {
    "khashino/AISH"
}

$requestedVersion = if ($env:AISH_VERSION) {
    $env:AISH_VERSION
} else {
    "latest"
}

$installDir = if ($env:AISH_INSTALL_DIR) {
    $env:AISH_INSTALL_DIR
} else {
    "$env:LOCALAPPDATA\AISH\bin"
}

if ($requestedVersion -eq "latest") {
    Write-Host "Finding latest AISH release..."

    $latestUrl = curl.exe -fsSL `
        -o NUL `
        -w "%{url_effective}" `
        "https://github.com/$repo/releases/latest"

    if ($LASTEXITCODE -ne 0) {
        throw "Unable to determine the latest AISH release."
    }

    $version = ($latestUrl.TrimEnd("/") -split "/")[-1]
} else {
    $version = $requestedVersion

    if (-not $version.StartsWith("v")) {
        $version = "v$version"
    }
}

$versionNumber = $version.TrimStart("v")
$asset = "aish-v$versionNumber-windows-amd64.exe"
$url = "https://github.com/$repo/releases/download/$version/$asset"

New-Item -ItemType Directory -Force -Path $installDir | Out-Null

$tempFile = Join-Path $env:TEMP "aish-download.exe"
$destination = Join-Path $installDir "aish.exe"

Write-Host "Downloading AISH $version..."
Write-Host "Asset: $asset"

curl.exe -fL --progress-bar $url -o $tempFile

if ($LASTEXITCODE -ne 0) {
    Remove-Item $tempFile -ErrorAction SilentlyContinue
    throw "Download failed: $url"
}

Move-Item -Force $tempFile $destination

$userPath = [Environment]::GetEnvironmentVariable("Path", "User")

if ($userPath -notlike "*$installDir*") {
    $newPath = if ([string]::IsNullOrWhiteSpace($userPath)) {
        $installDir
    } else {
        "$userPath;$installDir"
    }

    [Environment]::SetEnvironmentVariable("Path", $newPath, "User")
    Write-Host "Added AISH to your user PATH."
}

Write-Host ""
Write-Host "Installed AISH $version:"
Write-Host "  $destination"
Write-Host ""
Write-Host "Open a new PowerShell window, then run:"
Write-Host "  aish setup"
Write-Host "  aish doctor"