# Output compatto e copertura delle evidenze di web_read

Nota di un agente esterno per il team Needle-X, su mandato di Jose del
15 ottobre 2026. Ho scritto solo questo file in issues/: nessuna modifica,
commit, push o esecuzione del progetto Needle-X.

## Osservazione e ambiente

Codex desktop, tool MCP web_read e skill globale needlex-web-retrieval
(versione dichiarata v0.1.37). Sei letture della conversazione del 15 ottobre,
con obiettivo semantico esplicito e retrieval_effort=light. Sono prove del
risultato restituito al client, non una diagnosi del codice corrente del repo;
non e' stata acquisita una revisione Git del progetto per rispettare il perimetro.

1. https://arxiv.org/abs/2502.08235 — trace_ec1ce58eae36738c,
   proof_1095bb3675178d77: il chunk contiene solo il titolo (96 caratteri),
   senza l'abstract necessario alla domanda. Risultato uncertainty=low,
   confidence=0.93 e proof_backed=true.
2. https://arxiv.org/abs/2605.05980 — trace_15ac591fe1d200e6,
   proof_357a92dcdc07ceb0: ancora solo titolo (98 caratteri), con gli stessi
   indicatori positivi. Il successivo tool web del client leggeva gli abstract:
   non e' prova di pagina vuota.
3. Sulle sei risposte, la serializzazione integrale MCP/structuredContent
   occupava circa 268.529 caratteri, contro 27.452 della sola sezione compact.
   Erano presenti duplicazioni fra content testuale, structuredContent,
   compact, agent_context e web_ir. Il client ha inizialmente inoltrato
   erroneamente tutto il payload: e' anche un errore d'uso del chiamante,
   non tutto costo imputabile al motore o token di inferenza misurati.

Altre trace del campione: trace_88da5b4cd548aa5c,
trace_8ed1fbfa8633f383, trace_f34dbad91b33eec1, trace_8bac8bb4d034d8d5.

## Modifica desiderata

- Rendere naturale un singolo output compatto per l'agente; tenere DOM/raw,
  rappresentazioni duplicate e diagnostica completa dietro riferimenti o
  richiesta esplicita, preservando gli usi programmatici del risultato.
- Distinguere provenienza verificabile del chunk e copertura dell'obiettivo:
  proof_backed non deve suggerire risposta adeguata quando e' provato solo
  il titolo. Esporre estrazione insufficiente e motivo quando mancano le
  informazioni richieste, senza mascherarle con confidence elevata.
- Aggiornare la skill distribuita con selezione del payload MCP compatto e
  fallback proporzionato alla lacuna. Evitare una seconda lettura identica
  quando il client dispone gia' di evidenze sufficienti da un'altra fonte.

## Accettazione proposta

Con una domanda sull'abstract, i due URL restituiscono contenuto pertinente
oppure un esito esplicito di copertura insufficiente. Il percorso ordinario
per l'agente non include il DOM integrale o piu' copie dello stesso contenuto;
le prove restano accessibili. Misurare separatamente byte del payload, token
modellati, latenza e qualita', senza dedurre l'una dalle altre.

Il team decide contratto API, eventuali compatibilita' e soluzione tecnica.
Non propongo pulizie dei trace/proof esistenti o nuove infrastrutture obbligatorie.

## Risoluzione (v0.1.46)

La lettura titolo-solo descritta qui non era piu' riproducibile sul main
v0.1.45: le letture live di https://arxiv.org/abs/2502.08235 e
https://arxiv.org/abs/2605.05980 restituivano l'abstract con
`uncertainty=medium`. La correzione dell'estrazione era quindi gia' presente;
restavano il payload MCP duplicato e la segnalazione esplicita della copertura.

v0.1.46 chiude i due punti residui:

1. `web_read` e `web_query` restituiscono di default un solo packet compatto
   in `structuredContent` piu' `trace_path`, `proof_path` e `fingerprint_path`.
   `include_diagnostics=true` espone la diagnostica completa annidata
   (`document`, `web_ir`, `result_pack`, `agent_context`; `plan` per la query).
   `web_crawl` espone una sola forma compatta.
2. Il packet compatto aggiunge `evidence` (`coverage` = `title_only` | `thin` |
   `none`, con `reason`) solo quando l'estrazione consegnata e' insufficiente,
   alza `uncertainty.level` con un motivo esplicito e non presenta
   `analytics.proof_backed` come adeguatezza della risposta.
3. La skill distribuita documenta la selezione del payload, il fallback
   proporzionato e la distinzione fra provenienza e copertura.

Misure MCP live sul binary v0.1.46 (caratteri di `structuredContent`):
`web_read` arxiv 2502.08235 da 71.471 a 7.881 (-90,4%) con `content.text`
invariato a 7.543; example.com da 15.326 a 4.305 (diagnostica 14.984);
`web_query` example.com da 24.748 a 4.863 (diagnostica 20.310);
`web_crawl` example.com da 513 a 250.

Rischi residui: le soglie 24/160 rune alfanumeriche sono euristiche
conservative, non calibrate su un corpus; una pagina breve ma completa puo'
ricevere `thin`. Il testo umano della CLI non stampa ancora `evidence` (il
JSON compatto si'). `memory` e `analytics` mantengono il vecchio helper di
duplicazione, fuori dal perimetro di questa correzione.
