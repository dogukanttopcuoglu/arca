# M5 keeps Balanced as the production retrieval path

Status: accepted

M5 keeps the frozen `Balanced` policy as the production retrieval path for all requests. Comparison quality is improved through the already validated deterministic decomposition, not through runtime `DenseBiased` selection; `DenseBiased` remains available only to benchmark/configuration machinery until new evidence justifies a second runtime route.

The production runtime must also align with the frozen M4 `RETRIEVAL_MIN_SCORE=0.6` operating point. This is consistency with an existing calibration decision, not new M5 retrieval tuning.
