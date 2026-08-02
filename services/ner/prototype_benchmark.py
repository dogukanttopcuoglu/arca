# PROTOTYPE — Entity Extraction Benchmark
# THROWAWAY CODE — Do not merge to main.
# Question: RuleBased vs GLiNER vs Hybrid entity recall on Rick Rubin text.
#
# Run: python services/ner/prototype_benchmark.py
# (Requires GLiNER service running on localhost:8088)

import requests
import json
import time

# ---------------------------------------------------------------------------
# Benchmark corpus — Rick Rubin test sentences from inspection_result
# ---------------------------------------------------------------------------

BENCHMARK_TEXT = (
    "Rick Rubin founded Def Jam Recordings in New York alongside Russell Simmons. "
    "Def Jam Recordings released legendary hip-hop albums in New York. "
    "Creative expression is a fundamental human drive. "
    "To create without expectation is the essence of art."
)

LABELS = ["person", "organization", "location", "work_of_art", "product"]

GLINER_ENDPOINT = "http://localhost:8088/extract-entities"

# ---------------------------------------------------------------------------
# Rule-Based extraction (pure Python mirror of Go RuleBasedEntityExtractor)
# ---------------------------------------------------------------------------

import re

ORG_PATTERN = re.compile(
    r'\b([A-Z][a-z0-9]+(?:\s+[A-Z][a-z0-9]+)*\s+(?:Inc|Corp|Corporation|Ltd|Limited|LLC|Recordings|Records|Group|Bank|University|Agency))\b'
)

KNOWN_PERSONS   = ["Rick Rubin", "Mustafa Kemal Atatürk"]
KNOWN_LOCATIONS = ["New York", "Ankara"]


def rule_based_extract(text: str) -> list[dict]:
    entities = []
    seen = set()

    # Orgs
    for m in ORG_PATTERN.finditer(text):
        key = ("organization", m.group(0).strip().lower())
        if key not in seen:
            seen.add(key)
            entities.append({"text": m.group(0).strip(), "label": "organization", "confidence": 0.85, "source": "rule"})

    # Persons
    for p in KNOWN_PERSONS:
        if p.lower() in text.lower():
            key = ("person", p.lower())
            if key not in seen:
                seen.add(key)
                entities.append({"text": p, "label": "person", "confidence": 0.90, "source": "rule"})

    # Locations
    for loc in KNOWN_LOCATIONS:
        if loc.lower() in text.lower():
            key = ("location", loc.lower())
            if key not in seen:
                seen.add(key)
                entities.append({"text": loc, "label": "location", "confidence": 0.88, "source": "rule"})

    return entities


# ---------------------------------------------------------------------------
# GLiNER extraction
# ---------------------------------------------------------------------------

def gliner_extract(text: str) -> list[dict]:
    t0 = time.perf_counter()
    try:
        resp = requests.post(
            GLINER_ENDPOINT,
            json={"text": text, "labels": LABELS},
            timeout=10
        )
        latency_ms = round((time.perf_counter() - t0) * 1000, 1)
        if resp.status_code != 200:
            print(f"[GLiNER] Non-200 response: {resp.status_code}")
            return [], latency_ms
        data = resp.json()
        entities = [
            {**e, "source": "gliner"}
            for e in data.get("entities", [])
        ]
        return entities, latency_ms
    except Exception as ex:
        latency_ms = round((time.perf_counter() - t0) * 1000, 1)
        print(f"[GLiNER] Service unreachable: {ex}")
        return [], latency_ms


# ---------------------------------------------------------------------------
# Hybrid merge — Union with confidence-based deduplication
# ---------------------------------------------------------------------------

def hybrid_merge(rule_entities: list[dict], gliner_entities: list[dict]) -> list[dict]:
    merged = {}

    for e in rule_entities:
        key = (e["label"], e["text"].lower())
        merged[key] = {**e, "source": "rule"}

    for e in gliner_entities:
        key = (e["label"], e["text"].lower())
        if key in merged:
            # Keep higher confidence
            if e["confidence"] > merged[key]["confidence"]:
                merged[key] = {**e, "source": "hybrid(gliner-wins)"}
            else:
                merged[key]["source"] = "hybrid(rule-wins)"
        else:
            merged[key] = {**e, "source": "hybrid(gliner-only)"}

    return list(merged.values())


# ---------------------------------------------------------------------------
# Benchmark runner
# ---------------------------------------------------------------------------

def print_section(title: str):
    print(f"\n{'='*60}")
    print(f"  {title}")
    print(f"{'='*60}")


