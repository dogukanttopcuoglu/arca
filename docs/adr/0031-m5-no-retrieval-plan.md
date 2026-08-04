# M5 does not introduce RetrievalPlan

Status: accepted

M5 does not introduce a first-class `RetrievalPlan`. The validated runtime decision surface contains only comparison decomposition, represented by a minimal internal decision containing `decompose: bool`; retrieval mode, TopK, thresholds, and fusion parameters remain owned by existing configuration and frozen M4 machinery.

## Considered Options

- A general plan object owning policy, mode, TopK, thresholds, and decomposition was rejected as speculative ownership.
- A policy-aware retrieval seam was rejected because M5 has no benchmark-backed second runtime policy.
