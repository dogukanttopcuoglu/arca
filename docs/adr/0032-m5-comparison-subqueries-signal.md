# M5 uses the existing SubQueries comparison signal

Status: accepted

M5 identifies comparison queries solely through the existing `QueryAnalyzer` result: a non-empty `AnalyzedQuery.SubQueries` means comparison decomposition applies. The orchestrator consumes this validated capability signal and does not add a comparison classifier, a general `IntentHint`, or duplicate the decomposition matcher.

## Consequences

- Comparison routing is `Balanced` plus the existing deterministic decomposition and `MergeRankedLists` path.
- Non-comparison routing remains `Balanced` without decomposition.
- `DenseBiased` remains benchmark/configuration machinery only until new evidence supports runtime use.