def print_entities(entities: list[dict], latency_ms: float = None):
    if not entities:
        print("  (none)")
        return
    for e in entities:
        conf = f"conf={e['confidence']:.2f}" if 'confidence' in e else ""
        src  = f"  [{e.get('source', '')}]" if 'source' in e else ""
        print(f"  • {e['label']:15s}  {e['text']:30s}  {conf}{src}")
    if latency_ms is not None:
        print(f"\n  Latency: {latency_ms}ms")


def run_benchmark():
    print("\n" + "="*60)
    print("  ARC Entity Extraction Benchmark — PROTOTYPE")
    print("  Document: The Creative Act — Rick Rubin")
    print("="*60)
    print(f"\nBenchmark text:\n  \"{BENCHMARK_TEXT[:100]}...\"")

    # 1. RuleBased
    print_section("1. RuleBased Extractor")
    t0 = time.perf_counter()
    rule_entities = rule_based_extract(BENCHMARK_TEXT)
    rule_latency = round((time.perf_counter() - t0) * 1000, 1)
    print_entities(rule_entities, rule_latency)

    # 2. GLiNER
    print_section("2. GLiNER Extractor")
    gliner_entities, gliner_latency = gliner_extract(BENCHMARK_TEXT)
    print_entities(gliner_entities, gliner_latency)

    # 3. Hybrid
    print_section("3. Hybrid Extractor (Union + Confidence Merge)")
    hybrid_entities = hybrid_merge(rule_entities, gliner_entities)
    print_entities(hybrid_entities)

    # 4. Recall Analysis
    print_section("4. Recall Analysis")

    known_entities = {
        ("person", "rick rubin"),
        ("organization", "def jam recordings"),
        ("location", "new york"),
        ("person", "russell simmons"),   # The recall gap case
    }

    rule_found  = {(e["label"], e["text"].lower()) for e in rule_entities}
    gliner_found = {(e["label"], e["text"].lower()) for e in gliner_entities}
    hybrid_found = {(e["label"], e["text"].lower()) for e in hybrid_entities}

    def recall(found):
        hits = known_entities & found
        return len(hits) / len(known_entities), hits

    rule_r,   rule_hits   = recall(rule_found)
    gliner_r, gliner_hits = recall(gliner_found)
    hybrid_r, hybrid_hits = recall(hybrid_found)

    print(f"\n  {'Extractor':<20} {'Recall':>8}  {'Entities':>8}  Found")
    print(f"  {'-'*60}")
    print(f"  {'RuleBased':<20} {rule_r:>7.0%}  {len(rule_entities):>8}  {[h[1] for h in rule_hits]}")
    print(f"  {'GLiNER':<20} {gliner_r:>7.0%}  {len(gliner_entities):>8}  {[h[1] for h in gliner_hits]}")
    print(f"  {'Hybrid':<20} {hybrid_r:>7.0%}  {len(hybrid_entities):>8}  {[h[1] for h in hybrid_hits]}")

    missing_rule   = known_entities - rule_found
    missing_gliner = known_entities - gliner_found
    missing_hybrid = known_entities - hybrid_found

    if missing_rule:
        print(f"\n  ⚠ RuleBased missing: {[m[1] for m in missing_rule]}")
    if missing_gliner:
        print(f"\n  ⚠ GLiNER missing: {[m[1] for m in missing_gliner]}")
    if not missing_hybrid:
        print(f"\n  ✅ Hybrid: No missing entities!")

    # 5. Verdict
    print_section("5. Benchmark Verdict")
    if hybrid_r >= gliner_r and hybrid_r >= rule_r:
        winner = "C) Hybrid EntityExtractor"
        reason = "Best recall. RuleBased provides high-precision anchors; GLiNER fills recall gaps."
    elif gliner_r > rule_r:
        winner = "B) GLiNER only"
        reason = "GLiNER outperforms RuleBased recall."
    else:
        winner = "A) RuleBased only"
        reason = "GLiNER did not improve recall — service may be unavailable."

    print(f"\n  Winner: {winner}")
    print(f"  Reason: {reason}")
    print(f"\n  Recommendation for ARC:")
    print(f"    → Keep RuleBased as high-precision fallback (no network dependency).")
    print(f"    → Add GLiNER as secondary provider when service is available.")
    print(f"    → Implement HybridEntityExtractor merging both outputs.")
    print(f"    → Context pointer: prototype/gliner-entity-extraction branch")
    print()


if __name__ == "__main__":
    run_benchmark()
