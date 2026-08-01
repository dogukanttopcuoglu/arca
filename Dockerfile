# Stage 1: Build Go binary
FROM golang:alpine AS builder

ENV GOTOOLCHAIN=auto

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux go build -o pdfinspector ./cmd/demo

# Stage 2: Minimal runtime image
FROM alpine:latest

WORKDIR /app

COPY --from=builder /app/pdfinspector /app/pdfinspector

ENTRYPOINT ["/app/pdfinspector"]
