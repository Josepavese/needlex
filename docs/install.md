# Install

> [!WARNING]
> Needle-X is alpha software. The install flow, local state layout, CLI details, and output shape may still change.

## Current install path

Needle-X is installable today, but the honest current story is:
1. alpha
2. binary-first from GitHub Releases
3. wrapper-based local install

Installed command:
1. `needlex`

## Quick install

### Linux and macOS

```bash
curl -fsSL https://raw.githubusercontent.com/Josepavese/needlex/main/install/install.sh | bash
```

What it does:
1. downloads the latest release binary for your OS and architecture
2. installs a user-local wrapper command `needlex`
3. creates the local state root and subdirectories
4. creates the PAL SSOT config at `<state-root>/configs/needlex.json`
5. sets `NEEDLEX_HOME` and `NEEDLEX_CONFIG` inside the wrapper
6. installs or verifies local Ollama semantic prerequisites
7. pulls the default embedding model `embeddinggemma:latest`
8. installs or verifies a PAL-local headless render browser
9. enables render in the PAL SSOT config
10. updates PATH persistence for future shells or terminals
11. reconciles a previous install without duplicating PATH hooks
12. leaves unrelated commands untouched

Default paths:
1. binary wrapper: `~/.local/bin/needlex`
2. real binary: `~/.local/lib/needlex/needlex-real`
3. state root:
   Linux: `~/.local/share/needlex`
   macOS: `~/Library/Application Support/NeedleX`
4. runtime log: `<state-root>/logs/needlex.jsonl`
5. config: `<state-root>/configs/needlex.json`
6. render browser: `<state-root>/browsers/`

Diagnostic export:

```bash
needlex support bundle --out /tmp/needlex-support
```

Linux note:
1. the installer prefers `~/.local/share/needlex`
2. if `XDG_DATA_HOME` comes from a Snap-scoped shell such as VS Code Snap, the installer ignores that Snap path and keeps using the user-local default

### Windows

```powershell
irm https://raw.githubusercontent.com/Josepavese/needlex/main/install/install.ps1 | iex
```

Default paths:
1. binary wrapper: `%LOCALAPPDATA%\NeedleX\bin\needlex.cmd`
2. real binary: `%LOCALAPPDATA%\NeedleX\bin\needlex-real.exe`
3. state root: `%LOCALAPPDATA%\NeedleX`
4. config: `%LOCALAPPDATA%\NeedleX\configs\needlex.json`

## Semantic Prerequisites

Needle-X is embeddings-first and has no production mode without dense embeddings.

Default installed semantic backend:
1. runtime: local Ollama
2. endpoint: `http://127.0.0.1:11434/api/embed`
3. embedding model: `embeddinggemma:latest`
4. vector space: `ollama-embeddinggemma-v1`

The installer attempts to prepare these prerequisites:
1. Linux: installs Ollama through the official install script when `ollama` is missing
2. macOS: uses Homebrew when available; otherwise it stops with the official Ollama download URL
3. Windows: uses `winget install Ollama.Ollama` when available; otherwise it falls back to the official Ollama PowerShell installer
4. all platforms: starts the local Ollama API when needed, pulls the embedding model, and probes `/api/embed`

Skip only for controlled CI or packaging tests:

```bash
NEEDLEX_INSTALL_SKIP_SEMANTIC_PREREQS=1 bash install/install.sh
```

Change semantic config through the PAL SSOT file, not per-command env:

```bash
needlex config path
needlex config show
needlex config set semantic.provider_model nomic-embed-text:latest
needlex doctor
```

## Render Prerequisites

Needle-X prepares JavaScript rendering during installation because render is a required escalation path for client-rendered sites and future discovery quality.

