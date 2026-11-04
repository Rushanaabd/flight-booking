package bookings

import (
	"database/sql"
	"errors"
	"time"

	_ "github.com/lib/pq"
)

var ErrNotFound = errors.New("booking not found")

type Booking struct {
	ID             string
	UserID         string
	FlightID       string
	PassengerName  string
	PassengerEmail string
	SeatCount      int32
	TotalPrice     float64
	Status         string
	CreatedAt      time.Time
}

type Repo struct {
	db *sql.DB
}

func NewRepo(db *sql.DB) *Repo {
	return &Repo{db: db}
}

// сохраняет бронирование со статусом CONFIRMED
func (r *Repo) Create(b *Booking) error {
	b.Status = "CONFIRMED"
	return r.db.QueryRow(`INSERT INTO bookings (id, user_id, flight_id, passenger_name, passenger_email, seat_count, total_price, status)
	                      VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
	                      RETURNING created_at`,
		b.ID, b.UserID, b.FlightID, b.PassengerName, b.PassengerEmail, b.SeatCount, b.TotalPrice, b.Status).
		Scan(&b.CreatedAt)
}

// возвращает бронирование по ID
func (r *Repo) GetByID(id string) (*Booking, error) {
	var b Booking
	err := r.db.QueryRow(`SELECT id, user_id, flight_id, passenger_name, passenger_email, seat_count, total_price, status, created_at
	                      FROM bookings WHERE id = $1`, id).
		Scan(&b.ID, &b.UserID, &b.FlightID, &b.PassengerName, &b.PassengerEmail,
			&b.SeatCount, &b.TotalPrice, &b.Status, &b.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &b, nil
}

// возвращает бронирования пользователя
func (r *Repo) ListByUser(userID string) ([]Booking, error) {
	rows, err := r.db.Query(`SELECT id, user_id, flight_id, passenger_name, passenger_email, seat_count, total_price, status, created_at
	                         FROM bookings WHERE user_id = $1 ORDER BY created_at DESC`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Booking
	for rows.Next() {
		var b Booking
		if err = rows.Scan(&b.ID, &b.UserID, &b.FlightID, &b.PassengerName, &b.PassengerEmail,
			&b.SeatCount, &b.TotalPrice, &b.Status, &b.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, b)
	}
	return out, rows.Err()
}

// переводит бронирование в CANCELLED
func (r *Repo) Cancel(id string) error {
	res, err := r.db.Exec(`UPDATE bookings SET status = 'CANCELLED' WHERE id = $1 AND status = 'CONFIRMED'`, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}
