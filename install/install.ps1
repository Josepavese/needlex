$ErrorActionPreference = "Stop"

$Repo = if ($env:NEEDLEX_REPO) { $env:NEEDLEX_REPO } else { "Josepavese/needlex" }
$Version = if ($env:NEEDLEX_VERSION) { $env:NEEDLEX_VERSION } else { "latest" }
$ReleaseBaseUrl = if ($env:NEEDLEX_RELEASE_BASE_URL) { $env:NEEDLEX_RELEASE_BASE_URL } else { "" }
$SkipPathUpdate = if ($env:NEEDLEX_INSTALL_SKIP_PATH_UPDATE) { $env:NEEDLEX_INSTALL_SKIP_PATH_UPDATE } else { "0" }

function Remove-DuplicatePathEntries {
  param(
    [string[]]$Entries,
    [string]$CurrentBinDir
  )

  $seen = [System.Collections.Generic.HashSet[string]]::new([System.StringComparer]::OrdinalIgnoreCase)
  $result = New-Object System.Collections.Generic.List[string]
  foreach ($entry in $Entries) {
    if ([string]::IsNullOrWhiteSpace($entry)) {
      continue
    }
    $trimmed = $entry.Trim()
    if ($trimmed -ieq $CurrentBinDir) {
      continue
    }
    if ($seen.Add($trimmed)) {
      $result.Add($trimmed)
    }
  }
  return $result
}

function Get-ExistingStateRoot {
  param([string]$CmdPath)
  if (-not (Test-Path $CmdPath)) {
    return $null
  }
  $match = Select-String -Path $CmdPath -Pattern '^set NEEDLEX_HOME=(.+)$' | Select-Object -First 1
  if ($null -eq $match) {
    return $null
  }
  return $match.Matches[0].Groups[1].Value
}

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
$PreviousStateRoot = Get-ExistingStateRoot -CmdPath $NeedlexCmd

New-Item -ItemType Directory -Force -Path $BinDir | Out-Null
New-Item -ItemType Directory -Force -Path (Join-Path $StateRoot "traces") | Out-Null
New-Item -ItemType Directory -Force -Path (Join-Path $StateRoot "proofs") | Out-Null
New-Item -ItemType Directory -Force -Path (Join-Path $StateRoot "fingerprints") | Out-Null
New-Item -ItemType Directory -Force -Path (Join-Path $StateRoot "genome") | Out-Null
New-Item -ItemType Directory -Force -Path (Join-Path $StateRoot "discovery") | Out-Null
New-Item -ItemType Directory -Force -Path (Join-Path $StateRoot "candidates") | Out-Null
New-Item -ItemType Directory -Force -Path (Join-Path $StateRoot "domain_graph") | Out-Null
New-Item -ItemType Directory -Force -Path (Join-Path $StateRoot "fingerprint_graph") | Out-Null
New-Item -ItemType File -Force -Path (Join-Path $StateRoot "discovery\discovery.db") | Out-Null
Remove-Item -Force -ErrorAction SilentlyContinue (Join-Path $BinDir "needle.cmd")
Remove-Item -Force -ErrorAction SilentlyContinue (Join-Path $BinDir "needle.bat")
Remove-Item -Force -ErrorAction SilentlyContinue (Join-Path $BinDir "needle.exe")
Remove-Item -Force -ErrorAction SilentlyContinue (Join-Path $BinDir "needle-real.exe")

$tempDir = Join-Path ([System.IO.Path]::GetTempPath()) ("needlex-" + [guid]::NewGuid().ToString("N"))
New-Item -ItemType Directory -Force -Path $tempDir | Out-Null
try {
  $zipPath = Join-Path $tempDir "needlex.zip"
  Invoke-WebRequest -Uri $assetUrl -OutFile $zipPath
  Expand-Archive -Path $zipPath -DestinationPath $tempDir -Force
  Copy-Item (Join-Path $tempDir "needlex.exe") $RealExe -Force

  $cmd = "@echo off`r`nset NEEDLEX_HOME=$StateRoot`r`n`"$RealExe`" %*`r`n"
  Set-Content -Path $NeedlexCmd -Value $cmd -Encoding ascii
}
finally {
  Remove-Item -Recurse -Force $tempDir
}

if ($SkipPathUpdate -ne "1") {
  $userPath = [Environment]::GetEnvironmentVariable("Path", "User")
  $parts = @()
  if (-not [string]::IsNullOrWhiteSpace($userPath)) {
    $parts = $userPath.Split(';') | Where-Object { -not [string]::IsNullOrWhiteSpace($_) }
  }
  $deduped = Remove-DuplicatePathEntries -Entries $parts -CurrentBinDir $BinDir
  $newPath = (($deduped + $BinDir) -join ';')
  [Environment]::SetEnvironmentVariable("Path", $newPath, "User")
}

Write-Host ""
Write-Host "Installed needlex to $NeedlexCmd"
Write-Host "State root: $StateRoot"
if ($PreviousStateRoot -and $PreviousStateRoot -ne $StateRoot) {
  Write-Host "Previous state root preserved: $PreviousStateRoot"
}
if ($SkipPathUpdate -eq "1") {
  Write-Host "User PATH update skipped."
} else {
  Write-Host "Restart your shell to pick up PATH changes."
}
