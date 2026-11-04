package bookings

import (
	"context"
	flightv1 "flight-booking/gen/flight/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
	"log"
	"os"
	"strconv"
	"time"
)

// gRPC-клиент к Flight Service с API-ключом в metadata и retry
type FlightClient struct {
	conn        *grpc.ClientConn
	api         flightv1.FlightServiceClient
	apiKey      string
	maxAttempts int
	baseDelay   time.Duration
}

func NewFlightClient(addr, apiKey string) (*FlightClient, error) {
	conn, err := grpc.NewClient(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, err
	}
	maxAttempts := 3
	if raw := os.Getenv("RETRY_MAX_ATTEMPTS"); raw != "" {
		if n, pErr := strconv.Atoi(raw); pErr == nil && n > 0 {
			maxAttempts = n
		}
	}
	baseDelay := 100 * time.Millisecond
	if raw := os.Getenv("RETRY_BASE_DELAY_MS"); raw != "" {
		if n, pErr := strconv.Atoi(raw); pErr == nil && n > 0 {
			baseDelay = time.Duration(n) * time.Millisecond
		}
	}
	return &FlightClient{
		conn:        conn,
		api:         flightv1.NewFlightServiceClient(conn),
		apiKey:      apiKey,
		maxAttempts: maxAttempts,
		baseDelay:   baseDelay,
	}, nil
}

func (c *FlightClient) Close() error {
	return c.conn.Close()
}

// retry только для UNAVAILABLE и DEADLINE_EXCEEDED
func retryable(err error) bool {
	code := status.Code(err)
	return code == codes.Unavailable || code == codes.DeadlineExceeded
}

// вызов с retry: max N попыток, exponential backoff (100ms, 200ms, 400ms...)
func (c *FlightClient) do(ctx context.Context, name string, call func(ctx context.Context) error) error {
	for attempt := 1; ; attempt++ {
		callCtx := metadata.AppendToOutgoingContext(ctx, "x-api-key", c.apiKey)
		callCtx, cancel := context.WithTimeout(callCtx, 5*time.Second)
		err := call(callCtx)
		cancel()
		if err == nil || attempt >= c.maxAttempts || !retryable(err) {
			return err
		}
		delay := c.baseDelay << (attempt - 1)
		log.Printf("flight grpc call %s failed (attempt %d/%d): %v, retrying in %s",
			name, attempt, c.maxAttempts, err, delay)
		time.Sleep(delay)
	}
}

func (c *FlightClient) SearchFlights(ctx context.Context, origin, destination string, date *time.Time) (*flightv1.SearchFlightsResponse, error) {
	req := &flightv1.SearchFlightsRequest{Origin: origin, Destination: destination}
	if date != nil {
		req.DepartureDate = timestamppb.New(*date)
	}
	var resp *flightv1.SearchFlightsResponse
	err := c.do(ctx, "SearchFlights", func(callCtx context.Context) (callErr error) {
		resp, callErr = c.api.SearchFlights(callCtx, req)
		return
	})
	return resp, err
}

func (c *FlightClient) GetFlight(ctx context.Context, flightID string) (*flightv1.GetFlightResponse, error) {
	var resp *flightv1.GetFlightResponse
	err := c.do(ctx, "GetFlight", func(callCtx context.Context) (callErr error) {
		resp, callErr = c.api.GetFlight(callCtx, &flightv1.GetFlightRequest{FlightId: flightID})
		return
	})
	return resp, err
}

func (c *FlightClient) ReserveSeats(ctx context.Context, flightID, bookingID string, seatCount int32) (*flightv1.ReserveSeatsResponse, error) {
	var resp *flightv1.ReserveSeatsResponse
	err := c.do(ctx, "ReserveSeats", func(callCtx context.Context) (callErr error) {
		resp, callErr = c.api.ReserveSeats(callCtx, &flightv1.ReserveSeatsRequest{
			FlightId: flightID, BookingId: bookingID, SeatCount: seatCount,
		})
		return
	})
	return resp, err
}

func (c *FlightClient) ReleaseReservation(ctx context.Context, bookingID string) (*flightv1.ReleaseReservationResponse, error) {
	var resp *flightv1.ReleaseReservationResponse
	err := c.do(ctx, "ReleaseReservation", func(callCtx context.Context) (callErr error) {
		resp, callErr = c.api.ReleaseReservation(callCtx, &flightv1.ReleaseReservationRequest{BookingId: bookingID})
		return
	})
	return resp, err
}
