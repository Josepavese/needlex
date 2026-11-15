# Budget Evolution Workflow

Usare questo workflow quando si vuole:
- migliorare il concetto di budget della codebase
- valutare nuovi linter o nuove metriche strutturali
- rendere l'environment più competitivo sulle risorse
- aumentare la qualità del codice senza degradare velocità o focus del prodotto

## Obiettivo

Mantenere il budget come leva evolutiva reale.

Il budget non deve misurare solo:
- quante righe di codice esistono

Deve misurare anche:
- concentrazione della complessità
- dimensione dei punti caldi
- salute statica del codice
- disciplina di formattazione
- deriva architetturale
- costo di manutenzione implicito

## Filosofia

Questo workflow non serve a "passare il gate".

Serve a proteggere tre cose:
1. il vantaggio architetturale del prodotto
2. la qualità del codice come moltiplicatore di velocità futura
3. il rispetto dell'obiettivo originario: runtime locale, lean, spiegabile, competitivo

Principio guida:

`più controllo non significa più burocrazia; significa più capacità di evolvere senza tradire il progetto`

Quindi:
- non usare il budget per inseguire numeri vuoti
- non usare i linter per imporre preferenze estetiche
- non fare refactor piccoli e infiniti che consumano tempo ma non spostano la qualità reale
- non accettare miglioramenti locali che peggiorano la direzione sistemica

## Regola Dei Macrostep

Quando si apre questo workflow, lavorare per macrostep, non per microstep.

Un macrostep valido deve:
1. colpire una famiglia coerente di problemi
2. produrre un cambiamento misurabile nei numeri del budget o nella salute strutturale
3. chiudere un fronte, non solo "preparare il terreno"
4. lasciare il repo validato alla fine del burst

Un microstep da evitare è invece:
1. un piccolo refactor locale senza effetto sui guardrail
2. una modifica al budget senza analisi delle cause
3. un cambio tool senza decisione sul suo ruolo
4. un miglioramento stilistico che non riduce rischio o complessità

## Macrostep Canonici

Quando si lavora su budget evolution, usare solo uno di questi macrostep:

### 1. Metrics Recalibration

Usare quando il modello del budget è sbagliato o incompleto.

Output atteso:
- nuove metriche
- soglie aggiornate
- rimozione di metriche inutili
- documentazione riallineata

### 2. Structural Hotspot Reduction

Usare quando pochi file o funzioni concentrano complessità eccessiva.

Output atteso:
- meno `STRUCTURE_ISSUES`
- meno file sopra soglia
- orchestratori più sottili

### 3. Concentration Control

Usare quando il problema è nei package o nei cluster di codice, non nei singoli file.

Output atteso:
- riduzione `MAX_PACKAGE_LOC`
- distribuzione più sana delle responsabilità

### 4. Tooling Upgrade

Usare quando l'environment non spinge abbastanza qualità o non intercetta più i problemi giusti.

Output atteso:
- nuovi tool locali installati
- config versionata
- ruolo chiaro di ogni tool: report, soft budget o hard gate

### 5. Competitive Tightening

Usare quando il repo passa stabilmente e si può alzare l'asticella.

Output atteso:
- soglie più severe
- nessun degrado nei test
- pressione utile sui punti caldi reali

### 6. Strategic Recalibration

Usare quando una soglia punisce un substrate reale del prodotto e rischia di forzare amputazioni architetturali.

Output atteso:
- spiegazione esplicita del perché la vecchia soglia era sbagliata
- nuove metriche compensative più intelligenti
- zero allentamenti gratuiti

## Invarianti

Non allentare mai il budget per nascondere problemi reali.

Ogni modifica al budget deve rispettare queste regole:
1. nessun degrado su `go test ./...`
2. nessun degrado su `go vet`
3. nessun degrado su `staticcheck`
4. nessuna crescita incontrollata dei file grossi
5. nessuna nuova metrica entra se produce solo rumore e nessuna azione concreta
6. nessuna soglia viene alzata senza una motivazione architetturale esplicita

