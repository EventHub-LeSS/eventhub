package main

import (
	"backend/internal/db"
	"backend/internal/handler"

	//"backend/internal/repository"
	//"backend/internal/service"
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

	db, db_err := db.Connect()
	if db_err != nil {
		log.Fatal(db_err)
	}

	_ = db

	//eventRepo := repository.NewEventRepository(conn)
	//eventService := service.NewEventService(eventRepo)

	r := gin.Default()
	r.GET("/", handler.Healthcheck)

	v1 := r.Group("/api/v1")
	{ // hier routen registrieren
		v1.GET("/", handler.Healthcheck)

	}

	err := r.Run(fmt.Sprintf(":%d", *port))
	if err != nil {
		log.Fatal(err)
		return
	}
}
