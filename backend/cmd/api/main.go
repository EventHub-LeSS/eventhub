package main

import (
	"backend/internal/db"
	"backend/internal/handler"
	"backend/internal/repository"
	"backend/internal/service"
	"flag"
	"fmt"
	"log"

	"github.com/gin-gonic/gin"
	"github.com/joho/godotenv"
)

func main() {
	godotenv.Load()

	port := flag.Int("p", 8080, "port to listen on")
	flag.Parse()

	conn := db.Connect()
	eventRepo := repository.NewEventRepository(conn)
	eventService := service.NewEventService(eventRepo)
	handler.SetEventService(eventService)

	r := gin.Default()
	r.GET("/", handler.Healthcheck)

	v1 := r.Group("/api/v1")
	{
		v1.GET("/", handler.Healthcheck)

		v1.GET("/events/:eventId/sold-tickets", handler.GetSoldTicketsHandler)
		v1.GET("/events/:eventId/available-seats", handler.GetAvailableSeatsHandler)
	}

	err := r.Run(fmt.Sprintf(":%d", *port))
	if err != nil {
		log.Fatal(err)
	}
}
