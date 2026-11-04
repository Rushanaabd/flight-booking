package flights

import (
	"context"
	"errors"
	flightv1 "flight-booking/gen/flight/v1"
	"fmt"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/types/known/timestamppb"
	"time"
)

type Server struct {
	flightv1.UnimplementedFlightServiceServer
	repo  *Repo
	cache *Cache
}

func NewServer(repo *Repo, cache *Cache) *Server {
	return &Server{repo: repo, cache: cache}
}

func toProto(f *Flight) *flightv1.Flight {
	return &flightv1.Flight{
		Id:             f.ID,
		FlightNumber:   f.FlightNumber,
		Airline:        f.Airline,
		Origin:         f.Origin,
		Destination:    f.Destination,
		DepartureTime:  timestamppb.New(f.DepartureTime),
		ArrivalTime:    timestamppb.New(f.ArrivalTime),
		TotalSeats:     f.TotalSeats,
		AvailableSeats: f.AvailableSeats,
		Price:          f.Price,
		Status:         flightStatusToProto(f.Status),
	}
}

func flightStatusToProto(s string) flightv1.FlightStatus {
	switch s {
	case "DEPARTED":
		return flightv1.FlightStatus_FLIGHT_STATUS_DEPARTED
	case "CANCELLED":
		return flightv1.FlightStatus_FLIGHT_STATUS_CANCELLED
	case "COMPLETED":
		return flightv1.FlightStatus_FLIGHT_STATUS_COMPLETED
	default:
		return flightv1.FlightStatus_FLIGHT_STATUS_SCHEDULED
	}
}

func searchKey(origin, destination string, date *time.Time) string {
	day := "all"
	if date != nil {
		day = date.Format("2006-01-02")
	}
	return fmt.Sprintf("search:%s:%s:%s", origin, destination, day)
}

// Cache-Aside - кеш, при miss Б, запись в кеш с TTL
func (s *Server) SearchFlights(ctx context.Context, req *flightv1.SearchFlightsRequest) (*flightv1.SearchFlightsResponse, error) {
	origin, destination := req.GetOrigin(), req.GetDestination()
	if origin == "" || destination == "" {
		return nil, status.Error(codes.InvalidArgument, "origin and destination are required")
	}

	var date *time.Time
	if req.GetDepartureDate() != nil {
		t := req.GetDepartureDate().AsTime()
		date = &t
	}
	sKey := searchKey(origin, destination, date)

	if s.cache != nil {
		if payload, ok := s.cache.Get(ctx, sKey); ok {
			var cached flightv1.SearchFlightsResponse
			if protojson.Unmarshal(payload, &cached) == nil {
				return &cached, nil
			}
		}
	}

	rows, err := s.repo.Search(origin, destination, date)
	if err != nil {
		return nil, status.Error(codes.Internal, "search flights failed")
	}
	result := &flightv1.SearchFlightsResponse{}
	for i := range rows {
		result.Flights = append(result.Flights, toProto(&rows[i]))
	}

	if s.cache != nil {
		if payload, mErr := protojson.Marshal(result); mErr == nil {
			s.cache.Set(ctx, sKey, payload)
		}
	}
	return result, nil
}

// GetFlight: Cache-Aside
func (s *Server) GetFlight(ctx context.Context, req *flightv1.GetFlightRequest) (*flightv1.GetFlightResponse, error) {
	if req.GetFlightId() == "" {
		return nil, status.Error(codes.InvalidArgument, "flight_id is required")
	}
	fKey := "flight:" + req.GetFlightId()

	if s.cache != nil {
		if payload, ok := s.cache.Get(ctx, fKey); ok {
			var cached flightv1.Flight
			if protojson.Unmarshal(payload, &cached) == nil {
				return &flightv1.GetFlightResponse{Flight: &cached}, nil
			}
		}
	}

	f, err := s.repo.GetByID(req.GetFlightId())
	switch {
	case errors.Is(err, ErrNotFound):
		return nil, status.Errorf(codes.NotFound, "flight %s not found", req.GetFlightId())
	case err != nil:
		return nil, status.Error(codes.Internal, "get flight failed")
	}

	if s.cache != nil {
		if payload, mErr := protojson.Marshal(toProto(f)); mErr == nil {
			s.cache.Set(ctx, fKey, payload)
		}
	}
	return &flightv1.GetFlightResponse{Flight: toProto(f)}, nil
}

// после успешной транзакции - инвалидация кеша
func (s *Server) ReserveSeats(ctx context.Context, req *flightv1.ReserveSeatsRequest) (*flightv1.ReserveSeatsResponse, error) {
	reservationID, err := s.repo.ReserveSeats(req.GetFlightId(), req.GetBookingId(), req.GetSeatCount())
	switch {
	case errors.Is(err, ErrInvalidSeatCount):
		return nil, status.Error(codes.InvalidArgument, "seat_count must be positive")
	case errors.Is(err, ErrNotFound):
		return nil, status.Errorf(codes.NotFound, "flight %s not found", req.GetFlightId())
	case errors.Is(err, ErrNotScheduled):
		return nil, status.Errorf(codes.FailedPrecondition, "flight %s is not scheduled", req.GetFlightId())
	case errors.Is(err, ErrNotEnoughSeats):
		return nil, status.Errorf(codes.ResourceExhausted, "not enough seats on flight %s", req.GetFlightId())
	case errors.Is(err, ErrAlreadyReserved):
		return nil, status.Errorf(codes.AlreadyExists, "active reservation already exists for booking %s", req.GetBookingId())
	case err != nil:
		return nil, status.Error(codes.Internal, "reserve seats failed")
	}

	if s.cache != nil {
		s.cache.InvalidateFlight(ctx, req.GetFlightId())
	}
	return &flightv1.ReserveSeatsResponse{ReservationId: reservationID}, nil
}

// после успешной транзакции - инвалидация кеша
func (s *Server) ReleaseReservation(ctx context.Context, req *flightv1.ReleaseReservationRequest) (*flightv1.ReleaseReservationResponse, error) {
	reservationID, flightID, err := s.repo.ReleaseReservation(req.GetBookingId())
	switch {
	case errors.Is(err, ErrNotFound):
		return nil, status.Errorf(codes.NotFound, "no active reservation for booking %s", req.GetBookingId())
	case err != nil:
		return nil, status.Error(codes.Internal, "release reservation failed")
	}

	if s.cache != nil {
		s.cache.InvalidateFlight(ctx, flightID)
	}
	return &flightv1.ReleaseReservationResponse{ReservationId: reservationID}, nil
}
