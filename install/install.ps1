$ErrorActionPreference = "Stop"

$Repo = if ($env:NEEDLEX_REPO) { $env:NEEDLEX_REPO } else { "Josepavese/needlex" }
$Version = if ($env:NEEDLEX_VERSION) { $env:NEEDLEX_VERSION } else { "latest" }
$ReleaseBaseUrl = if ($env:NEEDLEX_RELEASE_BASE_URL) { $env:NEEDLEX_RELEASE_BASE_URL } else { "" }
$SkipPathUpdate = if ($env:NEEDLEX_INSTALL_SKIP_PATH_UPDATE) { $env:NEEDLEX_INSTALL_SKIP_PATH_UPDATE } else { "0" }
$SkipSemanticPrereqs = if ($env:NEEDLEX_INSTALL_SKIP_SEMANTIC_PREREQS) { $env:NEEDLEX_INSTALL_SKIP_SEMANTIC_PREREQS } else { "0" }
$SkipRenderPrereqs = if ($env:NEEDLEX_INSTALL_SKIP_RENDER_PREREQS) { $env:NEEDLEX_INSTALL_SKIP_RENDER_PREREQS } else { "0" }
$OllamaHost = if ($env:NEEDLEX_OLLAMA_HOST) { $env:NEEDLEX_OLLAMA_HOST } else { "http://127.0.0.1:11434" }
$SemanticEmbeddingUrl = if ($env:NEEDLEX_SEMANTIC_EMBEDDING_URL) { $env:NEEDLEX_SEMANTIC_EMBEDDING_URL } else { "$OllamaHost/api/embed" }
$SemanticModel = if ($env:NEEDLEX_SEMANTIC_PROVIDER_MODEL) { $env:NEEDLEX_SEMANTIC_PROVIDER_MODEL } else { "embeddinggemma:latest" }
$SemanticVectorSpace = if ($env:NEEDLEX_SEMANTIC_VECTOR_SPACE) { $env:NEEDLEX_SEMANTIC_VECTOR_SPACE } else { "ollama-embeddinggemma-v1" }
$RenderProvider = if ($env:NEEDLEX_RENDER_PROVIDER) { $env:NEEDLEX_RENDER_PROVIDER } else { "exec-dump-dom" }
$RenderBrowserPath = if ($env:NEEDLEX_RENDER_BROWSER_PATH) { $env:NEEDLEX_RENDER_BROWSER_PATH } else { "" }
$RenderTimeoutMS = if ($env:NEEDLEX_RENDER_TIMEOUT_MS) { $env:NEEDLEX_RENDER_TIMEOUT_MS } else { "5000" }
$RenderMaxConcurrency = if ($env:NEEDLEX_RENDER_MAX_CONCURRENCY) { $env:NEEDLEX_RENDER_MAX_CONCURRENCY } else { "1" }
$RenderChromeVersion = if ($env:NEEDLEX_RENDER_CHROME_VERSION) { $env:NEEDLEX_RENDER_CHROME_VERSION } else { "" }

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

function Get-OllamaCommand {
  $cmd = Get-Command ollama -ErrorAction SilentlyContinue
  if ($cmd) {
    return $cmd.Source
  }
  $candidates = @(
    (Join-Path $env:LOCALAPPDATA "Programs\Ollama\ollama.exe"),
    (Join-Path $env:ProgramFiles "Ollama\ollama.exe")
  )
  foreach ($candidate in $candidates) {
    if (Test-Path $candidate) {
      return $candidate
    }
  }
  return $null
}

function Install-OllamaIfMissing {
  if (Get-OllamaCommand) {
    return
  }
  $winget = Get-Command winget -ErrorAction SilentlyContinue
  if (-not $winget) {
    throw "Ollama is required but missing. Install winget or download Ollama from https://ollama.com/download, then rerun this installer."
  }
  & $winget.Source install --id Ollama.Ollama -e --accept-package-agreements --accept-source-agreements
  if (-not (Get-OllamaCommand)) {
    throw "Ollama install completed but ollama.exe was not found in PATH or standard install paths."
  }
}

