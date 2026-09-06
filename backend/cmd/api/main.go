package main

import (
	"backend/internal/db"
	"backend/internal/handler"
	"backend/internal/middleware"
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

	authConfig, err := middleware.LoadAuthenticationConfig()
	if err != nil {
		log.Fatal(err)
	}
	authenticator, err := middleware.NewAuthenticator(authConfig)
	if err != nil {
		log.Fatal(err)
	}

	eventRepo := repository.NewEventRepository(conn)
	orgRepo := repository.NewOrganizationRepository(conn)
	eventService := service.NewEventService(eventRepo, orgRepo)
	eventHandler := handler.NewEventHandler(eventService)

	r := gin.Default()
	r.GET("/", handler.Healthcheck)

	v1 := r.Group("/api/v1")
	{ // hier routen registrieren
		v1.GET("/", handler.Healthcheck)

		protected := v1.Group("")
		protected.Use(authenticator.Middleware())
		protected.GET("/users/me", handler.CurrentUser)

		events := protected.Group("/events")
		{
			events.PUT("/:id", eventHandler.UpdateEventHandler)
		}
	}

	err = r.Run(fmt.Sprintf(":%d", *port))
	if err != nil {
		log.Fatal(err)
	}
}
