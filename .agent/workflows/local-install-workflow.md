# Local Install Workflow

Usare questo workflow quando si deve:
- installare Needle-X localmente per test reali
- verificare installer Unix o Windows
- fare pulizia di installazioni precedenti
- controllare che CLI e MCP usino il path installato

## Obiettivo

Ottenere un’installazione locale pulita, verificabile e coerente con il rilascio.

Questo workflow esiste per evitare:
- test eseguiti sul binary del repo invece che su quello installato
- stato locale sporco che maschera problemi
- wrapper vecchi in PATH
- mismatch tra `needlex` e `NEEDLEX_HOME`

## Regola

Quando si testa l’installazione:
1. usare il binary installato
2. non usare `go run ./cmd/needle`
3. verificare esplicitamente quale binary viene eseguito

## Cartelle E Path

### Linux

Wrapper:
1. `~/.local/bin/needlex`

Binary reale:
1. `~/.local/lib/needlex/needlex-real`

State root:
1. `${XDG_DATA_HOME:-$HOME/.local/share}/needlex`

### macOS

Wrapper:
1. `~/.local/bin/needlex`

Binary reale:
1. `~/.local/lib/needlex/needlex-real`

State root:
1. `~/Library/Application Support/NeedleX`

### Windows

Wrapper:
1. `%LOCALAPPDATA%\NeedleX\bin\needlex.cmd`

Binary reale:
1. `%LOCALAPPDATA%\NeedleX\bin\needlex-real.exe`

State root:
1. `%LOCALAPPDATA%\NeedleX`

## Pulizia Pre-Install

Prima di reinstallare, pulire eventuali residui incoerenti.

### Linux e macOS

Controllare:

```bash
command -v needlex || true
ls -la ~/.local/bin/needlex 2>/dev/null || true
ls -la ~/.local/lib/needlex 2>/dev/null || true
```

Se serve install pulita:

```bash
rm -f ~/.local/bin/needlex
rm -rf ~/.local/lib/needlex
```

Pulire lo state solo se vuoi un test cold-state:

Linux:
```bash
rm -rf "${XDG_DATA_HOME:-$HOME/.local/share}/needlex"
```

macOS:
```bash
rm -rf "$HOME/Library/Application Support/NeedleX"
```

### Windows

Controllare:

```powershell
Get-Command needlex -ErrorAction SilentlyContinue
```

Se serve install pulita:

```powershell
Remove-Item "$env:LOCALAPPDATA\\NeedleX\\bin\\needlex.cmd" -Force -ErrorAction SilentlyContinue
Remove-Item "$env:LOCALAPPDATA\\NeedleX\\bin\\needlex-real.exe" -Force -ErrorAction SilentlyContinue
```

Pulire lo state solo se vuoi un test cold-state:

```powershell
Remove-Item "$env:LOCALAPPDATA\\NeedleX" -Recurse -Force -ErrorAction SilentlyContinue
```

## Install

### Linux e macOS

```bash
curl -fsSL https://raw.githubusercontent.com/Josepavese/needlex/main/install/install.sh | bash
```

### Windows

```powershell
irm https://raw.githubusercontent.com/Josepavese/needlex/main/install/install.ps1 | iex
```

## Test Locale Dell'Installer Prima Del Release

Se il release non esiste ancora, testare l’installer contro un asset locale.

Regola:
1. buildare l’asset della piattaforma corrente
2. servirlo via HTTP locale
3. usare `NEEDLEX_RELEASE_BASE_URL`
4. installare in directory temporanee dedicate

### Linux e macOS

Esempio:

```bash
./scripts/release/build_release.sh /tmp/needlex-dist
cd /tmp/needlex-dist && python3 -m http.server 8765
```

In un altro shell:

```bash
export NEEDLEX_RELEASE_BASE_URL=http://127.0.0.1:8765
export NEEDLEX_INSTALL_BIN_DIR=/tmp/needlex-install/bin
export NEEDLEX_INSTALL_LIB_DIR=/tmp/needlex-install/lib
export NEEDLEX_HOME=/tmp/needlex-install/state
export NEEDLEX_INSTALL_SKIP_SHELL_HOOKS=1
bash install/install.sh
```

Questo è il test corretto prima del primo tag release.

## Verifica Post-Install

### Linux e macOS

```bash
command -v needlex
echo "$NEEDLEX_HOME"
ls -la ~/.local/bin/needlex
```

### Windows

```powershell
Get-Command needlex
```

## Verifica State Root

Controllare che esistano:
1. `traces/`
2. `proofs/`
3. `fingerprints/`
4. `genome/`
5. `discovery/discovery.db`

## Test CLI Minimo

Eseguire:

```bash
needlex read https://example.com --json
needlex memory stats --json
```

Se il rilascio supporta il caso:

```bash
needlex query https://example.com --goal "pricing" --json
```

Controllare:
1. `read` ritorna un packet JSON valido
2. `memory stats` legge davvero il db sotto `NEEDLEX_HOME`
3. il comando usato è il wrapper installato

## Test MCP Minimo

Verificare che l’entrypoint installato lanci davvero MCP:

```bash
needlex mcp
```

Regola:
- il test MCP va fatto sul wrapper installato, non sul binary del repo
- lo smoke test minimo è accettato anche se il processo termina subito senza crash
- se esiste un harness MCP locale, usarlo al posto del solo smoke test

## Regola Di Verifica Binary

Confermare sempre quale executable stai usando.

Linux e macOS:

```bash
which needlex
```

Windows:

```powershell
Get-Command needlex | Format-List *
```

Se il comando punta al repo, il test install è invalido.

## Quando Fare Un Test Cold

Fare install cold quando vuoi verificare:
1. primo bootstrap
2. creazione directory
3. comportamento senza state precedente

Non fare sempre cold se stai testando solo:
1. upgrade wrapper
2. compatibilità PATH
3. start-up MCP

## Checklist Di Chiusura

Chiudere sempre con:

```text
Install Closure:
- platform:
- clean_install:
- binary_verified:
- wrapper_verified:
- state_root_verified:
- cli_tested:
- mcp_tested:
- residual_risks:
```