function Test-OllamaApi {
  try {
    Invoke-RestMethod -Uri "$OllamaHost/api/tags" -Method Get -TimeoutSec 3 | Out-Null
    return $true
  } catch {
    return $false
  }
}

function Start-OllamaIfNeeded {
  if (Test-OllamaApi) {
    return
  }
  $ollama = Get-OllamaCommand
  if (-not $ollama) {
    throw "ollama command not found"
  }
  Start-Process -FilePath $ollama -ArgumentList "serve" -WindowStyle Hidden | Out-Null
  for ($i = 0; $i -lt 20; $i++) {
    Start-Sleep -Seconds 1
    if (Test-OllamaApi) {
      return
    }
  }
  throw "Ollama API did not become ready at $OllamaHost"
}

function Pull-EmbeddingModelIfNeeded {
  $ollama = Get-OllamaCommand
  $list = & $ollama list 2>$null
  if ($list -match [regex]::Escape($SemanticModel)) {
    return
  }
  & $ollama pull $SemanticModel
}

function Test-EmbeddingEndpoint {
  $body = @{ model = $SemanticModel; input = @("Needle-X semantic install probe") } | ConvertTo-Json -Compress
  try {
    Invoke-RestMethod -Uri $SemanticEmbeddingUrl -Method Post -ContentType "application/json" -Body $body -TimeoutSec 20 | Out-Null
  } catch {
    throw "Embedding endpoint probe failed: $SemanticEmbeddingUrl model=$SemanticModel error=$($_.Exception.Message)"
  }
}

function Ensure-SemanticPrereqs {
  if ($SkipSemanticPrereqs -eq "1") {
    Write-Host "Semantic prerequisite install skipped by NEEDLEX_INSTALL_SKIP_SEMANTIC_PREREQS=1"
    return
  }
  Install-OllamaIfMissing
  Start-OllamaIfNeeded
  Pull-EmbeddingModelIfNeeded
  Test-EmbeddingEndpoint
}

function Get-ChromeForTestingPlatform {
  switch ($goarch) {
    "amd64" { return "win64" }
    "arm64" { return "win64" }
    default { throw "Chrome for Testing headless shell is not available as a PAL-local package for windows/$goarch." }
  }
}

function Get-ChromeForTestingVersion {
  if (-not [string]::IsNullOrWhiteSpace($RenderChromeVersion)) {
    return $RenderChromeVersion.Trim()
  }
  return ((Invoke-WebRequest -Uri "https://googlechromelabs.github.io/chrome-for-testing/LATEST_RELEASE_STABLE").Content).Trim()
}

function Install-ChromeHeadlessShell {
  $version = Get-ChromeForTestingVersion
  $platform = Get-ChromeForTestingPlatform
  $targetDir = Join-Path $StateRoot "browsers\chrome-headless-shell\$version\$platform"
  $targetExe = Join-Path $targetDir "chrome-headless-shell.exe"
  if (Test-Path $targetExe) {
    $script:RenderBrowserPath = $targetExe
    return
  }
  $archiveUrl = "https://storage.googleapis.com/chrome-for-testing-public/$version/$platform/chrome-headless-shell-$platform.zip"
  $renderTemp = Join-Path ([System.IO.Path]::GetTempPath()) ("needlex-render-" + [guid]::NewGuid().ToString("N"))
  New-Item -ItemType Directory -Force -Path $renderTemp | Out-Null
  try {
    $zipPath = Join-Path $renderTemp "chrome-headless-shell.zip"
    Invoke-WebRequest -Uri $archiveUrl -OutFile $zipPath
    Expand-Archive -Path $zipPath -DestinationPath $renderTemp -Force
    $sourceDir = Join-Path $renderTemp "chrome-headless-shell-$platform"
    $sourceExe = Join-Path $sourceDir "chrome-headless-shell.exe"
    if (-not (Test-Path $sourceExe)) {
      throw "Chrome for Testing archive did not contain chrome-headless-shell.exe for $platform"
    }
    $parent = Split-Path -Parent $targetDir
    New-Item -ItemType Directory -Force -Path $parent | Out-Null
    $stagedDir = "$targetDir.tmp.$PID"
    if (Test-Path $stagedDir) {
      Remove-Item -Recurse -Force $stagedDir
    }
    New-Item -ItemType Directory -Force -Path $stagedDir | Out-Null
    Copy-Item -Path (Join-Path $sourceDir "*") -Destination $stagedDir -Recurse -Force
    if (Test-Path $targetDir) {
      Remove-Item -Recurse -Force $targetDir
    }
    Move-Item -Path $stagedDir -Destination $targetDir
    $script:RenderBrowserPath = $targetExe
  }
  finally {
    Remove-Item -Recurse -Force $renderTemp
  }
}

