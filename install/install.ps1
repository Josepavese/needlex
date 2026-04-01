$ErrorActionPreference = "Stop"

$Repo = if ($env:NEEDLEX_REPO) { $env:NEEDLEX_REPO } else { "Josepavese/needlex" }
$Version = if ($env:NEEDLEX_VERSION) { $env:NEEDLEX_VERSION } else { "latest" }
$ReleaseBaseUrl = if ($env:NEEDLEX_RELEASE_BASE_URL) { $env:NEEDLEX_RELEASE_BASE_URL } else { "" }

$arch = $env:PROCESSOR_ARCHITECTURE
switch ($arch.ToUpperInvariant()) {
  "AMD64" { $goarch = "amd64" }
  "ARM64" { $goarch = "arm64" }
  default { throw "unsupported architecture: $arch" }
}

$base = "needlex_windows_$goarch.zip"
if (-not [string]::IsNullOrWhiteSpace($ReleaseBaseUrl)) {
  $assetUrl = "$ReleaseBaseUrl/$base"
} elseif ($Version -eq "latest") {
  $assetUrl = "https://github.com/$Repo/releases/latest/download/$base"
} else {
  $assetUrl = "https://github.com/$Repo/releases/download/$Version/$base"
}

$InstallRoot = if ($env:NEEDLEX_INSTALL_ROOT) { $env:NEEDLEX_INSTALL_ROOT } else { Join-Path $env:LOCALAPPDATA "NeedleX" }
$BinDir = Join-Path $InstallRoot "bin"
$StateRoot = if ($env:NEEDLEX_HOME) { $env:NEEDLEX_HOME } else { Join-Path $env:LOCALAPPDATA "NeedleX" }
$RealExe = Join-Path $BinDir "needlex-real.exe"
$NeedlexCmd = Join-Path $BinDir "needlex.cmd"
$NeedleCmd = Join-Path $BinDir "needle.cmd"

New-Item -ItemType Directory -Force -Path $BinDir | Out-Null
New-Item -ItemType Directory -Force -Path (Join-Path $StateRoot "traces") | Out-Null
New-Item -ItemType Directory -Force -Path (Join-Path $StateRoot "proofs") | Out-Null
New-Item -ItemType Directory -Force -Path (Join-Path $StateRoot "fingerprints") | Out-Null
New-Item -ItemType Directory -Force -Path (Join-Path $StateRoot "genome") | Out-Null
New-Item -ItemType Directory -Force -Path (Join-Path $StateRoot "discovery") | Out-Null
New-Item -ItemType File -Force -Path (Join-Path $StateRoot "discovery\discovery.db") | Out-Null

$tempDir = Join-Path ([System.IO.Path]::GetTempPath()) ("needlex-" + [guid]::NewGuid().ToString("N"))
New-Item -ItemType Directory -Force -Path $tempDir | Out-Null
try {
  $zipPath = Join-Path $tempDir "needlex.zip"
  Invoke-WebRequest -Uri $assetUrl -OutFile $zipPath
  Expand-Archive -Path $zipPath -DestinationPath $tempDir -Force
  Copy-Item (Join-Path $tempDir "needlex.exe") $RealExe -Force

  $cmd = "@echo off`r`nset NEEDLEX_HOME=$StateRoot`r`n`"$RealExe`" %*`r`n"
  Set-Content -Path $NeedlexCmd -Value $cmd -Encoding ascii
  Set-Content -Path $NeedleCmd -Value "@echo off`r`n`"$NeedlexCmd`" %*`r`n" -Encoding ascii
}
finally {
  Remove-Item -Recurse -Force $tempDir
}

Write-Host ""
Write-Host "Installed needlex to $NeedlexCmd"
Write-Host "Compatibility alias: $NeedleCmd"
Write-Host "State root: $StateRoot"
