package main

import (
	flightv1 "flight-booking/gen/flight/v1"
	"flight-booking/internal/auth"
	"flight-booking/internal/db"
	"flight-booking/internal/flights"
	"google.golang.org/grpc"
	"log"
	"net"
	"os"
	"strings"
	"time"
)

func main() {
	apiKey := os.Getenv("FLIGHT_API_KEY")
	if apiKey == "" {
		log.Fatal("FLIGHT_API_KEY is required")
	}

	database, err := db.Connect("flight_db")
	if err != nil {
		log.Fatal(err)
	}
	defer database.Close()

	repo := flights.NewRepo(database)

	var cache *flights.Cache
	switch {
	case os.Getenv("REDIS_SENTINEL_ADDRS") != "":
		ttl := 5 * time.Minute
		if raw := os.Getenv("CACHE_TTL"); raw != "" {
			if d, parseErr := time.ParseDuration(raw); parseErr == nil {
				ttl = d
			}
		}
		masterName := os.Getenv("REDIS_MASTER_NAME")
		if masterName == "" {
			masterName = "mymaster"
		}
		addrs := strings.Split(os.Getenv("REDIS_SENTINEL_ADDRS"), ",")
		cache = flights.NewFailoverCache(masterName, addrs, ttl)
		defer cache.Close()
		log.Printf("redis sentinel cache enabled master=%s sentinels=%v ttl=%s", masterName, addrs, ttl)

	case os.Getenv("REDIS_ADDR") != "":
		ttl := 5 * time.Minute
		if raw := os.Getenv("CACHE_TTL"); raw != "" {
			if d, parseErr := time.ParseDuration(raw); parseErr == nil {
				ttl = d
			}
		}
		cache = flights.NewStandaloneCache(os.Getenv("REDIS_ADDR"), ttl)
		defer cache.Close()
		log.Printf("redis standalone cache enabled addr=%s ttl=%s", os.Getenv("REDIS_ADDR"), ttl)

	default:
		log.Println("redis cache disabled (no REDIS_* env)")
	}

	srv := flights.NewServer(repo, cache)

	grpcServer := grpc.NewServer(grpc.ChainUnaryInterceptor(auth.UnaryAPIKey(apiKey)))
	flightv1.RegisterFlightServiceServer(grpcServer, srv)

	port := os.Getenv("GRPC_PORT")
	if port == "" {
		port = "50051"
	}
	lis, err := net.Listen("tcp", ":"+port)
	if err != nil {
		log.Fatal(err)
	}
	log.Printf("flight service (grpc) listening on :%s", port)
	if err = grpcServer.Serve(lis); err != nil {
		log.Fatal(err)
	}
}