## Baseline Operativa

Controllare sempre:

```bash
go test ./...
bash scripts/check_governance.sh .
```

Controllare sempre anche:

```bash
export PATH="$PATH:$(go env GOPATH)/bin"
gofumpt -l $(find . -type f -name '*.go' ! -path './vendor/*' ! -path './.git/*')
go vet ./...
staticcheck ./...
golangci-lint run ./internal/... --out-format line-number
```

## Metriche Correnti Da Preservare

Il budget attuale presidia:
- `PROD_LOC`
- `GO_FILE_COUNT`
- `BENCHMARK_LOC`
- `BENCHMARK_GO_FILE_COUNT`
- `AVG_FILE_LOC`
- `INTERNAL_PACKAGES`
- `RUNTIME_DEPS`
- `FILES_OVER_300`
- `FILES_OVER_350`
- `MAX_FILE_LOC_OBSERVED`
- `MAX_PACKAGE_LOC_OBSERVED`
- `GOFUMPT_PENDING`
- `GO_VET_STATUS`
- `STATICCHECK_STATUS`
- `STRUCTURE_ISSUES`

## Quando Aprire Questo Workflow

Aprire questo workflow quando accade almeno una di queste condizioni:
1. il budget passa ma la codebase sembra meno manutenibile
2. il budget fallisce per ragioni non allineate al valore reale del prodotto
3. una nuova area del codice cresce più in fretta del previsto
4. i linter esistenti non stanno più intercettando i problemi più costosi
5. si vuole aumentare la pressione competitiva senza introdurre teatro

## Regola Di Ingresso

Prima di iniziare, dichiarare sempre:

```text
Macrostep:
- name:
- family:
- success_signal:
- stop_condition:
```

Se non è possibile compilare questi 4 campi, il lavoro è troppo piccolo o troppo vago e non va iniziato in questa modalità.

## Step 1: Fotografare La Codebase

Raccogliere prima i numeri veri:

```bash
bash scripts/check_governance.sh .
find internal -name '*.go' -not -name '*_test.go' -exec wc -l {} + | sort -n | tail -20
python - <<'PY'
import os, collections
pkg=collections.Counter()
for root,_,names in os.walk('internal'):
    total=0
    for n in names:
        if n.endswith('.go') and not n.endswith('_test.go'):
            with open(os.path.join(root,n)) as f:
                total += sum(1 for _ in f)
    if total:
        pkg[root]=total
for k,v in sorted(pkg.items(), key=lambda kv: kv[1], reverse=True):
    print(f"{v:5d} {k}")
PY
```

Domande da chiudere:
1. il problema è volume totale o concentrazione locale?
2. i file grossi sono orchestratori o contengono logica duplicata?
3. il package più grande è giustificato dal dominio o è solo aggregazione accidentale?

## Step 2: Classificare I Problemi

Classificare ogni failure o sospetto in una sola famiglia:

1. `raw_size`
   Troppo codice totale.

2. `hot_file`
   Pochi file troppo grandi.

3. `hot_package`
   Un package concentra troppa superficie.

4. `structure`
   Funzioni troppo lunghe, troppo complesse, troppo orchestrative.

5. `static_health`
   Problemi veri rilevati da `staticcheck` o `go vet`.

6. `format_drift`
   Mancanza di disciplina automatica su formatting/imports/layout.

7. `noise_metric`
   Una metrica produce lavoro ma non migliora il design.

## Step 3: Valutare Nuovi Linter

Valutare un nuovo linter solo se può guidare una decisione strutturale reale.

### Linters già utili

Tenere sempre:
- `gofumpt`
- `go vet`
- `staticcheck`
- `funlen`
- `gocyclo`

### Linters candidati da valutare

Valutare in modo mirato:
- `dupl`
  Utile se la codebase torna a duplicare flow simili.
