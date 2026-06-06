# Build stage
FROM golang:1.24 AS builder

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=1 go build -o assistant-api .

# Runtime stage (small)
FROM debian:bookworm-slim

WORKDIR /app

COPY --from=builder /app/assistant-api .

EXPOSE 8000

CMD ["./assistant-api"]