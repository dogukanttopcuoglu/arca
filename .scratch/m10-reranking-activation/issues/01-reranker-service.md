# 01 — reranker-service (provider-agnostic Python HTTP service)

**What to build:** A Python microservice (`services/reranker-service/`) exposing the minimal `POST /rerank` contract — `{query, candidates:[{id,text}], top_k?}` → `{ranked_ids, scores}` — with the BGE cross-encoder as the first model. The model is loaded once at startup; if the model cannot be loaded the process **fails** (no hidden cold-start), and `/health` returns 200 only when the model is genuinely ready. The service joins docker-compose with a GPU reservation (the ollama pattern) and a healthcheck.

**Blocked by:** None — can start immediately.

**Status:** ready-for-agent

- [ ] `POST /rerank` implements the contract: ranked_ids order is the ordering, scores are informational; `top_k` optional and unused by the Go adapter
- [ ] Model (BAAI/bge-reranker-v2-m3) loads once at startup; load failure exits the process with a non-zero status and a clear error
- [ ] `/health` returns 200 only after the model is ready; 503 before that
- [ ] Stable on long-lived GPU processes (small batches, per-request cache eviction — the probe script's stability notes carry over)
- [ ] Docker: service in docker-compose with GPU `deploy.devices` reservation + healthcheck; model downloaded on first start
- [ ] Canned-pair smoke test passes against the running container (valid ranked_ids + scores response)