function Test-RenderBrowser {
  if ([string]::IsNullOrWhiteSpace($RenderBrowserPath) -or -not (Test-Path $RenderBrowserPath)) {
    throw "render browser is not executable: $RenderBrowserPath"
  }
  $profileDir = Join-Path ([System.IO.Path]::GetTempPath()) ("needlex-render-profile-" + [guid]::NewGuid().ToString("N"))
  New-Item -ItemType Directory -Force -Path $profileDir | Out-Null
  try {
    $args = @(
      "--disable-gpu",
      "--no-first-run",
      "--no-default-browser-check",
      "--disable-dev-shm-usage",
      "--disable-background-networking",
      "--disable-sync",
      "--user-data-dir=$profileDir",
      "--virtual-time-budget=1000",
      "--dump-dom",
      "data:text/html,<main>Needle-X%20render%20readiness%20probe</main>"
    )
    if ((Split-Path -Leaf $RenderBrowserPath) -notlike "*chrome-headless-shell*") {
      $args = @("--headless=new") + $args
    }
    $output = (& $RenderBrowserPath @args 2>$null | Out-String)
    if ($output -notlike "*Needle-X render readiness probe*") {
      throw "render browser probe failed: $RenderBrowserPath"
    }
  }
  finally {
    Remove-Item -Recurse -Force $profileDir
  }
}

function Ensure-RenderPrereqs {
  if ($SkipRenderPrereqs -eq "1") {
    Write-Host "Render prerequisite install skipped by NEEDLEX_INSTALL_SKIP_RENDER_PREREQS=1"
    return
  }
  if ($RenderProvider -ne "exec-dump-dom") {
    throw "unsupported render provider for installer-managed browser: $RenderProvider"
  }
  if (-not [string]::IsNullOrWhiteSpace($RenderBrowserPath)) {
    if (-not (Test-Path $RenderBrowserPath)) {
      throw "NEEDLEX_RENDER_BROWSER_PATH is not executable: $RenderBrowserPath"
    }
  } else {
    Install-ChromeHeadlessShell
  }
  Test-RenderBrowser
}

function Invoke-NeedlexConfigSet {
  param(
    [string]$Key,
    [string]$Value
  )
  & $RealExe config set $Key $Value | Out-Null
  if ($LASTEXITCODE -ne 0) {
    throw "needlex config set $Key failed"
  }
}

