# Builds PhantomArt for Windows and bundles Xray-core next to it.
# Usage (PowerShell, from the repository root):  .\scripts\build.ps1
param(
    [string]$XrayVersion = "latest"
)
$ErrorActionPreference = "Stop"

$root = Split-Path -Parent $PSScriptRoot
$out  = Join-Path $root "build\PhantomArt"
$core = Join-Path $out "core"
New-Item -ItemType Directory -Force -Path $core | Out-Null

Write-Host "Building PhantomArt.exe..."
Push-Location $root
go build -tags desktop,production -trimpath -ldflags "-H windowsgui -s -w" -o (Join-Path $out "PhantomArt.exe") .
Pop-Location

if (-not (Test-Path (Join-Path $core "xray.exe"))) {
    if ($XrayVersion -eq "latest") {
        $url = "https://github.com/XTLS/Xray-core/releases/latest/download/Xray-windows-64.zip"
    } else {
        $url = "https://github.com/XTLS/Xray-core/releases/download/$XrayVersion/Xray-windows-64.zip"
    }
    $zip = Join-Path $env:TEMP "xray-windows-64.zip"
    Write-Host "Downloading Xray-core from $url ..."
    Invoke-WebRequest -Uri $url -OutFile $zip
    Expand-Archive -Path $zip -DestinationPath $core -Force
    Remove-Item $zip
}

Write-Host "Done: $out"
