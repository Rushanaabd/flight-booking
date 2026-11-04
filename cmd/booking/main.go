package main

import (
	"flight-booking/internal/bookings"
	"flight-booking/internal/db"
	"github.com/gin-gonic/gin"
	"log"
	"os"
)

func main() {
	apiKey := os.Getenv("FLIGHT_API_KEY")
	if apiKey == "" {
		log.Fatal("FLIGHT_API_KEY is required")
	}
	addr := os.Getenv("FLIGHT_ADDR")
	if addr == "" {
		addr = "localhost:50051"
	}

	database, err := db.Connect("booking_db")
	if err != nil {
		log.Fatal(err)
	}
	defer database.Close()

	flightClient, err := bookings.NewFlightClient(addr, apiKey)
	if err != nil {
		log.Fatal(err)
	}
	defer flightClient.Close()

	h := bookings.NewHandler(bookings.NewRepo(database), flightClient)

	r := gin.New()
	r.Use(gin.Recovery())
	r.GET("/healthz", func(c *gin.Context) { c.JSON(200, gin.H{"status": "ok"}) })
	bookings.RegisterRoutes(r, h)

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	log.Printf("booking service (rest) listening on :%s", port)
	if err = r.Run(":" + port); err != nil {
		log.Fatal(err)
	}
}
