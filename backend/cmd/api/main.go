package main

import (
	"backend/internal/handler"
	"flag"
	"fmt"
	"log"

	"github.com/gin-gonic/gin"
)

func main() {
	port := flag.Int("p", 8080, "port to listen on")
	flag.Parse()

	r := gin.Default()
	r.GET("/", handler.Healthcheck)

	v1 := r.Group("/api/v1")
	{
		v1.GET("/", handler.Healthcheck)
	}

	err := r.Run(fmt.Sprintf(":%d", *port))
	if err != nil {
		log.Fatal(err)
		return
	}
}
