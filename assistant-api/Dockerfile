# Build stage
FROM golang:1.24.4-bookworm AS builder

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN go test ./...
RUN CGO_ENABLED=1 go build -o assistant-api .

# Runtime stage
# Keep Go available because the assistant's shell tool runs Go commands.
FROM golang:1.24.4-bookworm

RUN apt-get update \
    && apt-get install -y --no-install-recommends git \
    && rm -rf /var/lib/apt/lists/* \
    && git config --global --add safe.directory /workspace

WORKDIR /app

COPY --from=builder /app/assistant-api .

EXPOSE 8000

CMD ["./assistant-api"]
