FROM golang:1.26.3-alpine AS builder
WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY cmd ./cmd
COPY gen ./gen
COPY internal ./internal

RUN CGO_ENABLED=0 GOOS=linux go build -o /flight ./cmd/flight

FROM alpine:latest
RUN apk --no-cache add ca-certificates
WORKDIR /app
COPY --from=builder /flight /app/flight
EXPOSE 50051
ENTRYPOINT ["/app/flight"]