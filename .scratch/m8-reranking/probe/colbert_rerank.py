#!/usr/bin/env python3
"""M8 probe tooling: ColBERTv2 late-interaction reranker (ADR-0045, probe-side).

Speaks the NDJSON protocol of the Go probe ExecReranker adapter:
  request:  {"query": "...", "candidates": [{"chunk_id": "...", "content": "..."}]}
  response: {"ordering": [{"chunk_id": "...", "score": 0.9}], "model_load_ms": 123, "rss_bytes": 456}

Model: colbert-ir/colbertv2.0 (late-interaction family). Candidates are
embedded on the fly (the ~3K-chunk ARC corpus is small enough); scoring uses
token-wise MaxSim with mean pooling, matching the ColBERT ranking objective.
"""
import json
import resource
import sys
import time

import torch

from colbert.modeling.checkpoint import Checkpoint
from colbert.utils.utils import torch_load_dottable


def maxsim_scores(query_embs: torch.Tensor, doc_embs: torch.Tensor) -> torch.Tensor:
    # query_embs: (qlen, dim); doc_embs: (n, dlen, dim)
    sim = torch.matmul(query_embs, doc_embs.transpose(1, 2))  # (qlen, n, dlen)
    sim_max, _ = sim.max(dim=2)  # (qlen, n)
    return sim_max.mean(dim=0)  # (n,)


def main() -> None:
    start = time.time()
    checkpoint = Checkpoint("colbert-ir/colbertv2.0")
    load_ms = int((time.time() - start) * 1000)

    for line in sys.stdin:
        line = line.strip()
        if not line:
            continue
        req = json.loads(line)
        docs = [c.get("content", "") for c in req["candidates"]]
        chunk_ids = [c["chunk_id"] for c in req["candidates"]]

        with torch.inference_mode():
            q = checkpoint.queryFromText([req["query"]])[0]  # (qlen, dim)
            d = checkpoint.docFromText(docs)  # (n, dlen, dim)
            scores = maxsim_scores(q, d).tolist()

        scored = sorted(zip(chunk_ids, scores), key=lambda x: -x[1])
        resp = {
            "ordering": [
                {"chunk_id": cid, "score": float(s)} for cid, s in scored
            ],
            "model_load_ms": load_ms,
            "rss_bytes": resource.getrusage(resource.RUSAGE_SELF).ru_maxrss * 1024,
        }
        sys.stdout.write(json.dumps(resp) + "\n")
        sys.stdout.flush()


if __name__ == "__main__":
    main()
