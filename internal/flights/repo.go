package flights

import (
	"database/sql"
	"errors"
	"fmt"
	"github.com/google/uuid"
	"github.com/lib/pq"
	"time"
)

var (
	ErrNotFound         = errors.New("flight not found")
	ErrNotEnoughSeats   = errors.New("not enough seats available")
	ErrNotScheduled     = errors.New("flight is not in SCHEDULED status")
	ErrInvalidSeatCount = errors.New("seat_count must be positive")
	ErrAlreadyReserved  = errors.New("active reservation already exists for booking")
)

type Flight struct {
	ID             string
	FlightNumber   string
	Airline        string
	Origin         string
	Destination    string
	DepartureTime  time.Time
	ArrivalTime    time.Time
	TotalSeats     int32
	AvailableSeats int32
	Price          float64
	Status         string
}

type Repo struct {
	db *sql.DB
}

func NewRepo(db *sql.DB) *Repo {
	return &Repo{db: db}
}

// ищет рейсы со статусом SCHEDULED по маршруту и опциональной дате
func (r *Repo) Search(origin, destination string, date *time.Time) ([]Flight, error) {
	q := `SELECT id, flight_number, airline, origin, destination,
	             departure_time, arrival_time, total_seats, available_seats, price, status
	      FROM flights
	      WHERE origin = $1 AND destination = $2 AND status = 'SCHEDULED'`
	args := []any{origin, destination}
	if date != nil {
		q += ` AND departure_date = $3::date`
		args = append(args, *date)
	}
	q += ` ORDER BY departure_time`

	rows, err := r.db.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Flight
	for rows.Next() {
		var f Flight
		if err = rows.Scan(&f.ID, &f.FlightNumber, &f.Airline, &f.Origin, &f.Destination,
			&f.DepartureTime, &f.ArrivalTime, &f.TotalSeats, &f.AvailableSeats, &f.Price, &f.Status); err != nil {
			return nil, err
		}
		out = append(out, f)
	}
	return out, rows.Err()
}

// возвращает рейс по ID
func (r *Repo) GetByID(id string) (*Flight, error) {
	var f Flight
	err := r.db.QueryRow(`SELECT id, flight_number, airline, origin, destination,
	                             departure_time, arrival_time, total_seats, available_seats, price, status
	                      FROM flights WHERE id = $1`, id).
		Scan(&f.ID, &f.FlightNumber, &f.Airline, &f.Origin, &f.Destination,
			&f.DepartureTime, &f.ArrivalTime, &f.TotalSeats, &f.AvailableSeats, &f.Price, &f.Status)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &f, nil
}

// атомарно резервирует места: SELECT FOR UPDATE, уменьшение available_seats, создание seat_reservations, т.е. все в одной транзакции
func (r *Repo) ReserveSeats(flightID, bookingID string, seatCount int32) (string, error) {
	if seatCount <= 0 {
		return "", ErrInvalidSeatCount
	}

	tx, err := r.db.Begin()
	if err != nil {
		return "", err
	}
	defer tx.Rollback()

	var status string
	var available int32
	err = tx.QueryRow(`SELECT available_seats, status FROM flights WHERE id = $1 FOR UPDATE`, flightID).
		Scan(&available, &status)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrNotFound
	}
	if err != nil {
		return "", err
	}
	if status != "SCHEDULED" {
		return "", ErrNotScheduled
	}
	if int32(available) < seatCount {
		return "", ErrNotEnoughSeats
	}

	if _, err = tx.Exec(`UPDATE flights SET available_seats = available_seats - $1 WHERE id = $2`,
		seatCount, flightID); err != nil {
		return "", err
	}

	reservationID := uuid.NewString()
	_, err = tx.Exec(`INSERT INTO seat_reservations (id, flight_id, booking_id, seat_count, status)
	                  VALUES ($1, $2, $3, $4, 'ACTIVE')`,
		reservationID, flightID, bookingID, seatCount)
	if err != nil {
		var pqErr *pq.Error
		if errors.As(err, &pqErr) && pqErr.Code == "23505" {
			// повторный ReserveSeats с тем же booking_id не создаёт дубликат - возвращается существующая резервация
			var existingID, existingStatus string
			serr := tx.QueryRow(`SELECT id, status FROM seat_reservations
			                     WHERE flight_id = $1 AND booking_id = $2`, flightID, bookingID).
				Scan(&existingID, &existingStatus)
			if serr == nil && existingStatus == "ACTIVE" {
				return existingID, nil
			}
			return "", ErrAlreadyReserved
		}
		return "", fmt.Errorf("insert reservation: %w", err)
	}

	if err = tx.Commit(); err != nil {
		return "", err
	}
	return reservationID, nil
}

// возвращает места и переводит резервацию в RELEASED в одной транзакции
func (r *Repo) ReleaseReservation(bookingID string) (string, string, error) {
	tx, err := r.db.Begin()
	if err != nil {
		return "", "", err
	}
	defer tx.Rollback()

	var reservationID, flightID string
	var seatCount int32
	err = tx.QueryRow(`SELECT id, flight_id, seat_count FROM seat_reservations
	                   WHERE booking_id = $1 AND status = 'ACTIVE' FOR UPDATE`, bookingID).
		Scan(&reservationID, &flightID, &seatCount)
	if errors.Is(err, sql.ErrNoRows) {
		return "", "", ErrNotFound
	}
	if err != nil {
		return "", "", err
	}

	if _, err = tx.Exec(`UPDATE seat_reservations SET status = 'RELEASED' WHERE id = $1`,
		reservationID); err != nil {
		return "", "", err
	}
	if _, err = tx.Exec(`UPDATE flights SET available_seats = available_seats + $1 WHERE id = $2`,
		seatCount, flightID); err != nil {
		return "", "", err
	}

	if err = tx.Commit(); err != nil {
		return "", "", err
	}
	return reservationID, flightID, nil
}
