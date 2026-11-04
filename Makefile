PROTOC ?= protoc

.PHONY: generate build tidy docker-up docker-down

generate:
	protoc --proto_path=proto \
		--go_out=. --go_opt=module=flight-booking \
		--go-grpc_out=. --go-grpc_opt=module=flight-booking \
		proto/flight/v1/flight.proto

build:
	go build ./cmd/flight ./cmd/booking

tidy:
	go mod tidy

docker-up:
	docker compose up --build -d

docker-down:
	docker compose down -v