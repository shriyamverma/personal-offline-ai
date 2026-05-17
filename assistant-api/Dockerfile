FROM golang:1.24

WORKDIR /app

COPY go.mod .
COPY go.sum .

RUN go mod download

COPY . .

ENV CGO_ENABLED=1

RUN go build -o assistant-api .

CMD ["./assistant-api"]