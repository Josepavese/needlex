# Vercel Browser Agent Bridge

Questo file definisce il bridge minimo per usare `Vercel Browser Agent` nel benchmark competitivo.

Non presuppone un endpoint pubblico standard di Vercel.
Presuppone che tu deployi un piccolo endpoint tuo su Vercel che parli con il tuo browser agent stack.

## Env usate dal runner

1. `VERCEL_BROWSER_AGENT_ENDPOINT`
2. `VERCEL_BROWSER_AGENT_TOKEN` opzionale

## Request Contract

`POST` JSON:

```json
{
  "id": "zai-coding-plan",
  "family": "same_site_query",
  "language": "en",
  "seed_url": "https://docs.z.ai/guides/overview/quick-start",
  "task_type": "same_site_query_routing",
  "goal": "coding plan",
  "expected_domain": "docs.z.ai",
  "must_contain_facts": ["supports coding tools", "lists supported models or plans"]
}
```

## Response Contract

```json
{
  "selected_url": "https://docs.z.ai/devpack/overview",
  "summary": "supports coding tools and lists supported models or plans",
  "text": "optional longer text",
  "chunks": [
    { "text": "supports coding tools and lists supported models or plans" }
  ],
  "latency_ms": 321
}
```

Campi accettati:
1. `selected_url` oppure `url`
2. `summary`
3. `text` opzionale
4. `chunks[].text` opzionale
5. `latency_ms` opzionale

## File esempio

Bridge stub di esempio:
1. [vercel_browser_agent_bridge_example.ts](../benchmarks/competitive/runner/vercel_browser_agent_bridge_example.ts)

## Regola

Questo comparator resta:
1. valido per routing/navigation/browser-guided tasks
2. non isomorfo a Needle-X su proof/provenance
