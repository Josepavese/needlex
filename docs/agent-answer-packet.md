# Agent Answer Packet

`Agent Answer Packet` e' il contratto di default per l'output compatto di Needle-X.

Non e' un dump del runtime.
E' il payload minimo che un'altra AI deve poter consumare senza preprocessing aggiuntivo.

## Obiettivo

Il packet deve rispondere in quest'ordine:
1. che tipo di risposta e'
2. qual e' il locator principale
3. qual e' la risposta breve
4. quanta incertezza c'e'
5. quali evidenze minime la sostengono
6. quali alternative o motivi di selezione esistono
7. quanto e' costato il run

## Regole

1. default transport output = `Agent Answer Packet`
2. `proof`, `trace`, `web_ir` completo e artifact completi restano disponibili, ma espliciti
3. nessuna dipendenza da euristiche linguistiche per `uncertainty`
4. `summary` deve derivare dal miglior chunk strutturale disponibile, non da generazione libera

## V1 Shape

Campi comuni:
1. `kind`
2. locator primario
3. `summary`
4. `uncertainty`
5. `chunks`
6. `signals`
7. `cost_report`

Regola per `chunks`:
1. il packet compatto privilegia copertura informativa, non cardinalita' fissa
2. puo' restituire meno chunk del cap se quelli successivi sono ridondanti rispetto ai precedenti
3. puo' scartare tail chunks strutturalmente deboli quando esiste gia' un anchor esplicativo forte

Regola per `signals`:
1. `confidence` e `substrate_class` descrivono la superficie compilata
2. `content_source` descrive la provenienza del contenuto: assente quando la pagina non e' stata renderizzata, `dom` quando il contenuto viene dal DOM renderizzato, `dom+network` quando hanno contribuito anche payload applicativi catturati (fetch, XHR, SSE, WebSocket)
3. `network_resources` e `network_bytes` quantificano l'evidenza applicativa trattenuta
4. `network_truncated` segnala che la cattura e' stata tagliata da un budget o da uno stream ancora aperto
5. `render_degraded` segnala che il percorso browser e' fallito e l'evidenza applicativa non e' stata osservata
6. i campi di provenienza compaiono solo per letture che hanno coinvolto il rendering: le letture statiche restano senza rumore aggiuntivo

Regola per `evidence`:
1. il campo compare solo quando l'estrazione e' insufficiente, mai sulle pagine ordinarie
2. forma: `{"coverage":"title_only"|"thin"|"none","reason":"..."}`
3. `title_only` = i chunk consegnati contengono solo titolo o heading, non il corpo che serve all'obiettivo
4. `thin` = il corpo consegnato e' piu' corto di una o due frasi
5. `none` = nessun testo di corpo utilizzabile estratto
6. la copertura limitata aggiunge un motivo di `uncertainty` (`title_only_extraction`, `thin_extraction`, `no_extraction`) e alza `uncertainty.level`
7. `analytics.proof_backed` resta veritiero (esistono proof ref) ma non e' un giudizio di adeguatezza della risposta: un packet `title_only` puo' essere proof-backed

Campi query-specific:
1. `selected_url`
2. `selection_why`
3. `candidates`

Regola per `document_extraction`:
1. il campo compare solo quando la sorgente e' un PDF testuale; non aggiunge rumore alle letture HTML
2. schema stabile: `needlex.document.read.v1`
3. `raw_sha256` e' lo SHA-256 completo dei byte ricevuti e `source_bytes` la loro dimensione
4. `pages_total`, `pages_processed`, `pages_with_text` e `pages_failed[]` rendono conto delle pagine
5. `truncated` e `coverage` (`complete`|`partial`|`none`) dichiarano i limiti dell'estrazione
6. `limits` riporta i budget effettivi: byte, pagine, caratteri di output e timeout
7. i chunk PDF hanno `page_start`/`page_end`; il proof span ha `page` e il selettore `/pdf/page[N]/block[M]`
8. errori distinti: `document_encrypted`, `document_malformed`, `document_no_text`, `document_unsupported`, `document_limit_exceeded`, `document_timeout`; `render_not_applicable` per `render=required`
9. nessun OCR, nessun `pdftotext` e nessun rendering browser per i PDF

## `read` Example

```json
{
  "kind": "page_read",
  "url": "https://example.com/page",
  "title": "Example Page",
  "summary": "This page explains the core runtime and supported tools.",
  "uncertainty": {
    "level": "low"
  },
  "chunks": [
    {
      "text": "This page explains the core runtime and supported tools.",
      "heading_path": ["Overview"],
      "source_url": "https://example.com/page",
      "source_selector": "/article[1]/p[1]",
      "proof_ref": "proof_123"
    }
  ],
  "signals": {
    "confidence": 0.91,
    "substrate_class": "generic_content"
  },
  "web_ir_summary": {
    "node_count": 72,
    "heading_ratio": 0.36,
    "short_text_ratio": 0.78,
    "substrate_class": "generic_content"
  },
  "cost_report": {
    "latency_ms": 396,
    "token_in": 0,
    "token_out": 0,
    "lane_path": [0]
  }
}
```

