# LLM generation via an OpenAI-compatible gateway; embeddings remain local via Ollama

Status: proposed

ARC separates the embedding runtime from the generation runtime. Embeddings are produced locally by Ollama (`nomic-embed-text`); answer generation is accessed through a single OpenAI-compatible HTTP adapter (chat/completions) configured by `LLM_BASE_URL`, `LLM_API_KEY`, and `LLM_MODEL`. AgentRouter (`https://agentrouter.org/v1`) is the current deployment target for the gateway, not a domain dependency. `LLM_PROVIDER` is an observability label only — the adapter never branches on it.

## Decision

**Embedding runtime — local:**
- Local Ollama, `nomic-embed-text`
- Optimized for indexing throughput (bulk, deterministic, batching)
- Privacy/cost/offline benefits: no network egress, no per-token cost, deterministic dimensions for differential indexing

**Generation runtime — OpenAI-compatible gateway:**
- Single OpenAI-compatible HTTP adapter (`chat/completions`, Bearer auth)
- AgentRouter as the current endpoint (`LLM_BASE_URL` default `https://agentrouter.org/v1`)
- Configurable through environment variables (`LLM_BASE_URL`, `LLM_API_KEY`, `LLM_MODEL`, `LLM_PROVIDER`)
- Provider-neutral adapter: compatible with any OpenAI-compatible gateway (vLLM, Groq, LocalAI, api.openai.com)

**Model identifier — runtime configuration, not architecture:**
`LLM_MODEL` is forwarded to the gateway as configured; ARC makes no assumption about the identifier. AgentRouter supports multiple model IDs, and the adapter must not validate, alias, or branch on the model value. Changing models requires editing configuration only — no code changes.

**`LLM_PROVIDER` — observability only:**
`LLM_PROVIDER` names the endpoint identity for `AnswerMetadata`, logs, and telemetry. It must never affect adapter behavior or introduce provider-specific branching.

## Context

M1 bootstrapped a fully local pipeline for inspection, enrichment, and vector indexing (Ollama embeddings + Qdrant). M2 wires the existing QA seams into a real generation pipeline. The machine is CPU-only, so local generation (llama3.2:3b) is slow and quality-limited for answer synthesis, while the bulk embedding path is well served locally. Choosing one OpenAI-compatible adapter over provider-specific adapters (native OpenAI, Anthropic, Ollama chat) keeps the `LLMProvider` seam free of vendor concepts: any OpenAI-compatible endpoint can be swapped in via config alone.

## Rationale

- **One seam, one adapter.** A single chat/completions client covers every endpoint that speaks the protocol; provider variety becomes configuration, not code.
- **Cloud for quality, local for volume.** Answer generation is user-facing and quality-sensitive (short, infrequent) — appropriate for a cloud gateway. Embedding is bulk, deterministic, and privacy-sensitive — appropriate to keep local.
- **Observability without coupling.** `LLM_PROVIDER` names the endpoint identity; runtime behavior is determined solely by `LLM_BASE_URL`, `LLM_API_KEY`, and `LLM_MODEL`.

## Trade-offs considered

- **Local generation via Ollama** (rejected for M2): fully offline and free, but slow on CPU (minutes per answer for 7B-class models) and weaker citation adherence; remains possible later by pointing `LLM_BASE_URL` at an OpenAI-compatible local server.
- **Provider-specific adapters** (rejected for M2): native access to provider-only features (e.g. Anthropic's top-level system field, native tool use, provider streaming formats), but multiplies adapter surface and leaks vendor concepts into the seam for no M2 benefit.
- **Capability-aware context budgeting** (rejected): coupling `ContextBuilder` to `LLMProvider.Capabilities()` leaks provider concerns into prompt assembly; the budget stays composition-owned config (`LLM_CONTEXT_BUDGET`).

## Consequences

- The seam only carries the OpenAI-compatible surface; native provider-specific features are intentionally outside it. Providers that do not expose chat/completions cannot be integrated without seam changes.
- Adding OpenAI/Anthropic/Ollama-as-compatible later is an additive config change — no interface modifications.
- Generation requires network egress and a valid API key; the no-evidence path and verification advisory semantics keep the pipeline usable when the gateway is unreachable (graceful degradation per ADR-0005).
- Phase 2 entailment verification remains an unimplemented seam (`EntailmentChecker`) — deliberately deferred; `VerificationStatus` (`verified`, `unverified`, `no_evidence`) derives entirely from structural citation checks.
