# Macrostep Delivery Workflow

Usare questo workflow quando si deve:
- scegliere il prossimo blocco di lavoro tecnico
- evitare iterazioni piccole, locali e poco significative
- chiudere burst completi di sviluppo
- mantenere il lavoro aderente alla filosofia del progetto

## Obiettivo

Fare avanzare Needle-X per fronti strategici completi, non per patch di movimento.

Questo workflow esiste per impedire che il progetto degeneri in:
- microstep continui
- refactor senza direzione
- checklist locali scollegate dal moat
- lavoro tecnicamente corretto ma strategicamente sterile

## Filosofia

Un macrostep valido deve aumentare almeno una di queste tre cose:
1. capacità reale del prodotto
2. qualità strutturale del runtime
3. vantaggio competitivo del sistema

Se un’attività non migliora in modo percepibile almeno uno di questi assi, non è un macrostep.

Principio guida:

`non ottimizzare il movimento; ottimizzare il cambiamento`

Corollari:
- non fare dieci fix minori quando serve una decisione architetturale
- non confondere manutenzione con avanzamento
- non introdurre nuove superfici se non spostano il vantaggio del prodotto
- non sacrificare il disegno per la gratificazione di “fare qualcosa”

## Invarianti

Ogni macrostep deve rispettare:
1. deterministic-first
2. local-first
3. proof/trace/replay come strumenti centrali, non accessori
4. substrate compatti e forti
5. nessuna crescita gratuita della superficie
6. niente stubs vuoti o scaffolding senza uso reale

## Quando Aprire Questo Workflow

Aprire questo workflow quando:
1. bisogna scegliere il prossimo fronte importante
2. il lavoro sta cadendo in loop di microstep
3. il repo è tecnicamente sano ma il progresso strategico è confuso
4. ci sono più opzioni plausibili e serve decidere per impatto
5. si vuole verificare se il lavoro in corso sta ancora rispettando l’obiettivo iniziale

## Regola Di Ingresso

Prima di iniziare un nuovo burst compilare sempre:

```text
Macrostep:
- name:
- product_axis:
- code_axis:
- competitive_axis:
- success_signal:
- stop_condition:
```

### Regola

Se non è possibile compilare tutti i campi in modo concreto:
- il macrostep è troppo vago
- oppure è troppo piccolo
- oppure è mascherato da “fare cose utili” ma non ha una tesi forte

In tutti e tre i casi, non iniziare.

## Assi Di Valutazione

Ogni macrostep va valutato su tre assi:

### 1. Product Axis

Quanto aumenta la capacità reale di Needle-X?

Esempi:
- migliore read/query/crawl
- migliore intelligence routing
- migliore substrate discovery
- output più utile per agenti

### 2. Code Axis

Quanto riduce complessità, attrito, concentrazione o deriva?

Esempi:
- meno hot spot
- meno orchestratori monolitici
- meno duplicazione di competenze
- budget più competitivo

### 3. Competitive Axis

Quanto rende il sistema più difficile da sostituire con stack commodity?

Esempi:
- WebIR più centrale
- proof-carrying context più forte
- typed patch intelligence più selettiva e misurabile
- delta-aware substrate più utile

## Macrostep Canonici

Usare solo macrostep appartenenti a una di queste famiglie:

### 1. Runtime Capability Burst

Usare quando si aggiunge una capacità reale del prodotto.

Output atteso:
- nuova capacità eseguibile
- test reali
- proof/trace coerenti
- documentazione aggiornata

### 2. Structural Consolidation Burst

Usare quando il problema è dispersione, frammentazione o eccesso di centri decisionali.

Output atteso:
- meno duplicazione concettuale
- substrate più compatti
- budget o lint migliori

### 3. Intelligence Integration Burst

Usare quando si integra o si espande il ruolo dell’intelligence in modo misurabile.

Output atteso:
- task tipizzati o backend reali
- acceptance gate coerente
- nessuna inferenza libera fuori dal substrate

### 4. Benchmark Closure Burst

