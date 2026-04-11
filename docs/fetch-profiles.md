# Fetch Profiles

Needle-X now treats fetch hardening as a product capability, not as a benchmark-only tweak.

## Default

Current defaults:
1. `fetch.profile = browser_like`
2. `fetch.retry_profile = hardened`

Why:
1. the product default should maximize successful acquisition on real-world targets
2. anti-bot failures in `acquire` make the rest of the runtime irrelevant

## Profiles

### `standard`

Use when you want a cleaner baseline for:
1. benchmark comparability
2. transport debugging
3. controlled regression analysis

### `browser_like`

Use for normal product operation.

Behavior:
1. browser-like user agent by default
2. browser impersonation for `https` targets
3. better success rate on anti-bot-protected sites

### `hardened`

Use as a stronger retry profile for difficult targets.

Behavior:
1. browser impersonation
2. explicit Chrome-like TLS fingerprinting

## Overrides

Environment:

```bash
export NEEDLEX_FETCH_PROFILE=standard
export NEEDLEX_FETCH_RETRY_PROFILE=standard
```

Config:

```json
{
  "fetch": {
    "profile": "browser_like",
    "retry_profile": "hardened"
  }
}
```

Accepted values:
1. `standard`
2. `browser_like`
3. `hardened`

## Live Validation Snapshot

Observed on real targets after this change:
1. `https://www.coni.it/`
   `standard` -> `503`
   `browser_like` -> successful page read
2. `https://www.gazzetta.it/`
   `standard` -> successful page read
   `browser_like` -> successful page read

Read this correctly:
1. browser-like default improves acquisition on some blocked targets
2. it is not a claim that every anti-bot system is solved

## Roadmap

Tracked here:
1. [Fetch Resilience Roadmap](roadmap/fetch-resilience-roadmap.md)

This roadmap is intentionally about:
1. compatibility
2. pacing
3. observability
4. benchmark discipline

It is not a roadmap for stealth or anti-bot evasion.

## Traceability

The `acquire` stage trace now records:
1. `fetch_mode`
2. `fetch_profile`
3. `final_url`
4. `retry_count`
5. `retry_reason`
6. `retry_sleep_ms`
7. `host_pacing_ms`