Default installed render backend:
1. provider: `exec-dump-dom`
2. browser payload: Chrome for Testing `chrome-headless-shell` on supported platforms
3. Linux ARM64 payload: Playwright Chromium headless shell under the PAL browser directory
4. config fields: `render.enabled=true` and `render.browser_path=<PAL browser executable>`
5. network-aware CDP capture defaults: `render.network_max_bytes=64000000`, `render.network_resource_max_bytes=64000000`, `render.network_max_resources=32`, `render.network_max_messages=4096`

The installer does not install a global Chrome package. It downloads the browser payload into the PAL state root, probes the renderer, and writes the PAL SSOT config. Existing installs are reconciled by updating the `render.*` config keys. Render is enabled by default; use `--render off` only when you intentionally need static-only acquisition.

Runtime note: `exec-dump-dom` is the installed provider name. The implementation first uses Chrome DevTools Protocol against the PAL browser so the rendered DOM can carry the final `location.href` and same-origin textual network payloads from fetch/XHR, SSE, and WebSocket messages; `--dump-dom` remains a fallback. Older experimental provider names such as `remote-cdp` and `playwright-worker` are not accepted config values unless a future implementation wires them explicitly.

Skip only for controlled CI or packaging tests:

```bash
NEEDLEX_INSTALL_SKIP_RENDER_PREREQS=1 bash install/install.sh
```

Linux ARM64 note: Chrome for Testing does not currently publish a `linux-arm64` package. On `linux/arm64`, the installer uses Playwright with `PLAYWRIGHT_BROWSERS_PATH=<state-root>/browsers/playwright`. If Node.js/npm is missing, a Node.js LTS binary is downloaded into `<state-root>/browsers/node` for installation-time use.

## Re-running the installer

The installer is designed to converge, not just append.

Unix:
1. reuses the same wrapper path and real binary path
2. rewrites the `needlex` wrapper deterministically
3. keeps a single `# needlex-path` block in shell startup files
4. reuses or updates the PAL-local render browser
5. leaves unrelated commands untouched
6. preserves old state roots on disk if you intentionally switch to a new one

Windows:
1. rewrites `needlex.cmd` deterministically
2. deduplicates the user PATH before appending the install directory
3. reuses or updates the PAL-local render browser
4. leaves unrelated commands untouched
5. preserves old state roots on disk if you intentionally switch to a new one

## Build from source

```bash
go build -o needlex ./cmd/needle
```

Repo-local runs keep using the current local default state root unless you override it:

```bash
export NEEDLEX_HOME=/path/to/needlex-state
./needlex read https://example.com --json
```

## State root

Needle-X local state is controlled by:

```text
NEEDLEX_HOME
```

If unset, Needle-X uses the platform application data directory:
1. Linux: `$XDG_DATA_HOME/needlex` or `~/.local/share/needlex`
2. macOS: `~/Library/Application Support/NeedleX`
3. Windows: `%LOCALAPPDATA%\NeedleX`

## Fetch profiles

Needle-X now defaults to a browser-like fetch profile for real-world targets.

Current defaults:
1. `fetch.profile = browser_like`
2. `fetch.retry_profile = hardened`

Why:
1. the product default should maximize successful acquisition on the noisy web
2. benchmark/debug mode can still force a stricter transport profile

Operator overrides:

```bash
export NEEDLEX_FETCH_PROFILE=standard
export NEEDLEX_FETCH_RETRY_PROFILE=browser_like
```

Accepted values:
1. `standard`
2. `browser_like`
3. `hardened`

## Release packaging

Release archives are built with:

```bash
./scripts/release/build_release.sh dist
```

Current target artifacts:
1. `needlex_linux_amd64.tar.gz`
2. `needlex_linux_arm64.tar.gz`
3. `needlex_darwin_amd64.tar.gz`
4. `needlex_darwin_arm64.tar.gz`
5. `needlex_windows_amd64.zip`
6. `needlex_windows_arm64.zip`

## Recommendation

For now:
1. use the installer scripts if you want a user-local install
2. use source builds if you are working inside the repo
3. GitHub Releases are the binary distribution channel
4. treat every install as alpha-grade, not production-stable
