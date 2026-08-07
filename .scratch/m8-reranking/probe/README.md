# M8 Reranking Probe — tooling notes

Probe-side model script (ADR-0045: benchmark tooling, never production code).
Speaks the NDJSON protocol of the Go `ExecReranker` adapter
(`internal/eval/probe/exec.go`):

```
request:  {"query": "...", "candidates": [{"chunk_id": "...", "content": "..."}]}
response: {"ordering": [{"chunk_id": "...", "score": 0.9}], "model_load_ms": 123, "rss_bytes": 456}
```

## Setup

```bash
python -m venv .venv && .venv/bin/pip install -r requirements.txt
# first run downloads the BGE-reranker-v2-m3 model from Hugging Face (~1.1GB)
```

## Run

1. Collect the candidate artifact from the production baseline (needs the
   live Qdrant + graph stores running):

   ```bash
   arc eval probe collect \
     --goldset internal/eval/testdata/goldset_v3.json \
     --artifact-out .scratch/m8-reranking/artifact_v3.json \
     --candidate-top-n 100
   ```

2. Run the probe over the artifact (budget values are frozen BEFORE the
   benchmark starts — see the manifest; first calibrate from a warm-up run):

   ```bash
   arc eval probe run \
     --artifact .scratch/m8-reranking/artifact_v3_1.json \
     --goldset internal/eval/testdata/goldset_v3_1.json \
     --bge-command ".venv/bin/python .scratch/m8-reranking/probe/bge_rerank.py" \
     --n 20,50,100 \
     --budget-p95-ms 750 \
     --budget-rss-bytes 4000000000 \
     --m5-gate \
     --report .scratch/m8-reranking/probe_manifest_v3.json
   ```

   Selective reranking (research E1/E2):
   - `--bge-intents entity` — gate the BGE reranker to entity queries only
     (gated-out queries keep the baseline ordering, measured byte-identical)
   - `--structure --structure-intents heading` — deterministic heading-overlap
     bonus reranker (model-free, E2)
   - `--gate-runs 3` — repeat each gate evaluation and take the lower median
     decision, stabilizing the verified rate against LLM variance (single-shot
     noise floor ±3.4..7pp exceeds the ±1pp MAR threshold; M7 BULGU-2 rule)

## Determinism

- The artifact is generated once and reused for every combination —
  retrieval variance is fully eliminated.
- Re-running the probe over the same artifact yields the same orderings
  (ordering contract, ADR-0044) and the same bootstrap CI (fixed seed).
- The manifest carries the artifact fingerprint; production activation is
  locked to it (ADR-0046).

## Kill gate (ADR-0045)

Thresholds are frozen constants in `internal/eval/probe/gate.go`:
MPI nDCG@5 >= +1 pp; MAR MRR >= -0.5 pp; MAR verified >= -1 pp;
abstention = hard invariant. Budget values are frozen in the manifest before
the benchmark. Selection: highest quality; within 5% tolerance, smallest N.
"None accepted" closes M8 with production unchanged.
