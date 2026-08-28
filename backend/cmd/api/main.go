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

	conn, err := db.Connect()
	if err != nil {
		log.Fatal(err)
	}
	eventRepo := repository.NewEventRepository(conn)
	eventService := service.NewEventService(eventRepo)
	eventHandler := handler.NewEventHandler(eventService)

	r := gin.Default()
	r.GET("/", handler.Healthcheck)

	v1 := r.Group("/api/v1")
	{ // hier routen registrieren
		v1.GET("/", handler.Healthcheck)

	}

	events := v1.Group("/events")
	{
		events.PUT("/:id", eventHandler.UpdateEventHandler)
	}

	err = r.Run(fmt.Sprintf(":%d", *port))
	if err != nil {
		log.Fatal(err)
	}
}
