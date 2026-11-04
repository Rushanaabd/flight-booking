# flight-booking

Бронирование авиабилетов: два микросервиса с раздельными БД
Go + Gin (REST) + gRPC + PostgreSQL + Flyway + Redis (Sentinel)
Client (REST), Booking Service (gRPC), Flight Service; у каждого своя PostgreSQL, во Flight Service - кеш Cache-Aside в Redis Sentinel

## Архитектура

- Booking Service (:8080) - REST API для клиентов, бронирования в своей PostgreSQL. При создании брони вызывает Flight Service по gRPC: GetFlight (цена) -> ReserveSeats (атомарный захват мест), фиксирует snapshot цены. Для gRPC-вызовов - retry (3 попытки, exponential backoff 100/200/400ms, только UNAVAILABLE/DEADLINE_EXCEEDED)
- Flight Service (:50051) - gRPC API, рейсы и резервации в своей PostgreSQL, кеш в Redis (Sentinel: master + replica + sentinel), ключи `flight:{id}` и `search:{origin}:{destination}:{date}` с TTL 5 минут, инвалидация при мутациях. Проверяет API-ключ в metadata (x-api-key)
- Миграции - flyway, применяются автоматически при старте compose
- ER-диаграмма (3NF) - docs/er.md

## Запуск

docker compose up --build

Поднимаются 2 postgres (flight_db :5433, booking_db :5434), redis sentinel (master + replica + sentinel), flyway-миграции и оба сервиса

## Проверка

Поиск рейсов:

curl 'http://localhost:8080/flights?origin=SVO&destination=LED'

Рейс по id (404 если нет):

curl http://localhost:8080/flights/<id>

Создать бронирование (total_price = seat_count * price рейса):

curl -s -X POST localhost:8080/bookings -H 'Content-Type: application/json' \
  -d '{"user_id":"user-1","flight_id":"<id рейса>","passenger_name":"Ivanov Ivan","passenger_email":"ivan@ex.com","seat_count":2}'

Нехватка мест или рейс не SCHEDULED - 409, бронирование не создаётся

Бронирование по id и список бронирований пользователя:

curl localhost:8080/bookings/<id>
curl 'localhost:8080/bookings?user_id=user-1'

Отменить бронирование (возвращает места, статус CANCELLED):

curl -X POST localhost:8080/bookings/<id>/cancel

Кеш: два одинаковых поиска подряд - в логах flight cache MISS, затем cache HIT; после бронирования/отмены кеш инвалидируется

Failover Redis Sentinel:

docker compose stop redis-master   # sentinel повысит реплику (+switch-master в логах), кеш продолжит работать

Retry: docker compose stop flight - в логах booking три попытки с backoff, после исчерпания клиент получает 502 UPSTREAM_ERROR вместо таймаута (attempt 1/3 -> 100ms, 2/3 -> 200ms)

Данные в БД:

docker exec -it $(docker ps -qf name=flight-db) psql -U flight -d flight_db \
  -c "SELECT flight_number, available_seats, status FROM flights;" \
  -c "SELECT booking_id, seat_count, status FROM seat_reservations;"

docker exec -it $(docker ps -qf name=booking-db) psql -U booking -d booking_db \
  -c "SELECT passenger_name, seat_count, total_price, status FROM bookings;"

## Локальная сборка

make generate
go build ./cmd/flight ./cmd/booking