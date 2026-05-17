FROM golang:1.24-alpine

WORKDIR /app

COPY go.mod .

COPY . .

RUN go build -o assistant-api .

CMD ["./assistant-api"]