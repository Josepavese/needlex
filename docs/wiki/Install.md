# Install

Use the public installer if you want a user-local setup.

## Quick Install

Linux and macOS:

```bash
curl -fsSL https://raw.githubusercontent.com/Josepavese/needlex/main/install/install.sh | bash
```

Windows:

```powershell
irm https://raw.githubusercontent.com/Josepavese/needlex/main/install/install.ps1 | iex
```

Installed command:
1. `needlex`

## What The Installer Does

1. downloads the release asset for your platform
2. installs the binary wrapper
3. creates the local state root
4. wires `NEEDLEX_HOME`
5. prepares the same runtime surface for CLI and MCP

## Fetch Defaults

Needle-X defaults to a browser-like fetch profile.

Current defaults:
1. `browser_like`
2. retry with `hardened`

Use `standard` only when you need benchmark/debug comparability.

## Next

1. [CLI](./CLI.md)
2. [MCP And Tool Calling](./MCP-And-Tool-Calling.md)

## Full Reference

1. [Install Guide](../install.md)
2. [Fetch Profiles](../fetch-profiles.md)
