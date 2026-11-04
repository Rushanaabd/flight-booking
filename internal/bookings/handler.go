package bookings

import (
	"errors"
	flightv1 "flight-booking/gen/flight/v1"
	"flight-booking/internal/util"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"log"
	"net/http"
	"time"
)

type Handler struct {
	repo    *Repo
	flights *FlightClient
}

func NewHandler(repo *Repo, flights *FlightClient) *Handler {
	return &Handler{repo: repo, flights: flights}
}

func RegisterRoutes(r *gin.Engine, h *Handler) {
	r.GET("/flights", h.ListFlights)
	r.GET("/flights/:id", h.GetFlight)
	r.POST("/bookings", h.CreateBooking)
	r.GET("/bookings", h.ListBookings)
	r.GET("/bookings/:id", h.GetBooking)
	r.POST("/bookings/:id/cancel", h.CancelBooking)
}

func httpStatus(err error) (int, string) {
	code := status.Convert(err).Code()
	switch code {
	case codes.NotFound:
		return http.StatusNotFound, "NOT_FOUND"
	case codes.ResourceExhausted:
		return http.StatusConflict, "RESOURCE_EXHAUSTED"
	case codes.AlreadyExists:
		return http.StatusConflict, "ALREADY_EXISTS"
	case codes.InvalidArgument:
		return http.StatusBadRequest, "BAD_REQUEST"
	case codes.FailedPrecondition:
		return http.StatusConflict, "CONFLICT"
	case codes.Unauthenticated:
		return http.StatusServiceUnavailable, "SERVICE_AUTH_ERROR"
	case codes.Unavailable:
		return http.StatusServiceUnavailable, "SERVICE_UNAVAILABLE"
	default:
		return http.StatusBadGateway, "UPSTREAM_ERROR"
	}
}

func failFromGRPC(c *gin.Context, err error) {
	st, code := httpStatus(err)
	util.Error(c, st, code, status.Convert(err).Message(), nil)
}

func (h *Handler) toFlightResp(f *flightv1.Flight) gin.H {
	return gin.H{
		"id":              f.GetId(),
		"flight_number":   f.GetFlightNumber(),
		"airline":         f.GetAirline(),
		"origin":          f.GetOrigin(),
		"destination":     f.GetDestination(),
		"departure_time":  f.GetDepartureTime().AsTime(),
		"arrival_time":    f.GetArrivalTime().AsTime(),
		"total_seats":     f.GetTotalSeats(),
		"available_seats": f.GetAvailableSeats(),
		"price":           f.GetPrice(),
		"status":          f.GetStatus().String(), // FLIGHT_STATUS_SCHEDULED
	}
}

func (h *Handler) bookingResp(b *Booking) gin.H {
	return gin.H{
		"id":              b.ID,
		"user_id":         b.UserID,
		"flight_id":       b.FlightID,
		"passenger_name":  b.PassengerName,
		"passenger_email": b.PassengerEmail,
		"seat_count":      b.SeatCount,
		"total_price":     b.TotalPrice,
		"status":          b.Status,
		"created_at":      b.CreatedAt,
	}
}

func (h *Handler) ListFlights(c *gin.Context) {
	origin, destination := c.Query("origin"), c.Query("destination")
	if origin == "" || destination == "" {
		util.Error(c, http.StatusBadRequest, "VALIDATION_ERROR", "origin and destination are required", nil)
		return
	}
	var date *time.Time
	if raw := c.Query("date"); raw != "" {
		t, err := time.Parse("2006-01-02", raw)
		if err != nil {
			util.Error(c, http.StatusBadRequest, "VALIDATION_ERROR", "date must be YYYY-MM-DD", nil)
			return
		}
		date = &t
	}
	resp, err := h.flights.SearchFlights(c.Request.Context(), origin, destination, date)
	if err != nil {
		failFromGRPC(c, err)
		return
	}
	out := make([]gin.H, 0, len(resp.GetFlights()))
	for _, f := range resp.GetFlights() {
		out = append(out, h.toFlightResp(f))
	}
	c.JSON(http.StatusOK, out)
}

func (h *Handler) GetFlight(c *gin.Context) {
	resp, err := h.flights.GetFlight(c.Request.Context(), c.Param("id"))
	if err != nil {
		failFromGRPC(c, err)
		return
	}
	c.JSON(http.StatusOK, h.toFlightResp(resp.GetFlight()))
}

