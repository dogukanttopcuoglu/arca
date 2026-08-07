"""reranker-service: provider-agnostic reranking HTTP service (M10, ADR-0049).

Exposes the minimal rerank contract — the retrieval engine depends on this
HTTP contract, never on the model implementation:

    POST /rerank
    request:  {"query": "...", "candidates": [{"id": "...", "text": "..."}], "top_k": N?}
    response: {"ranked_ids": ["..."], "scores": [0.9, ...]}

    GET /health -> 200 only after the model is ready.

First model implementation: BAAI/bge-reranker-v2-m3 (cross-encoder). Model
choice lives entirely here; swapping models never touches the Go side.
"""

import os
import resource
import time

import torch
from fastapi import FastAPI, HTTPException
from pydantic import BaseModel
from sentence_transformers import CrossEncoder

MODEL_NAME = os.environ.get("RERANKER_MODEL", "BAAI/bge-reranker-v2-m3")
MAX_LENGTH = int(os.environ.get("RERANKER_MAX_LENGTH", "1024"))
BATCH_SIZE = int(os.environ.get("RERANKER_BATCH_SIZE", "16"))


class Candidate(BaseModel):
    id: str
    text: str = ""


class RerankRequest(BaseModel):
    query: str
    candidates: list[Candidate]
    top_k: int | None = None


class RerankResponse(BaseModel):
    ranked_ids: list[str]
    scores: list[float]


def load_model() -> CrossEncoder:
    # Loaded once per service lifetime. A load failure must fail the process:
    # a running container with a lazy model is a hidden cold-start.
    model = CrossEncoder(MODEL_NAME, max_length=MAX_LENGTH)
    return model


model: CrossEncoder | None = None
model_load_ms: int = 0

app = FastAPI(title="reranker-service")


@app.on_event("startup")
def startup() -> None:
    global model, model_load_ms
    start = time.time()
    try:
        model = load_model()
    except Exception as exc:  # noqa: BLE001 - fail fast with a clear signal
        raise SystemExit(f"reranker-service: model load failed: {exc}") from exc
    model_load_ms = int((time.time() - start) * 1000)


@app.get("/health")
def health() -> dict:
    if model is None:
        raise HTTPException(status_code=503, detail="model not ready")
    return {"status": "ok", "model": MODEL_NAME, "ready": True}


@app.post("/rerank", response_model=RerankResponse)
def rerank(req: RerankRequest) -> RerankResponse:
    if model is None:
        raise HTTPException(status_code=503, detail="model not ready")
    if not req.query or not req.candidates:
        raise HTTPException(status_code=400, detail="query and candidates are required")

    pairs = [(req.query, c.text) for c in req.candidates]
    # Small batches + per-request cache eviction keep the long-lived process
    # stable on CUDA (CUBLAS execution failures appeared on later requests of
    # the same process during the M8 probe).
    scores = model.predict(pairs, batch_size=BATCH_SIZE)
    scored = sorted(
        zip([c.id for c in req.candidates], scores),
        key=lambda x: -x[1],
    )
    resp = RerankResponse(
        ranked_ids=[cid for cid, _ in scored],
        scores=[float(s) for _, s in scored],
    )
    del scores
    torch.cuda.empty_cache()
    return resp


def rss_bytes() -> int:
    return resource.getrusage(resource.RUSAGE_SELF).ru_maxrss * 1024
