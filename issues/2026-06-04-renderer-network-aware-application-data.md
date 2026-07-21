# Renderer network-aware per dati applicativi dinamici

## Contesto

Needle-X ora renderizza pagine JavaScript come ultima escalation dopo le sorgenti agent-readable standard. Questo recupera DOM client-side, ma non basta per siti che caricano il contenuto principale tramite canali applicativi asincroni.

Caso osservato: `https://carratellire.com/`.

La homepage statica contiene una shell Angular e inizializza un `EventSource` verso:

```text
php/stream/all.php?chunk=true&first_chunk=1
```

Misura del 2026-06-04:

```text
HTML statico: ~1.39 MB
SSE stream: ~37.7 MB
SSE durata: ~9.2s
SSE chunk JSON: 43
prodotti/immobili nello stream: 852
```

Il DOM renderizzato con `--dump-dom` mostra una proiezione parziale della pagina, ma non materializza l'intero dataset applicativo. Quindi aspettare `document.readyState=complete`, `network idle` debole, o un budget temporale fisso non garantisce copertura informativa.

## Problema

Il renderer attuale cattura principalmente:

1. `document.documentElement.outerHTML` via CDP
2. fallback Chromium `--dump-dom`

Questo è fragile per pagine in cui i dati reali viaggiano su:

1. Server-Sent Events (`text/event-stream`)
2. `fetch` / XHR JSON
3. WebSocket
4. endpoint API same-origin o same-family invocati dopo bootstrap
5. stream lunghi o chunked HTTP
6. trasporti HTTP/2 o HTTP/3 sotto il browser

Il livello HTTP/2/HTTP/3 non deve essere trattato con euristiche dedicate nel ranking. Il browser/CDP deve astrarre il trasporto e fornire eventi di rete. Needlex deve ragionare sui payload e sulla provenance, non sul protocollo di trasporto come segnale semantico.

## Obiettivo

Rendere il renderer network-aware e capace di restituire, oltre al DOM:

1. risorse testuali rilevanti osservate via CDP
2. payload SSE chiusi o quiescenti
3. messaggi WebSocket testuali rilevanti
4. payload JSON/testo da fetch/XHR/documenti same-origin
5. metriche di quiescenza e limiti applicati

Il comportamento deve rimanere generale:

1. nessun riferimento a `window._a2s`
2. nessun path applicativo hardcoded
3. nessun ranking su nomi custom
4. nessuna dipendenza da framework specifici

## Strategia

### 1. CDP Network

Abilitare `Network.enable` nel renderer CDP e raccogliere eventi:

1. `Network.responseReceived`
2. `Network.loadingFinished`
3. `Network.loadingFailed`
4. `Network.eventSourceMessageReceived`
5. `Network.webSocketFrameReceived`
6. `Network.webSocketFrameSent` solo come diagnostica, non come contenuto

Per ogni risorsa testuale acquisibile:

1. mantenere URL finale
2. content-type
3. resource type CDP
4. status
5. body parziale entro budget
6. byte originali osservati
7. flag `truncated`
8. provenance `render_network`

### 2. Attesa stabilità applicativa

Sostituire l'attesa `readyState + 500ms` con una attesa composta:

1. `document.readyState` almeno `interactive`
2. finestra minima post-ready
3. idle network per richieste testuali rilevanti
4. idle SSE/WebSocket per messaggi ricevuti
5. limite massimo render
6. chiusura stream quando disponibile

L'algoritmo non deve attendere WebSocket infiniti senza limite. Deve usare quiescenza temporale e budget.

### 3. Budgets

Separare budget DOM e network:

1. `render.timeout_ms`
2. `runtime.max_bytes` per documento/DOM
3. budget per singola risorsa network
4. budget totale network
5. massimo numero risorse testuali
6. massimo messaggi SSE/WebSocket conservati

Default implementati:

1. timeout render: 30000ms
2. network total: 64 MB
3. single resource: 64 MB
4. risorse testuali: 32
5. messaggi SSE/WebSocket osservati e conservabili per risorsa: 4096

Per siti come Carratelli il payload osservato e misurato rientra nel budget network da 64 MB. Se il payload supera i limiti, il sistema deve catturare quanto possibile e segnalare truncation.

### 4. Integrazione WebIR

Il core deve ridurre il DOM più un documento sintetico derivato da risorse network testuali rilevanti.

Principio:

```text
rendered DOM + render network text => RawPage HTML sintetico con provenance
```

Le risorse network non devono essere trattate come ranking provider custom. Sono evidence payload della pagina renderizzata.

### 5. Diagnostica

Il trace `render` deve includere:

1. durata
2. browser
3. risorse network catturate
4. SSE/WebSocket message count
5. bytes/truncation
6. idle reason
7. fallback CDP/dump-dom

### 6. Test

Test richiesti:

1. server locale con pagina che usa SSE e chiude con `end`
2. server locale con fetch JSON
3. server locale con WebSocket testuale
4. verifica che il WebIR includa contenuto arrivato via network anche se non presente nel DOM iniziale
5. verifica che un SSE infinito non blocchi oltre timeout/quiescenza

## Stato implementazione

Implementato in data 2026-06-04:

1. renderer CDP con `Network.enable`
2. cattura payload testuali `fetch`/XHR/document
3. cattura messaggi `EventSource`
4. cattura frame testuali WebSocket ricevuti
5. chiusura WebSocket osservata via CDP
6. budget configurabili `render.network_*`
7. testo network sintetizzato in nodi `/network/resource[...]`
8. trace render con risorse, byte, messaggi, truncation e idle reason
9. test locale fetch + SSE + WebSocket
10. test unitario per limite messaggi e SSE idle aperto

Smoke reale del 2026-06-04 su `https://carratellire.com/`:

```text
fetch_mode: render
browser: google-chrome cdp
duration_ms: 10156
event_source_messages: 44
network_resources: 4
network_bytes: 40051253
network_truncated: false
chunk_count: 1128
```

## Non obiettivi

1. Non implementare stealth o anti-bot bypass.
2. Non usare path applicativi noti.
3. Non interpretare HTTP/2 o HTTP/3 come segnali semantici.
4. Non sostituire gli standard agent-readable: questi restano priorità prima del render.

## Criterio di accettazione

1. `go test ./...` passa.
2. Carratelli non deve tornare a shell-only quando il renderer recupera dati network testuali.
3. Il trace deve rendere ispezionabile se i dati sono arrivati da DOM o da risorse network.
4. I payload network devono avere budget e truncation espliciti.
