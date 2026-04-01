# Release Workflow

Usare questo workflow quando si deve:
- preparare un rilascio GitHub di Needle-X
- decidere la prossima versione
- costruire e pubblicare gli asset binari
- validare che install, CLI e MCP funzionino davvero

## Obiettivo

Pubblicare un rilascio onesto, riproducibile e installabile.

Questo workflow esiste per impedire:
- release improvvisate
- salti di versione incoerenti
- pubblicazione di credenziali o file locali
- artifact non testati
- rilascio di materiale interno come `.agent`

## Regola Di Versione

Lo schema versione è sempre:

```text
x.y.z
```

Regola standard:
1. incrementare sempre e solo `z`
2. non toccare `x` o `y` senza richiesta esplicita dell’utente

Esempio:
- `0.1.3 -> 0.1.4`

Eccezioni:
1. l’utente chiede esplicitamente major/minor bump
2. esiste una decisione esplicita sul significato del salto di compatibilità

Se non c’è richiesta chiara:
- fare patch release

## Invarianti

Ogni rilascio deve rispettare:
1. `.agent` non entra mai nel rilascio
2. nessuna credenziale va su git
3. gli installer devono puntare ad asset reali del release
4. CLI e MCP devono essere testati manualmente su install locale
5. il warning alpha deve restare visibile finché il prodotto è alpha

## Regole Di Esclusione

### `.agent`

`.agent` è superficie interna di lavoro.

Regole:
1. non includerla mai negli asset release
2. non usarla come parte del prodotto distribuito
3. non descriverla come componente richiesta dall’installazione

### Credenziali

Non committare mai:
1. `.needlex/competitive-benchmark.env`
2. token API
3. chiavi SaaS
4. endpoint privati con segreti embedded
5. file locali di ambiente o cache

Prima del rilascio controllare sempre:

```bash
git status --short
git diff --cached
rg -n "API_KEY|TOKEN|SECRET|PASSWORD|tvly-|fc-" .
```

Se un match è reale e sensibile:
- fermarsi
- rimuoverlo dal tree e dalla history del commit locale prima di proseguire

## Checklist Pre-Release

Prima di taggare:

1. eseguire il tool automatico di test funzionali
2. eseguire la suite automatica completa
3. costruire gli artifact di rilascio
4. installare localmente l’applicazione
5. testare manualmente CLI
6. testare manualmente MCP stdio
7. verificare che gli installer puntino agli asset corretti

## Gate Automatici Minimi

Eseguire sempre:

```bash
go test ./... -count=1
```

Eseguire sempre anche:

```bash
bash scripts/check_budget.sh .
```

Se esiste un tool automatico dedicato di test funzionali per il rilascio, eseguirlo prima del tag.

Regola:
- non sostituire i test funzionali con sola compilazione

## Build Release

Costruire gli asset con:

```bash
./scripts/release/build_release.sh dist
```

Gli artifact attesi sono 6:
1. `needlex_linux_amd64.tar.gz`
2. `needlex_linux_arm64.tar.gz`
3. `needlex_darwin_amd64.tar.gz`
4. `needlex_darwin_arm64.tar.gz`
5. `needlex_windows_amd64.zip`
6. `needlex_windows_arm64.zip`

## Install Test Locale

Prima del rilascio usare il workflow:
- [local-install-workflow.md](/home/jose/hpdev/Libraries/needlex/.agent/workflows/local-install-workflow.md)

L’install test locale non è opzionale.

Bisogna verificare:
1. wrapper `needlex`
2. alias `needle`
3. `NEEDLEX_HOME`
4. cartelle state create
5. esecuzione CLI
6. esecuzione MCP stdio

## Test Manuale CLI

Dopo install locale, verificare almeno:

```bash
needlex read https://example.com --json
needlex query https://example.com --goal "pricing" --json
needlex memory stats --json
needlex proof proof_1 --json || true
```

Controllare:
1. binary trovato in PATH
2. state root coerente
3. output senza crash
4. directory locali effettivamente usate

## Test Manuale MCP

Dopo install locale, verificare anche MCP stdio:

```bash
needlex mcp
```

Verificare:
1. processo parte
2. stdio MCP non crasha subito
3. entrypoint installato è lo stesso wrapper del CLI

Se esiste un harness locale MCP, usarlo.
Se non esiste:
- almeno verificare start-up e handshake base via stdio

## Workflow CI Release

Il rilascio GitHub deve usare la workflow:
- `.github/workflows/dist.yml`

Regola:
1. su push e PR deve buildare e testare
2. su tag `v*` deve pubblicare gli asset

Non fare release manuali uploadando file costruiti a mano se la workflow non è verde.

## Sequenza Operativa

Usare sempre questa sequenza:

1. scegliere versione `x.y.z`
2. aggiornare eventuali riferimenti release
3. eseguire test automatici
4. buildare asset
5. installare localmente
6. testare manualmente CLI
7. testare manualmente MCP
8. verificare hygiene credenziali
9. taggare
10. pushare il tag
11. verificare la release GitHub e gli installer

## Verifica Post-Release

Dopo il rilascio controllare:
1. asset presenti nel release GitHub
2. URL installer funzionanti
3. install Linux/macOS da `install.sh`
4. install Windows da `install.ps1`
5. README coerente col release appena pubblicato

## Checklist Di Chiusura

Chiudere sempre con:

```text
Release Closure:
- version:
- automatic_tests_run:
- functional_tool_run:
- local_install_tested:
- cli_tested:
- mcp_tested:
- assets_built:
- assets_published:
- .agent_excluded:
- credentials_checked:
- residual_risks:
```
