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
5. wires `NEEDLEX_CONFIG` to the PAL SSOT config
6. installs or verifies local Ollama embeddings prerequisites
7. pulls the default embedding model `embeddinggemma:latest`
8. prepares the same runtime surface for CLI and MCP
9. reconciles reruns without duplicating PATH hooks
10. leaves unrelated commands untouched
11. prints the optional Codex skill path for agent-side usage guidance
12. creates the PAL runtime log directory used by `needlex logs`

## Semantic Config

Needle-X is embeddings-first. The installed command reads:

```text
<state-root>/configs/needlex.json
```

Default semantic backend:
1. Ollama local API
2. `http://127.0.0.1:11434/api/embed`
3. `embeddinggemma:latest`
4. `ollama-embeddinggemma-v1`

Operators change this with:

```bash
needlex config show
needlex config set semantic.provider_model nomic-embed-text:latest
needlex doctor
```

## Optional Agent Skill

Install the Needle-X web retrieval skill when Codex or another AI agent should know when to use Needle-X and when to escalate to browser/raw-fetch tools.

Skill path:
1. [skills/needlex-web-retrieval](../../skills/needlex-web-retrieval)

Codex install helper:

```bash
python3 ~/.codex/skills/.system/skill-installer/scripts/install-skill-from-github.py --repo Josepavese/needlex --path skills/needlex-web-retrieval
```

Restart Codex after installing the skill.

## Reinstall Behavior

The installer is meant to be re-runnable.

Unix:
1. keeps one `needlex` wrapper
2. keeps one PATH hook block
3. reuses the same install paths unless you override them

Windows:
1. rewrites `needlex.cmd`
2. deduplicates the user PATH
3. keeps the install convergent on rerun

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
