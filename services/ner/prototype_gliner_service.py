# PROTOTYPE — GLiNER Entity Extraction Provider
# THROWAWAY CODE — Do not merge to main.
# Question: Does GLiNER zero-shot NER improve entity recall over RuleBased?
# Verdict: See benchmark notes at the bottom of this file.
#
# One command to run:
#   pip install fastapi uvicorn gliner
#   python services/ner/prototype_gliner_service.py
#
# Then test with:
#   python services/ner/prototype_benchmark.py

from fastapi import FastAPI
from pydantic import BaseModel
from typing import List
import uvicorn

app = FastAPI(title="ARC NER Microservice — GLiNER Prototype", version="0.0.1-prototype")


# ---------------------------------------------------------------------------
# Request / Response contracts
# These match the Go GLiNEREntityExtractor HTTP contract in gliner_client.go
# ---------------------------------------------------------------------------

class ExtractEntitiesRequest(BaseModel):
    text: str
    labels: List[str] = ["person", "organization", "location", "work_of_art", "product"]


class EntityResult(BaseModel):
    text: str
    label: str
    confidence: float
    start: int
    end: int


class ExtractEntitiesResponse(BaseModel):
    entities: List[EntityResult]


# ---------------------------------------------------------------------------
# GLiNER model — lazy loaded on first request
# ---------------------------------------------------------------------------

_model = None

def get_model():
    global _model
    if _model is None:
        from gliner import GLiNER
        print("[prototype] Loading GLiNER model: urchade/gliner_mediumv2.1 ...")
        _model = GLiNER.from_pretrained("urchade/gliner_mediumv2.1")
        print("[prototype] GLiNER model loaded.")
    return _model


# ---------------------------------------------------------------------------
# Endpoint
# ---------------------------------------------------------------------------

@app.post("/extract-entities", response_model=ExtractEntitiesResponse)
def extract_entities(req: ExtractEntitiesRequest):
    """
    Zero-shot NER via GLiNER.
    Labels are passed dynamically — no retraining required.
    """
    model = get_model()
    raw = model.predict_entities(req.text, req.labels, threshold=0.5)

    results = []
    for e in raw:
        results.append(EntityResult(
            text=e["text"],
            label=e["label"],
            confidence=round(float(e["score"]), 4),
            start=e["start"],
            end=e["end"],
        ))

    return ExtractEntitiesResponse(entities=results)


@app.get("/health")
def health():
    return {"status": "ok", "model": "urchade/gliner_mediumv2.1", "phase": "prototype"}


# ---------------------------------------------------------------------------
# Run
# ---------------------------------------------------------------------------

if __name__ == "__main__":
    uvicorn.run(app, host="0.0.0.0", port=8088, log_level="info")