type createBookingReq struct {
	UserID         string `json:"user_id"`
	FlightID       string `json:"flight_id"`
	PassengerName  string `json:"passenger_name"`
	PassengerEmail string `json:"passenger_email"`
	SeatCount      int32  `json:"seat_count"`
}

func (h *Handler) CreateBooking(c *gin.Context) {
	var req createBookingReq
	if err := c.ShouldBindJSON(&req); err != nil {
		util.Error(c, http.StatusBadRequest, "VALIDATION_ERROR", err.Error(), nil)
		return
	}
	if req.UserID == "" || req.FlightID == "" || req.PassengerName == "" || req.PassengerEmail == "" {
		util.Error(c, http.StatusBadRequest, "VALIDATION_ERROR", "user_id, flight_id, passenger_name, passenger_email are required", nil)
		return
	}
	if req.SeatCount <= 0 {
		util.Error(c, http.StatusBadRequest, "VALIDATION_ERROR", "seat_count must be positive", nil)
		return
	}

	flight, err := h.flights.GetFlight(c.Request.Context(), req.FlightID)
	if err != nil {
		failFromGRPC(c, err)
		return
	}

	bookingID := uuid.NewString()
	if _, err = h.flights.ReserveSeats(c.Request.Context(), req.FlightID, bookingID, req.SeatCount); err != nil {
		failFromGRPC(c, err)
		return
	}

	booking := &Booking{
		ID:             bookingID,
		UserID:         req.UserID,
		FlightID:       req.FlightID,
		PassengerName:  req.PassengerName,
		PassengerEmail: req.PassengerEmail,
		SeatCount:      req.SeatCount,
		TotalPrice:     float64(req.SeatCount) * flight.GetFlight().GetPrice(),
	}
	if err = h.repo.Create(booking); err != nil {
		log.Printf("create booking %s failed, releasing reservation: %v", bookingID, err)
		if _, rerr := h.flights.ReleaseReservation(c.Request.Context(), bookingID); rerr != nil {
			log.Printf("release reservation for booking %s failed: %v", bookingID, rerr)
		}
		util.Error(c, http.StatusBadGateway, "BOOKING_FAILED", "failed to create booking", nil)
		return
	}
	c.JSON(http.StatusCreated, h.bookingResp(booking))
}

func (h *Handler) ListBookings(c *gin.Context) {
	userID := c.Query("user_id")
	if userID == "" {
		util.Error(c, http.StatusBadRequest, "VALIDATION_ERROR", "user_id is required", nil)
		return
	}
	list, err := h.repo.ListByUser(userID)
	if err != nil {
		util.Error(c, http.StatusInternalServerError, "INTERNAL", err.Error(), nil)
		return
	}
	out := make([]gin.H, 0, len(list))
	for i := range list {
		out = append(out, h.bookingResp(&list[i]))
	}
	c.JSON(http.StatusOK, out)
}

func (h *Handler) GetBooking(c *gin.Context) {
	b, err := h.repo.GetByID(c.Param("id"))
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			util.Error(c, http.StatusNotFound, "NOT_FOUND", "booking not found", nil)
			return
		}
		util.Error(c, http.StatusInternalServerError, "INTERNAL", err.Error(), nil)
		return
	}
	c.JSON(http.StatusOK, h.bookingResp(b))
}

func (h *Handler) CancelBooking(c *gin.Context) {
	id := c.Param("id")
	b, err := h.repo.GetByID(id)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			util.Error(c, http.StatusNotFound, "NOT_FOUND", "booking not found", nil)
			return
		}
		util.Error(c, http.StatusInternalServerError, "INTERNAL", err.Error(), nil)
		return
	}
	if b.Status != "CONFIRMED" {
		util.Error(c, http.StatusConflict, "CONFLICT", "booking is not in CONFIRMED status", nil)
		return
	}
	if _, err = h.flights.ReleaseReservation(c.Request.Context(), id); err != nil {
		failFromGRPC(c, err)
		return
	}
	if err = h.repo.Cancel(id); err != nil {
		util.Error(c, http.StatusBadGateway, "CANCEL_FAILED", "failed to cancel booking", nil)
		return
	}
	b.Status = "CANCELLED"
	c.JSON(http.StatusOK, h.bookingResp(b))
}
