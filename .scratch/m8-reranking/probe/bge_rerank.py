#!/usr/bin/env python3
"""M8 probe tooling: BGE cross-encoder reranker (ADR-0045, probe-side only).

Speaks the NDJSON protocol of the Go probe ExecReranker adapter:
  request:  {"query": "...", "candidates": [{"chunk_id": "...", "content": "..."}]}
  response: {"ordering": [{"chunk_id": "...", "score": 0.9}], "model_load_ms": 123, "rss_bytes": 456}

Model: BAAI/bge-reranker-v2-m3 (cross-encoder family).
"""
import json
import resource
import sys
import time

import torch

from sentence_transformers import CrossEncoder


def main() -> None:
    start = time.time()
    model = CrossEncoder("BAAI/bge-reranker-v2-m3", max_length=1024)
    load_ms = int((time.time() - start) * 1000)

    for line in sys.stdin:
        line = line.strip()
        if not line:
            continue
        req = json.loads(line)
        pairs = [(req["query"], c.get("content", "")) for c in req["candidates"]]
        # Small batches + per-request cache eviction keep the long-lived
        # process stable on WSL/CUDA (CUBLAS execution failures appeared on
        # later requests of the same process).
        scores = model.predict(pairs, batch_size=16)
        scored = sorted(
            zip([c["chunk_id"] for c in req["candidates"]], scores),
            key=lambda x: -x[1],
        )
        resp = {
            "ordering": [
                {"chunk_id": cid, "score": float(s)} for cid, s in scored
            ],
            "model_load_ms": load_ms,
            "rss_bytes": resource.getrusage(resource.RUSAGE_SELF).ru_maxrss * 1024,
        }
        sys.stdout.write(json.dumps(resp) + "\n")
        sys.stdout.flush()
        del scores
        torch.cuda.empty_cache()


if __name__ == "__main__":
    main()