function Configure-RenderConfig {
  if ($SkipRenderPrereqs -eq "1") {
    return
  }
  if ([string]::IsNullOrWhiteSpace($RenderBrowserPath)) {
    throw "render browser path is empty after prerequisite install"
  }
  $oldNeedlexHome = $env:NEEDLEX_HOME
  $oldNeedlexConfig = $env:NEEDLEX_CONFIG
  try {
    $env:NEEDLEX_HOME = $StateRoot
    $env:NEEDLEX_CONFIG = $ConfigPath
    Invoke-NeedlexConfigSet "render.enabled" "true"
    Invoke-NeedlexConfigSet "render.provider" $RenderProvider
    Invoke-NeedlexConfigSet "render.browser_path" $RenderBrowserPath
    Invoke-NeedlexConfigSet "render.timeout_ms" $RenderTimeoutMS
    Invoke-NeedlexConfigSet "render.max_concurrency" $RenderMaxConcurrency
  }
  finally {
    $env:NEEDLEX_HOME = $oldNeedlexHome
    $env:NEEDLEX_CONFIG = $oldNeedlexConfig
  }
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
$ConfigPath = if ($env:NEEDLEX_CONFIG) { $env:NEEDLEX_CONFIG } else { Join-Path $StateRoot "configs\needlex.json" }
$RealExe = Join-Path $BinDir "needlex-real.exe"
$NeedlexCmd = Join-Path $BinDir "needlex.cmd"
$PreviousStateRoot = Get-ExistingStateRoot -CmdPath $NeedlexCmd

New-Item -ItemType Directory -Force -Path $BinDir | Out-Null
New-Item -ItemType Directory -Force -Path (Join-Path $StateRoot "analytics") | Out-Null
New-Item -ItemType Directory -Force -Path (Join-Path $StateRoot "configs") | Out-Null
New-Item -ItemType Directory -Force -Path (Join-Path $StateRoot "traces") | Out-Null
New-Item -ItemType Directory -Force -Path (Join-Path $StateRoot "proofs") | Out-Null
New-Item -ItemType Directory -Force -Path (Join-Path $StateRoot "fingerprints") | Out-Null
New-Item -ItemType Directory -Force -Path (Join-Path $StateRoot "genome") | Out-Null
New-Item -ItemType Directory -Force -Path (Join-Path $StateRoot "logs") | Out-Null
New-Item -ItemType Directory -Force -Path (Join-Path $StateRoot "discovery") | Out-Null
New-Item -ItemType Directory -Force -Path (Join-Path $StateRoot "candidates") | Out-Null
New-Item -ItemType Directory -Force -Path (Join-Path $StateRoot "domain_graph") | Out-Null
New-Item -ItemType Directory -Force -Path (Join-Path $StateRoot "fingerprint_graph") | Out-Null
New-Item -ItemType Directory -Force -Path (Join-Path $StateRoot "browsers") | Out-Null
New-Item -ItemType File -Force -Path (Join-Path $StateRoot "discovery\discovery.db") | Out-Null

$tempDir = Join-Path ([System.IO.Path]::GetTempPath()) ("needlex-" + [guid]::NewGuid().ToString("N"))
New-Item -ItemType Directory -Force -Path $tempDir | Out-Null
try {
  $zipPath = Join-Path $tempDir "needlex.zip"
  Invoke-WebRequest -Uri $assetUrl -OutFile $zipPath
  Expand-Archive -Path $zipPath -DestinationPath $tempDir -Force
  Copy-Item (Join-Path $tempDir "needlex.exe") $RealExe -Force
  Ensure-RenderPrereqs

  $oldNeedlexHome = $env:NEEDLEX_HOME
  $oldNeedlexConfig = $env:NEEDLEX_CONFIG
  $oldSemanticURL = $env:NEEDLEX_SEMANTIC_EMBEDDING_URL
  $oldSemanticModel = $env:NEEDLEX_SEMANTIC_PROVIDER_MODEL
  $oldSemanticVectorSpace = $env:NEEDLEX_SEMANTIC_VECTOR_SPACE
  $oldModelsBaseURL = $env:NEEDLEX_MODELS_BASE_URL
  $oldRenderEnabled = $env:NEEDLEX_RENDER_ENABLED
  $oldRenderProvider = $env:NEEDLEX_RENDER_PROVIDER
  $oldRenderBrowserPath = $env:NEEDLEX_RENDER_BROWSER_PATH
  $oldRenderTimeoutMS = $env:NEEDLEX_RENDER_TIMEOUT_MS
  $oldRenderMaxConcurrency = $env:NEEDLEX_RENDER_MAX_CONCURRENCY
  try {
    $env:NEEDLEX_HOME = $StateRoot
    $env:NEEDLEX_CONFIG = $ConfigPath
    $env:NEEDLEX_SEMANTIC_EMBEDDING_URL = $SemanticEmbeddingUrl
    $env:NEEDLEX_SEMANTIC_PROVIDER_MODEL = $SemanticModel
    $env:NEEDLEX_SEMANTIC_VECTOR_SPACE = $SemanticVectorSpace
    $env:NEEDLEX_MODELS_BASE_URL = "$OllamaHost/v1"
    $env:NEEDLEX_RENDER_ENABLED = if ($SkipRenderPrereqs -eq "1") { "false" } else { "true" }
    $env:NEEDLEX_RENDER_PROVIDER = $RenderProvider
    $env:NEEDLEX_RENDER_BROWSER_PATH = $RenderBrowserPath
    $env:NEEDLEX_RENDER_TIMEOUT_MS = $RenderTimeoutMS
    $env:NEEDLEX_RENDER_MAX_CONCURRENCY = $RenderMaxConcurrency
    & $RealExe config init
  }
  finally {
    $env:NEEDLEX_HOME = $oldNeedlexHome
    $env:NEEDLEX_CONFIG = $oldNeedlexConfig
    $env:NEEDLEX_SEMANTIC_EMBEDDING_URL = $oldSemanticURL
    $env:NEEDLEX_SEMANTIC_PROVIDER_MODEL = $oldSemanticModel
    $env:NEEDLEX_SEMANTIC_VECTOR_SPACE = $oldSemanticVectorSpace
    $env:NEEDLEX_MODELS_BASE_URL = $oldModelsBaseURL
    $env:NEEDLEX_RENDER_ENABLED = $oldRenderEnabled
    $env:NEEDLEX_RENDER_PROVIDER = $oldRenderProvider
    $env:NEEDLEX_RENDER_BROWSER_PATH = $oldRenderBrowserPath
    $env:NEEDLEX_RENDER_TIMEOUT_MS = $oldRenderTimeoutMS
    $env:NEEDLEX_RENDER_MAX_CONCURRENCY = $oldRenderMaxConcurrency
  }
  Configure-RenderConfig
  Ensure-SemanticPrereqs

  $cmd = "@echo off`r`nset NEEDLEX_HOME=$StateRoot`r`nset NEEDLEX_CONFIG=$ConfigPath`r`n`"$RealExe`" %*`r`n"
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
Write-Host "Config: $ConfigPath"
Write-Host "Runtime log: $(Join-Path $StateRoot 'logs\\needlex.jsonl')"
Write-Host "Semantic endpoint: $SemanticEmbeddingUrl"
Write-Host "Semantic model: $SemanticModel"
if ([string]::IsNullOrWhiteSpace($RenderBrowserPath)) {
  Write-Host "Render browser: <skipped>"
} else {
  Write-Host "Render browser: $RenderBrowserPath"
}
Write-Host "Agent skill: https://github.com/$Repo/tree/main/skills/needlex-web-retrieval"
Write-Host "Codex skill install: python3 ~/.codex/skills/.system/skill-installer/scripts/install-skill-from-github.py --repo $Repo --path skills/needlex-web-retrieval"
if ($PreviousStateRoot -and $PreviousStateRoot -ne $StateRoot) {
  Write-Host "Previous state root preserved: $PreviousStateRoot"
}
if ($SkipPathUpdate -eq "1") {
  Write-Host "User PATH update skipped."
} else {
  Write-Host "Restart your shell to pick up PATH changes."
}
