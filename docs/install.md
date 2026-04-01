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
4. sets `NEEDLEX_HOME` to an OS-appropriate state directory
5. updates PATH persistence for future shells or terminals

Default paths:
1. binary wrapper: `~/.local/bin/needlex`
2. real binary: `~/.local/lib/needlex/needlex-real`
3. state root:
   Linux: `~/.local/share/needlex`
   macOS: `~/Library/Application Support/NeedleX`

### Windows

```powershell
irm https://raw.githubusercontent.com/Josepavese/needlex/main/install/install.ps1 | iex
```

Default paths:
1. binary wrapper: `%LOCALAPPDATA%\NeedleX\bin\needlex.cmd`
2. real binary: `%LOCALAPPDATA%\NeedleX\bin\needlex-real.exe`
3. state root: `%LOCALAPPDATA%\NeedleX`

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

If unset, the current repo-local default remains:

```text
.needlex/
```

That preserves the current operator workflow while giving installed setups a clean platform-specific home.

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