- `gocognit`
  Utile se la complessità cognitiva sfugge a `gocyclo`.
- `revive`
  Utile se si vuole una disciplina stile/maintainability più ampia.
- `gocritic`
  Utile per hot spot pragmatici e semplificazioni locali.
- `maintidx`
  Utile come segnale aggregato, non come gate primario.
- `nestif`
  Utile se iniziano a crescere blocchi condizionali profondi.
- `ireturn`
  Utile solo se si vuole disciplinare confini di API; spesso troppo rumoroso.
- `depguard`
  Utile se si vuole bloccare dipendenze o package non ammessi.
- `interfacebloat`
  Utile se iniziano a crescere interfacce troppo larghe.
- `wastedassign`
  Utile se compaiono refactor incompleti o assegnazioni inutili.
- `unparam`
  Utile per rimuovere funzione/parametri inutili dopo grandi refactor.

### Linters da trattare con cautela

Non mettere come gate duro senza prova:
- `lll`
- `nlreturn`
- `varnamelen`
- `godot`
- `wsl`
- `godox`

Questi spesso spingono stile o micro-preferenze, non qualità strutturale.

## Step 4: Criterio Di Adozione Di Un Nuovo Linter

Per adottare un nuovo linter:
1. eseguire il linter sul repo
2. classificare gli issue in:
   - veri problemi
   - rumore tollerabile
   - rumore inutile
3. stimare quanti refactor reali produce
4. decidere una delle tre modalità:
   - `report_only`
   - `soft_budget`
   - `hard_gate`

### Regola

Un linter entra nel budget solo se:
1. almeno il `60%` degli issue trovati porta a miglioramenti veri
2. il costo di manutenzione del gate è basso
3. la metrica è coerente con la filosofia runtime-lean

## Step 5: Parametri Su Cui Agire

I parametri da stringere o allentare sono questi:

### Size
- `MAX_PROD_LOC`
- `MAX_AVG_FILE_LOC`
- `MAX_FILE_LOC`
- `MAX_FILES_OVER_300`
- `MAX_FILES_OVER_350`
- `MAX_PACKAGE_LOC`

### Health
- `REQUIRE_GOFUMPT`
- `REQUIRE_GOVET`
- `REQUIRE_STATICCHECK`
- `REQUIRE_STRUCTURE_LINT`

### Structure
- `MAX_STRUCTURE_ISSUES`

## Step 6: Politica Di Tuning

### Quando stringere una soglia

Stringere una soglia solo se:
1. il repo passa comodamente
2. i residui sono hot spot reali e non accidenti temporanei
3. stringere il gate produce pressione utile

Esempi:
- `STRUCTURE_ISSUES 8 -> 6` quando il repo scende stabilmente sotto `6`
- `FILES_OVER_350 5 -> 4` quando il repo sta a `4` per più burst consecutivi
- `MAX_PACKAGE_LOC 4000 -> 3700` quando il package più grande è stabilmente sotto `3700`

### Quando alzare una soglia

Alzare una soglia solo se:
1. il problema è nel modello del budget, non nel codice
2. la metrica punisce un substrate reale del prodotto
3. esistono altre metriche compensative più severe

Esempio corretto:
- `MAX_PROD_LOC 8000 -> 10500`
  accompagnato da:
  - `AVG_FILE_LOC`
  - `MAX_PACKAGE_LOC`
  - `STRUCTURE_ISSUES`
  - `gofumpt/go vet/staticcheck`

Precedente reale, v0.1.34 (recalibrazione per la consegna dei dati applicativi renderizzati):
- `HARD_MAX_PROD_LOC 34500 -> 34900` con `TARGET_MAX_PROD_LOC 34000 -> 34500`
  accompagnato da un irrigidimento contestuale di:
  - `HARD_MAX_AVG_FILE_LOC 235 -> 210` e `TARGET 210 -> 200`
  - `HARD_MAX_FILES_OVER_300 30 -> 29`
  - `HARD_MAX_FILES_OVER_350 24 -> 23`
