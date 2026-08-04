# M5 preserves HybridRetriever and SetFusionPolicy

Status: accepted

M5 does not alter `HybridRetriever`, `FusionPolicy`, or `SetFusionPolicy`. Production retrievers are configured before serving requests and never mutate policy per request; offline evaluation may continue to set a policy before execution. Runtime policy routing is deferred until a second policy has benchmark-backed value.

## Consequences

- The existing `Balanced` retriever remains safe as a shared production dependency because M5 does not mutate it during requests.
- M4 benchmark machinery remains reproducible.
- A future runtime policy decision will require a separate immutability/design change rather than an implicit M5 expansion.
