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

### Materiale di licenza

Le release native devono essere ridistribuibili. Ogni archivio deve contenere:
1. `LICENSE` con la licenza del progetto e il copyright
2. `THIRD_PARTY_NOTICES.txt` con le licenze dei moduli Go linkati

`scripts/release/build_release.sh` e la workflow CI rifiutano un archivio che non li contiene; il check è `scripts/release/check_archive_licenses.sh`. Dopo una modifica a `go.mod` o ai moduli linkati rigenerare i notice con `python3 scripts/release/generate_third_party_notices.py`.

## Checklist Pre-Release

Prima di taggare:

1. eseguire il preflight di spazio (vedi sotto): su questa workstation il disco è il vincolo, non il tempo
2. eseguire il tool automatico di test funzionali
3. eseguire la suite automatica completa
4. allineare `skills/needlex-web-retrieval/SKILL.md` alla versione che si sta taggando (`version: vX.Y.Z`) e aggiornarne le regole se il rilascio cambia il contratto pubblico
5. aggiornare `LICENSE`/`THIRD_PARTY_NOTICES.txt` se cambiano licenza o dipendenze e verificare che ogni archivio li contenga
6. costruire gli artifact di rilascio
7. installare localmente l’applicazione
8. testare manualmente CLI
9. testare manualmente MCP stdio
10. verificare che gli installer puntino agli asset corretti
11. eseguire lo smoke di cattura render su una pagina dinamica reale e controllare che il packet riporti `signals.content_source`
12. verificare con `needlex doctor` che la skill installata sull’host sia allineata (`Agent Skill: ... stale=false`)
13. a rilascio chiuso, rimuovere `dist/`, i residui temporanei e reclamare le cache se il gate di spazio lo segnala

## Preflight E Pulizia Di Spazio

Questa workstation è governata come device con carenza di spazio (`governance/workstation.env`). Prima del rilascio:

```bash
bash scripts/check_workstation_space.sh .
df -h /
```

Regole:
1. se il gate di spazio fallisce, si reclama spazio prima di costruire: `go clean -cache`, rimozione di `dist/`, toolchain Go non più usati, `needlex prune`
2. non si disattivano i controlli per far passare il gate
3. `dist/` esiste per la durata del rilascio: dopo la pubblicazione va rimosso
4. le cache di altri progetti non si toccano mai

## Versione Della Skill

La skill dichiara la versione del contratto che documenta:

```text
skills/needlex-web-retrieval/SKILL.md -> version: vX.Y.Z
```

Regole:
1. la versione della skill deve coincidere con `NEEDLEX_VERSION` del rilascio; `scripts/release/build_release.sh` blocca la build se non coincidono
2. se il rilascio cambia il contratto pubblico (tool, mode, campi del packet, regole di escalation), aggiornare anche il testo della skill, non solo il numero
3. la copia installata su un host agent è uno snapshot: l’installer la riaggiorna con backup, e `needlex doctor` segnala lo scostamento
4. `scripts/check_skills.sh` richiede il frontmatter `version:`; `scripts/check_skill_refresh.sh` verifica backup e rollback del refresh

## Smoke Di Cattura Render

Se il rilascio tocca resa, cattura di rete, budget o packet, lo smoke minimo è:

1. `needlex read <pagina dinamica reale> --objective "<obiettivo concreto>" --json --json-mode full`
2. controllare nello stage `render` del trace: `render_path=cdp`, `network_observed > 0`, `network_body_missing` e `network_streams_open` coerenti con quanto ci si aspetta
3. controllare nel packet compatto che `signals.content_source` sia presente e che rifletta l'esito (`dom` o `dom+network`)
4. se compare `render_degraded` o `network_truncated`, dichiararlo nel closure come rischio residuo invece di trattarlo come successo

Un packet con `content_source` assente su una pagina notoriamente dinamica è un esito da spiegare, non da ignorare.

## Gate Automatici Minimi

Eseguire sempre:

```bash
go test ./... -count=1
```

Eseguire sempre anche:

```bash
bash scripts/check_governance.sh .
```

Se esiste un tool automatico dedicato di test funzionali per il rilascio, eseguirlo prima del tag.

Regola:
- non sostituire i test funzionali con sola compilazione

## Build Release

Costruire gli asset con:

```bash
NEEDLEX_VERSION=vX.Y.Z ./scripts/release/build_release.sh dist
```

`NEEDLEX_VERSION` is mandatory for a release build: without it the script
falls back to `dev` and skips the skill-version guard. Keep the skill marker
equal to the tag you are about to create.

Gli artifact attesi sono 6:
1. `needlex_linux_amd64.tar.gz`
2. `needlex_linux_arm64.tar.gz`
3. `needlex_darwin_amd64.tar.gz`
4. `needlex_darwin_arm64.tar.gz`
5. `needlex_windows_amd64.zip`
6. `needlex_windows_arm64.zip`

Ogni archivio deve contenere il binario, `LICENSE` e `THIRD_PARTY_NOTICES.txt`. Controllo:

```bash
bash scripts/release/check_archive_licenses.sh dist/needlex_linux_amd64.tar.gz
```

## Install Test Locale

Prima del rilascio usare il workflow:
- [local-install-workflow.md](local-install-workflow.md)

L’install test locale non è opzionale.

Bisogna verificare:
1. wrapper `needlex`
2. `NEEDLEX_HOME`
3. cartelle state create
4. esecuzione CLI
5. esecuzione MCP stdio

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
3. build e release devono rifiutare archivi privi di `LICENSE` e `THIRD_PARTY_NOTICES.txt`

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
- render_capture_smoked:
- captured_content_source:
- degradation_observed:
- skill_version_aligned:
- budget_recalibrated:
- space_preflight_passed:
- local_residue_removed:
- assets_built:
- assets_published:
- .agent_excluded:
- credentials_checked:
- residual_risks:
```

Campi `space_preflight_passed` e `local_residue_removed` riflettono il preflight di spazio e la pulizia di fine rilascio richiesti dalla governance del device.

Campi `render_capture_smoked`, `captured_content_source` e `degradation_observed` riflettono lo smoke di cattura render.
Il campo `skill_version_aligned` riporta la versione dichiarata dalla skill e conferma che l'install locale non risulti `stale` in `needlex doctor`.
Il campo `budget_recalibrated` va compilato quando il rilascio ha toccato `governance/budget.env`, indicando quali soglie sono state alzate e quali compensate più severe sono state introdotte.
