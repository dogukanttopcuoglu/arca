# reranker-service (M10, ADR-0049): CUDA build — the GPU reservation in
# docker-compose only works when the wheel itself carries CUDA kernels.
FROM python:3.11-slim

WORKDIR /app

COPY requirements.txt ./
RUN pip install --no-cache-dir -r requirements.txt \
    && pip install --no-cache-dir torch --index-url https://download.pytorch.org/whl/cu121

COPY main.py ./

EXPOSE 3003

CMD ["uvicorn", "main:app", "--host", "0.0.0.0", "--port", "3003"]