Usare quando la tecnologia esiste ma non è ancora dimostrata.

Output atteso:
- confronto reale
- metriche esportate
- decisione netta su cosa tenere o scartare

### 5. Strategic Prune Burst

Usare quando il problema è superficie, zavorra o crescita non giustificata.

Output atteso:
- meno LOC utili ma non amputazione del moat
- meno task o metadata inutili
- budget e leggibilità migliori

### 6. Tooling And Guardrails Burst

Usare quando il repo non ha abbastanza pressione competitiva su qualità e struttura.

Output atteso:
- gate più intelligenti
- environment più forte
- feedback loop più rapido

## Sequenza Di Decisione

Prima di eseguire un macrostep, fare sempre questo ordine:

1. identificare il vincolo principale
2. capire se il vincolo è:
   - prodotto
   - struttura
   - benchmark
   - tooling
3. scegliere una sola famiglia di macrostep
4. definire un burst che chiuda un fronte, non che “inizi qualcosa”

## Regola Anti-Microstep

Un’attività è troppo piccola se:
1. tocca un solo helper senza muovere metriche o design
2. migliora il codice localmente ma non il substrate
3. richiede subito un “piccolo follow-up” per diventare utile
4. non cambia nessun numero, nessuna capacità o nessuna decisione

### Esempi da evitare

- “sistemo due helper e poi vediamo”
- “inizio a preparare la base per…”
- “aggiungo una metrica ma senza decidere la soglia”
- “creo un task ma resta spento e non benchmarkato”

### Regola

Se un passo richiede immediatamente altri tre mini-passi per avere senso, non era un macrostep.

## Regola Di Chiusura

Un macrostep è chiuso bene solo se:
1. il fronte scelto è stato davvero chiuso
2. il repo è validato
3. il risultato è misurabile
4. la documentazione riflette la nuova realtà
5. il prossimo step cambia fronte o approfondisce un fronte ancora aperto in modo giustificato

## Checklist Di Uscita

Chiudere sempre con questa checklist:

```text
Closure:
- capability_gained:
- structure_improved:
- competitive_gain:
- tests_run:
- gates_run:
- docs_updated:
- residual_risks:
- next_macrostep:
```

## Criterio Di Priorità

Quando esistono più macrostep possibili, scegliere con questo ordine:

1. quello che sblocca il prodotto
2. poi quello che riduce il rischio strutturale
3. poi quello che dimostra il vantaggio
4. poi quello che migliora il tooling

### Eccezione

Invertire la priorità se:
- il budget è vicino a fallire
- la complessità sta crescendo più della capacità prodotto
- i benchmark non permettono più decisioni difendibili

## Rapporti Con Il Budget

Il budget non decide da solo i macrostep.

Il budget serve a segnalare:
- dove la struttura sta degenerando
- dove la velocità futura si sta erodendo

Quindi:
- non fare macrostep di budget quando il problema è di prodotto
- non fare macrostep di prodotto quando la struttura sta già collassando

## Rapporti Con La Filosofia Del Progetto

Ogni macrostep deve aiutare Needle-X a restare:
- locale
- deterministico per default
- compatto
- verificabile
- competitivo contro stack più rumorosi e più pesanti

Se un macrostep introduce:
- più complessità di quanta ne rimuove
- più dipendenze di quanto giustifica
- più magia di quanto riesce a spiegare

allora sta tradendo il progetto, anche se funziona.

## Anti-Pattern

Non fare mai:
1. burst senza tesi
2. refactor senza criterio di uscita
3. delivery senza validazione finale
4. benchmarking senza decisione
5. tooling più complicato del prodotto
6. microstep ripetuti per evitare una scelta strategica
7. crescita del codice senza crescita del moat

## Exit Criteria

Questo workflow è usato bene quando:
1. il team sceglie macrostep chiari
2. i burst chiudono fronti veri
3. il repo non cade in iterazioni piccole e sterili
4. il prodotto resta aderente alla sua filosofia
5. ogni ciclo lascia un guadagno netto, non solo attività completata