- motivazione architetturale dichiarata: il substrate di resa/cattura aveva 150 righe di margine, insufficienti a chiudere buchi di cattura misurati senza amputare diagnostica
- il rilascio è stato chiuso con il campo `budget_recalibrated` nel closure di release

Questo è il formato atteso: nessun allentamento gratuito, compensazione più severa nello stesso burst, documentazione aggiornata e numero nuovo giustificato da una misura.

## Step 7: Environment Competitivo

Per rendere l'environment competitivo:

1. tenere tool locali installati e pronti:
   - `gofumpt`
   - `staticcheck`
   - `golangci-lint`

2. evitare tool che richiedono cloud o setup fragile per il controllo base

3. preferire sempre tool:
   - veloci
   - stabili
   - eseguibili in locale
   - con output parsabile

4. tenere `golangci-lint` con config versionata nel repo

5. usare il budget per guidare refactor di macro-livello, non micro-style wars

## Step 8: Decisione Finale

Alla fine di ogni revisione del budget scegliere una sola uscita:

1. `tighten`
   Stringere una o più soglie.

2. `hold`
   Tenere le soglie e pianificare refactor.

3. `recalibrate`
   Cambiare il modello del budget perché punisce il valore reale.

### Template decisione

Compilare sempre:

```text
Decision:
- mode: tighten | hold | recalibrate
- reason:
- metrics_changed:
- linters_added:
- linters_rejected:
- structural_hotspots_remaining:
- next_target:
```

## Step 8.5: Macrostep Closure

Prima di considerare chiuso il lavoro, verificare sempre:
1. il macrostep ha ridotto un problema reale
2. il macrostep ha evitato dispersione in fix secondari
3. il macrostep ha lasciato il repo più aderente all'obiettivo, non solo più conforme al gate
4. il prossimo step è un nuovo fronte coerente, non un "continuo dello stesso rifinitore"

## Step 9: Azioni Concrete Dopo La Decisione

### Se `tighten`

1. modificare `scripts/check_budget.sh`
2. aggiornare `.golangci.yml` se serve
3. aggiornare:
   - `README.md`
   - `docs/vademecum.md`
   - `docs/development-plan.md`
4. eseguire:

```bash
go test ./...
bash scripts/check_governance.sh .
```

### Se `hold`

1. lasciare il gate invariato
2. aprire un macrostep di refactor sui veri hot spot
3. non toccare le soglie per inseguire pass artificiali

### Se `recalibrate`

1. spiegare perché la metrica vecchia era sbagliata
2. introdurre almeno una metrica nuova compensativa
3. aggiornare script e documentazione nello stesso burst
4. rieseguire tutti i gate

## Hot Spot Order

Quando bisogna ridurre `STRUCTURE_ISSUES`, attaccare in questo ordine:

1. orchestratori transport/MCP
2. flow `query/read/crawl`
3. pack engine
4. analyzer/runtime backend
5. config e store

Motivo:
- i primi sono più facili da spezzare senza toccare la semantica core

## Anti-Pattern

Non fare mai queste mosse:

1. aggiungere un linter solo perché “sembra professionale”
2. alzare il LOC limit senza introdurre metriche compensative
3. stringere una soglia quando il repo passa solo per caso
4. usare il budget per imporre preferenze di stile personali
5. far entrare tool non ripetibili o non installabili facilmente
6. spezzare un problema strutturale in micro-iterazioni che non cambiano i numeri
7. usare il refactor come surrogato di decisione strategica
8. dimenticare il prodotto e ottimizzare solo il framework di controllo

## Exit Criteria

Il workflow è chiuso bene quando:
1. il budget è verde
2. la nuova metrica o il nuovo linter ha una motivazione chiara
3. la documentazione è aggiornata
4. il repo è più competitivo, non solo più controllato
5. il macrostep ha prodotto un guadagno percepibile e non solo un avanzamento amministrativo
