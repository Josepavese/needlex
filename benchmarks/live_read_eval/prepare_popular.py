#!/usr/bin/env python3
"""Freeze existing Tranco/CrUX rankings into the installed MCP reader's corpus.

Tranco provides the domain order; CrUX provides real web origins. This creates
no popularity ranking. Selection is uniform within declared popularity bands,
not weighted by traffic, business categories or Needle-X success.
"""
import argparse
from collections import Counter, defaultdict
import csv
import gzip
import hashlib
import json
from pathlib import Path
from urllib.parse import urlsplit
import urllib.request

TOTAL_URLS = 1000
SELECTION_SEED = "needlex-popular-web-v1"
EXECUTION = dict(memory_max_bytes=2 * 1024**3, memory_swap_max_bytes=0,
                 cpu_quota="200%", rpc_timeout_s=300, state_growth_mib=1024,
                 repetitions=1, profile="deep", render="auto")
RANK_BANDS = (("1-1000", 1, 1000), ("1001-5000", 1001, 5000), ("5001-10000", 5001, 10000))
QUESTIONS = {
    "en": "Identify the organization, service or publication represented by this page and describe its main purpose using facts from the source.",
    "it": "Identifica l'organizzazione, il servizio o la pubblicazione rappresentata dalla pagina e descrivine lo scopo principale con fatti della fonte.",
    "fr": "Identifie l'organisation, le service ou la publication de cette page et decris son objectif principal avec des faits provenant de la source.",
    "de": "Identifiziere die Organisation, den Dienst oder die Publikation dieser Seite und beschreibe den Hauptzweck anhand von Fakten aus der Quelle.",
    "es": "Identifica la organizacion, el servicio o la publicacion de esta pagina y describe su finalidad principal con hechos de la fuente.",
    "zh": "识别此页面代表的组织、服务或出版物，并依据来源中的事实说明其主要用途。",
}
# An independently reviewed reference panel, declared before Needle-X runs.
# Every included origin must still be present in both external ranking inputs.
REFERENCE_DOMAINS = {
    "nasa.gov": "science", "si.edu": "culture", "un.org": "international_public_service",
    "gov.uk": "public_service", "mit.edu": "education", "loc.gov": "archives",
    "ikea.com": "commerce", "airbnb.com": "travel_service", "booking.com": "travel_service",
    "bbc.com": "media", "theguardian.com": "media", "lemonde.fr": "media_fr",
    "elpais.com": "media_es", "dw.com": "media_de", "rai.it": "media_it",
    "nhk.or.jp": "media_ja", "archive.org": "archives", "openlibrary.org": "culture",
}


def digest(path):
    return hashlib.sha256(path.read_bytes()).hexdigest()


def domain_for(host, domains):
    parts = host.lower().rstrip(".").split(".")
    for start in range(len(parts)):
        suffix = ".".join(parts[start:])
        if suffix in domains:
            return suffix
    return None


def selection_key(domain):
    return hashlib.sha256((SELECTION_SEED + ":" + domain).encode()).hexdigest()


