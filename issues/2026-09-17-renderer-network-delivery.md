# Consegna affidabile dei dati applicativi renderizzati

## Contesto

Il 2026-06-04 il renderer CDP è stato reso network-aware: `Network.enable`, cattura di fetch/XHR, messaggi EventSource e frame WebSocket, budget `render.network_*`, evidenza testuale sintetizzata in nodi `/network/resource[N]`. Quel lavoro è entrato nella release v0.1.33 (commit `0e6f89d`).

La misura sul campo dice però che la cattura non arrivava all'agente. Sui 26 trace locali che registrano i contatori del render:

```text
event_source_messages: 0 in tutti i run
network_resources: 0 nella maggior parte dei run
network_idle_reason: network_idle x23, websocket_idle x2, settle_timeout x1
durate render: quasi tutte <= 8s (due sole eccezioni non-auto: Figma 17s/28 risorse/37MB)
browser: chrome-headless-shell ... cdp in tutti i run
```

## Problema

Quattro difetti distinti, tutti misurabili:

### 1. Budget strutturalmente incompatibile con gli stream

`AutoRenderDeadlineTimeout = 8s` in `internal/core/sourceresolution/resolver.go` cappava il render in modalità `auto`, mentre il settle di uno stream EventSource richiede almeno 12s di osservazione (`internal/rendering/network_state.go`). In `auto`, quindi, uno stream SSE non poteva mai essere catturato per intero: il caso che aveva motivato la feature (Carratelli, stream da 9.2s) era catturabile solo con `--render required`.

### 2. Fetch-stream mai catturati

`handleResponseReceived` marcava `Source="event_source"` anche per risorse `fetch`/`xhr` con content-type `event-stream`. Poiché `shouldFetchResponseBody` esclude la source `event_source`, il body di uno stream applicativo moderno (fetch + reader) non veniva mai recuperato, i messaggi non arrivano (Chrome emette `eventSourceMessageReceived` solo per l'API EventSource), e lo stream bloccava comunque il settle. Costo pagato, contenuto perso.

### 3. Degradazione silenziosa

Se il percorso CDP falliva, `Render` ricadeva su `--dump-dom` senza alcun segnale: il trace mostrava un render riuscito con zero evidenza di rete. Il contatore `bodyUnavailableCount` era codice morto, mai esposto. `ResourceCount` contava solo i body non vuoti, confondendo risorse osservate e risorse trattenute.

### 4. Nessuna visibilità per l'agente

Il packet compatto non diceva se il contenuto proveniva dal DOM statico, dal DOM renderizzato o anche da payload applicativi. L'agente non poteva distinguere una pagina letta male da una pagina letta bene.

## Strategia

Escalation semantica, non "render sempre": la pagina statica resta la prima lettura, e il browser entra quando la superficie è un cappotto **oppure** quando il contenuto statico non copre semanticamente l'obiettivo. Il budget di render diventa adattivo fino a `render.timeout_ms`, guidato dall'attività reale della rete.

## Implementazione

1. **Budget adattivo** (`robots.go`, `resolver.go`): rimosso il cap fisso a 8s. In `auto` il timeout è `min(render.timeout_ms, deadline residuo - 2s)`, con floor 1s e guardia `AutoRenderDeadlineMinRemaining` invariata.
2. **Settle guidato dall'attività** (`network_state.go`): finestra minima di osservazione 8s, quiete di stream `max(idle*4, 6s)`, attività tracciata anche via `Network.dataReceived` per gli stream che non emettono eventi applicativi.
3. **Cattura fetch-stream** (`network_event_source.go`, `cdp_client.go`): `event_source` solo per l'API EventSource; gli stream fetch/xhr restano `response`, quindi catturabili a chiusura tramite `Network.getResponseBody`, con set `activeStreams` dedicato al settle.
4. **Degradazione visibile** (`rendering.go`, `robots.go`): `Page.Degraded`/`DegradeReason`, contatori `ObservedResources`, `BodyUnavailable`, `StreamsOpen`; trace arricchito con `render_path`, `network_observed`, `network_body_missing`, `network_streams_open`, `render_degraded`. Uno stream tagliato mentre è aperto viene marcato come troncato.
5. **Escalation semantica** (`robots.go`): in `auto`, se le euristiche strutturali non scattano, la superficie ridotta viene confrontata con l'obiettivo tramite l'allineatore semantico già in uso. Soglia `0.5`, calibrata sull'embedder locale: shell di navigazione ~0.35, contenuti realmente pertinenti 0.59-0.72, anche cross-lingua. Objective troppo brevi o segnaposto (es. la lane `crawl`) non attivano l'escalation.
6. **Nessun render su contenuti non-HTML** in `auto` (`IsHTMLLikeRawPage`); `required` resta intenzione esplicita.
7. **Provenienza nel packet** (`packet.go`, `packet_helpers.go`): `signals.content_source` (`dom`, `dom+network`), `network_resources`, `network_bytes`, `network_truncated`, `render_degraded`. Le letture statiche restano senza campi aggiuntivi.
8. **Contratto agente** (`skills/needlex-web-retrieval/SKILL.md`): l'agente controlla `content_source` e, prima di uscire dallo strumento, ritenta una volta con `render: "required"`.

## Test

Unit e live, tutti verdi:

1. regressione del cap: uno stream SSE che emette per ~11.5s viene catturato oltre il vecchio limite (browser reale)
2. regressione fetch-stream: il body di un `fetch` su `text/event-stream` compare in `NetworkResources` (browser reale)
3. stream aperto al momento dello snapshot: marcato troncato e contato in `StreamsOpen`
4. fallback degradato: browser finto che non espone CDP produce `render_path=dump_dom` e `render_degraded=true` senza browser reale
5. escalation semantica: copertura bassa = un render esatto con `semantic_gap_similarity` nel trace, copertura alta = nessun render, objective segnaposto = nessun render
6. budget adattivo: il timeout passato al renderer supera il vecchio cap e resta entro il deadline residuo
7. contenuti non-HTML non renderizzati in `auto`
8. provenienza packet: `dom+network`, `dom`, vuota per letture statiche, degradata

## Non obiettivi

1. Nessuna intercettazione `Fetch.enable` o lettura progressiva di stream ancora aperti: gli stream che non chiudono entro il budget vengono segnalati, non inseguiti.
2. Nessuno stealth o anti-bot bypass.
3. Nessuna modifica alla superficie seedless sperimentale.

## Criterio di accettazione

1. `go test ./...` passa.
2. `network_truncated` e `content_source` rendono ispezionabile se i dati applicativi sono arrivati o sono stati tagliati.
3. Un render in `auto` non può superare il deadline dell'operazione.
4. Le letture statiche non pagano latenza aggiuntiva: l'escalation semantica scatta solo su copertura debole.
