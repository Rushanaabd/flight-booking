FROM golang:1.26.3-alpine AS builder
WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY cmd ./cmd
COPY gen ./gen
COPY internal ./internal

RUN CGO_ENABLED=0 GOOS=linux go build -o /booking ./cmd/booking

FROM alpine:latest
RUN apk --no-cache add ca-certificates
WORKDIR /app
COPY --from=builder /booking /app/booking
EXPOSE 8080
ENTRYPOINT ["/app/booking"]