## `read` Example With Rendered Application Data

Quando la lettura passa dal rendering e cattura payload applicativi, `signals` aggiunge la provenienza:

```json
{
  "kind": "page_read",
  "url": "https://example.com/app",
  "title": "Registrar Overview",
  "summary": "The page lists registrar records with pricing after the application loads.",
  "uncertainty": {
    "level": "low"
  },
  "chunks": [
    {
      "text": "Registrar record: Aurora, 320 square metres, price 1,250,000 euro.",
      "heading_path": ["Records"],
      "source_url": "https://example.com/api/records",
      "source_selector": "/network/resource[1]",
      "proof_ref": "proof_789"
    }
  ],
  "signals": {
    "confidence": 0.93,
    "substrate_class": "rendered_html",
    "content_source": "dom+network",
    "network_resources": 8,
    "network_bytes": 951884
  },
  "web_ir_summary": {
    "node_count": 103,
    "substrate_class": "rendered_html"
  },
  "cost_report": {
    "latency_ms": 9043,
    "token_in": 0,
    "token_out": 0,
    "lane_path": [0, 4]
  }
}
```

Note:
1. un chunk può avere come `source_url` l'endpoint applicativo invece della pagina, perché l'evidenza di rete entra nel substrate con provenance `/network/resource[N]`
2. se la cattura è stata tagliata, `network_truncated` è `true` e l'agente deve considerare che esistono dati non materializzati
3. se il percorso browser è fallito, `render_degraded` è `true` e `content_source` resta `dom`

## `read` Example With PDF Document Extraction

Un PDF testuale aggiunge `document_extraction` e conserva la provenienza per pagina:

```json
{
  "kind": "page_read",
  "url": "https://pdfobject.com/pdf/sample.pdf",
  "title": "Sample PDF",
  "document_extraction": {
    "schema": "needlex.document.read.v1",
    "format": "pdf",
    "raw_sha256": "229defbb0cee6f02673a5cde290d0673e75a0dc31cec43989c8ab2a4eca7e1bb",
    "source_bytes": 18810,
    "pages_total": 1,
    "pages_processed": 1,
    "pages_with_text": 1,
    "truncated": false,
    "coverage": "complete",
    "limits": {
      "max_bytes": 4000000,
      "max_pages": 100,
      "max_output_chars": 200000,
      "timeout_ms": 5000
    }
  },
  "chunks": [
    {
      "text": "Sample PDF This is a simple PDF file. Fun fun fun.",
      "source_url": "https://pdfobject.com/pdf/sample.pdf",
      "source_selector": "/pdf/page[1]/block[1]",
      "page_start": 1,
      "page_end": 1,
      "proof_ref": "proof_123"
    }
  ],
  "signals": {
    "confidence": 0.93,
    "substrate_class": "pdf_text",
    "content_source": "pdf_text"
  }
}
```

Note:
1. `raw_sha256` permette di verificare i byte della sorgente indipendentemente dal packet
2. `coverage` parziale o `truncated` impongono di non dichiarare copertura completa
3. `render=required` su un PDF fallisce con `render_not_applicable`: usare `render=off` o omettere il parametro
4. un PDF scansionato o solo immagine restituisce `document_no_text`; l'OCR richiede uno strumento esterno

## `query` Example

```json
{
  "kind": "goal_query",
  "goal": "coding plan",
  "seed_url": "https://docs.z.ai/guides/overview/quick-start",
  "selected_url": "https://docs.z.ai/devpack/overview",
  "summary": "This page describes the coding plan, supported tools, and model access.",
  "uncertainty": {
    "level": "low"
  },
  "selection_why": [
    "structure_hint",
    "domain_identity_match",
    "semantic_goal_alignment"
  ],
  "chunks": [
    {
      "text": "Access to high-intelligence Coding Model...",
      "heading_path": ["Usage", "Advantages"],
      "source_url": "https://docs.z.ai/devpack/overview",
      "source_selector": "/div[...]/ul[1]/li[1]",
      "proof_ref": "proof_456"
    }
  ],
  "candidates": [
    {
      "url": "https://docs.z.ai/devpack/overview",
      "label": "Coding Plan",
      "reason": ["structure_hint", "semantic_goal_alignment"]
    }
  ],
  "signals": {
    "confidence": 0.91,
    "substrate_class": "generic_content"
  },
  "cost_report": {
    "latency_ms": 412,
    "token_in": 0,
    "token_out": 0,
    "lane_path": [0]
  }
}
```

## Non-Goals

Il packet di default non deve includere:
1. `trace` completo
2. `proof_records` completi
3. `web_ir.nodes`
4. `document` raw completo
5. `result_pack` diagnostico completo

Questi artifact restano disponibili tramite:
1. `--json-mode full`
2. `needlex proof <trace-id|proof-ref|chunk-id>`
3. `needlex replay`
4. `needlex diff`
