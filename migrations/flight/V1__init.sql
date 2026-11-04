CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

CREATE TYPE flight_status AS ENUM ('SCHEDULED', 'DEPARTED', 'CANCELLED', 'COMPLETED');
CREATE TYPE reservation_status AS ENUM ('ACTIVE', 'RELEASED', 'EXPIRED');

CREATE TABLE flights (
  id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
  flight_number VARCHAR(10) NOT NULL,
  airline VARCHAR(100) NOT NULL,
  origin CHAR(3) NOT NULL,
  destination CHAR(3) NOT NULL,
  departure_date DATE NOT NULL,
  departure_time TIMESTAMPTZ NOT NULL,
  arrival_time TIMESTAMPTZ NOT NULL,
  total_seats INTEGER NOT NULL CHECK (total_seats > 0),
  available_seats INTEGER NOT NULL CHECK (available_seats >= 0),
  price NUMERIC(10,2) NOT NULL CHECK (price > 0),
  status flight_status NOT NULL DEFAULT 'SCHEDULED',
  CONSTRAINT available_not_gt_total CHECK (available_seats <= total_seats),
  CONSTRAINT unique_flight_per_date UNIQUE (flight_number, departure_date)
);

CREATE TABLE seat_reservations (
  id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
  flight_id UUID NOT NULL REFERENCES flights(id),
  booking_id UUID NOT NULL,
  seat_count INTEGER NOT NULL CHECK (seat_count > 0),
  status reservation_status NOT NULL DEFAULT 'ACTIVE',
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  CONSTRAINT unique_booking_reservation UNIQUE (flight_id, booking_id),
  CONSTRAINT available_min_seats CHECK (seat_count > 0)
);