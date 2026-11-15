# Build stage
FROM golang:1.24.4-bookworm AS builder

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN go test ./...
RUN CGO_ENABLED=1 go build -o assistant-api .

# Default cloud runtime: chat and memory only, without Go or Git tooling.
FROM debian:bookworm-slim AS runtime
RUN apt-get update \
    && apt-get install -y --no-install-recommends ca-certificates \
    && rm -rf /var/lib/apt/lists/*
WORKDIR /app
COPY --from=builder /app/assistant-api .
EXPOSE 8000
CMD ["./assistant-api"]