def build(root, legacy_path):
    sources = json.loads((root / "ranking-sources.json").read_text())
    for key in ["tranco", "crux"]:
        record = sources[key]
        if digest(root / record["file"]) != record["sha256"]:
            raise ValueError("ranking input hash mismatch: " + key)
    tranco = {domain: int(rank) for rank, domain in csv.reader((root / sources["tranco"]["file"]).open())}
    origins, exclusions = {}, Counter()
    with gzip.open(root / sources["crux"]["file"], "rt") as data:
        for row in csv.DictReader(data):
            parsed = urlsplit(row["origin"])
            if parsed.scheme not in ("http", "https") or not parsed.hostname or parsed.username or parsed.password:
                exclusions["not_public_http_origin"] += 1
                continue
            domain = domain_for(parsed.hostname, tranco)
            if domain is None:
                exclusions["outside_tranco_prefix"] += 1
                continue
            # Prefer the highest published CrUX bucket, HTTPS and the domain's
            # root/www origin; stable lexical URL order breaks identity ties.
            key = (int(row["rank"]), parsed.scheme != "https", parsed.hostname not in (domain, "www." + domain), row["origin"])
            if domain not in origins or key < origins[domain][0]:
                origins[domain] = (key, row["origin"])
    legacy = json.loads(legacy_path.read_text())["cases"]
    cases, by_url, legacy_domains = [], {}, set()
    for old in legacy:
        url = old["expected_url"]
        if url in by_url:
            by_url[url]["origins"].append(old["id"])
            continue
        case = dict(old, cohort="historical", origins=[old["id"]], source_language=old.get("language", "und"))
        cases.append(case)
        by_url[url] = case
        host = urlsplit(url).hostname
        legacy_domains.add(domain_for(host, tranco) or host.removeprefix("www."))
    eligible = {domain: value for domain, value in origins.items() if domain not in legacy_domains}
    reference = sorted(set(REFERENCE_DOMAINS) & set(eligible), key=lambda d: tranco[d])
    selected = list(reference)
    groups = defaultdict(list)
    for domain in eligible:
        if domain in reference:
            continue
        band = next(name for name, lo, hi in RANK_BANDS if lo <= tranco[domain] <= hi)
        groups[band].append(domain)
    for group in groups.values():
        group.sort(key=selection_key)
    # Equal round-robin allocation makes sample balance explicit without
    # fabricating semantic categories or replacing failed websites.
    while len(cases) + len(selected) < TOTAL_URLS:
        progress = False
        for name, _, _ in RANK_BANDS:
            if groups[name] and len(cases) + len(selected) < TOTAL_URLS:
                selected.append(groups[name].pop(0))
                progress = True
        if not progress:
            raise ValueError("not enough eligible ranked domains")
    languages = list(QUESTIONS)
    for index, domain in enumerate(selected):
        key, origin = eligible[domain]
        language = languages[index % len(languages)]
        case = dict(id="popular-" + domain, family=domain, language=language,
                    source_language="und", seed_url=origin + "/", expected_url=origin + "/",
                    expected_domain=domain, task_type="read_page_understanding", goal=QUESTIONS[language],
                    must_contain_facts=[], must_expose_proof=True, cohort="ranked",
                    origins=["popular-" + domain], tranco_rank=tranco[domain], crux_bucket=key[0],
                    popularity_band=next(name for name, lo, hi in RANK_BANDS if lo <= tranco[domain] <= hi),
                    reference_panel=REFERENCE_DOMAINS.get(domain))
        cases.append(case)
    if len(cases) != TOTAL_URLS or len({c["expected_url"] for c in cases}) != TOTAL_URLS:
        raise ValueError("corpus is not exactly 1000 distinct URLs")
    metadata = json.loads((root / sources["tranco_metadata"]["file"]).read_text())
    return dict(specification=SELECTION_SEED, execution=EXECUTION, sources=sources, tranco_configuration=metadata,
                legacy_sha256=digest(legacy_path), selection=dict(seed=SELECTION_SEED, rank_bands=RANK_BANDS,
                method="historical unique URLs + declared reference panel + equal round-robin hash sampling across Tranco bands, restricted to published CrUX web origins",
                exclusions=dict(exclusions), legacy_duplicate_occurrences=len(legacy)-len(by_url),
                selected_reference_domains=reference, ranked_eligible_domains=len(eligible),
                scope="1000 URL tasks, 99 historical and 901 ranked; question language is known, source language is not inferred from TLD. No traffic-weighted representativeness or independent-organization claim. No outcome-based replacement."), cases=cases)


def fetch_frozen(inputs, corpus_path):
    sources = json.loads(corpus_path.read_text())["sources"]
    inputs.mkdir(parents=True, exist_ok=True)
    for key in ["tranco_metadata", "tranco", "crux"]:
        record = sources[key]
        if Path(record["file"]).name != record["file"]:
            raise ValueError("source filename must remain inside owned inputs")
        path = inputs / record["file"]
        if path.exists():
            if digest(path) != record["sha256"]:
                raise ValueError("existing input differs; preserving it: " + str(path))
            continue
        with urllib.request.urlopen(record["url"], timeout=60) as response:
            data = response.read(record["bytes"] + 1)
        if len(data) != record["bytes"] or hashlib.sha256(data).hexdigest() != record["sha256"]:
            raise ValueError("upstream frozen source bytes changed: " + key)
        path.write_bytes(data)
    manifest = inputs / "ranking-sources.json"
    if manifest.exists() and json.loads(manifest.read_text()) != sources:
        raise ValueError("existing source manifest differs; preserving it")
    manifest.write_text(json.dumps(sources, indent=2) + "\n")


def main():
    p = argparse.ArgumentParser(description=__doc__)
    p.add_argument("--inputs", type=Path, required=True)
    p.add_argument("--legacy", type=Path, default=Path("benchmarks/corpora/unique-sources-corpus-v1.json"))
    p.add_argument("--out", type=Path, required=True)
    p.add_argument("--fetch-from", type=Path, help="fetch only missing pinned ranking inputs from an existing corpus, verifying exact hashes/lengths")
    a = p.parse_args()
    if a.fetch_from:
        fetch_frozen(a.inputs, a.fetch_from)
    data = build(a.inputs, a.legacy)
    a.out.write_text(json.dumps(data, ensure_ascii=False, indent=2) + "\n")
    print(json.dumps(dict(cases=len(data["cases"]), cohorts=dict(Counter(c["cohort"] for c in data["cases"])),
                         bands=dict(Counter(c.get("popularity_band", "historical") for c in data["cases"])),
                         reference=data["selection"]["selected_reference_domains"])))


if __name__ == "__main__":
    main()
