# Build stage
FROM golang:1.24 AS builder

WORKDIR /app

COPY go.mod ./
COPY go.sum ./
RUN go mod download 

COPY . ./

RUN go build -o examtopicsdl ./main.go

FROM debian:bookworm-slim

WORKDIR /app

COPY --from=builder /app/examtopicsdl .

CMD ["./examtopicsdl"]
