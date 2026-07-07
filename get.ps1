# remagic bootstrap for Windows: download the prebuilt CLI and run setup.
#
#   irm https://raw.githubusercontent.com/maximerivest/remagic/main/get.ps1 | iex
#
# The binary lands in $env:LOCALAPPDATA\remagic\remagic.exe and `remagic setup`
# runs immediately. Re-running is safe; it just refreshes the binary.
$ErrorActionPreference = "Stop"

$repo = "maximerivest/remagic"
$arch = if ([System.Runtime.InteropServices.RuntimeInformation]::OSArchitecture -eq "Arm64") { "arm64" } else { "amd64" }
$asset = "remagic-windows-$arch.exe"
$url = "https://github.com/$repo/releases/latest/download/$asset"

$dir = Join-Path $env:LOCALAPPDATA "remagic"
New-Item -ItemType Directory -Force -Path $dir | Out-Null
$dest = Join-Path $dir "remagic.exe"

Write-Host "==> downloading $asset (latest release)"
Invoke-WebRequest -Uri $url -OutFile $dest
Write-Host "  ✓ installed: $dest"

# Add to the user PATH if it isn't there yet (takes effect in new shells).
$userPath = [Environment]::GetEnvironmentVariable("Path", "User")
if ($userPath -notlike "*$dir*") {
    [Environment]::SetEnvironmentVariable("Path", "$userPath;$dir", "User")
    Write-Host "  ✓ added $dir to your PATH (new terminals will have 'remagic')"
}

if ($env:REMAGIC_NO_SETUP -eq "1") {
    Write-Host "==> done. Run '$dest setup' when your tablet is connected."
    exit 0
}
Write-Host "==> starting setup (Ctrl-C to stop; run '$dest setup' any time)"
& $dest setup
