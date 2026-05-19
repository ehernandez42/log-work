$ErrorActionPreference = "Stop"

New-Item -ItemType Directory -Force -Path "dist" | Out-Null

$env:CGO_ENABLED = "0"

$env:GOOS = "windows"
$env:GOARCH = "amd64"
go build -trimpath -ldflags "-s -w" -o "dist/log-work.exe" .

$env:GOOS = "linux"
$env:GOARCH = "amd64"
go build -trimpath -ldflags "-s -w" -o "dist/log-work-linux-amd64" .

Remove-Item Env:GOOS -ErrorAction SilentlyContinue
Remove-Item Env:GOARCH -ErrorAction SilentlyContinue
Remove-Item Env:CGO_ENABLED -ErrorAction SilentlyContinue

# Install the Windows build into a user bin directory and add that directory
# to the user's Windows PATH so `log-work` can be called from anywhere.
$windowsInstallDir = Join-Path $HOME "bin"
New-Item -ItemType Directory -Force -Path $windowsInstallDir | Out-Null
Copy-Item "dist/log-work.exe" (Join-Path $windowsInstallDir "log-work.exe") -Force

$userPath = [Environment]::GetEnvironmentVariable("Path", "User")
$pathEntries = @()
if ($userPath) {
    $pathEntries = $userPath -split ";" | Where-Object { $_ -ne "" }
}

if ($pathEntries -notcontains $windowsInstallDir) {
    $newUserPath = if ($userPath) { "$userPath;$windowsInstallDir" } else { $windowsInstallDir }
    [Environment]::SetEnvironmentVariable("Path", $newUserPath, "User")
    $env:Path = "$env:Path;$windowsInstallDir"
}

# If WSL is available, install the Linux build into ~/.local/bin/log-work there.
# This lets WSL shells call `log-work` from anywhere when ~/.local/bin is on PATH.
$wslInstalled = $false
$wslCommand = Get-Command "wsl.exe" -ErrorAction SilentlyContinue
if ($wslCommand) {
    $linuxBytes = [System.IO.File]::ReadAllBytes((Resolve-Path "dist/log-work-linux-amd64"))
    $linuxBase64 = [Convert]::ToBase64String($linuxBytes)
    $linuxBase64 | wsl.exe sh -lc "mkdir -p ~/.local/bin && base64 -d > ~/.local/bin/log-work && chmod +x ~/.local/bin/log-work"
    wsl.exe sh -lc "cp /mnt/d/log-work/dist/log-work-linux-amd64 ~/.local/bin/log-work && chmod +x ~/.local/bin/log-work"
    $wslInstalled = $true
}

Write-Host "Built:"
Write-Host "  dist/log-work.exe"
Write-Host "  dist/log-work-linux-amd64"
Write-Host "Installed Windows executable:"
Write-Host "  $windowsInstallDir\log-work.exe"
if ($wslInstalled) {
    Write-Host "Installed WSL executable:"
    Write-Host "  ~/.local/bin/log-work"
    Write-Host "If WSL cannot find log-work, add this in WSL:"
    Write-Host '  export PATH="$HOME/.local/bin:$PATH"'
} else {
    Write-Host "WSL executable not installed because wsl.exe was not found."
}

Write-Host "Open a new PowerShell window if Windows PATH changes are not visible."
