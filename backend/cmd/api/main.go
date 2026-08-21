package main

import (
	"backend/internal/handler"
	"backend/internal/service"
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/gin-gonic/gin"
)

func main() {
	port := flag.Int("p", 8080, "port to listen on")
	flag.Parse()

	// Initialize Keycloak Service
	keycloakCfg := service.KeycloakClientConfig{
		Host:         os.Getenv("KEYCLOAK_HOST"),
		AdminRealm:   os.Getenv("KEYCLOAK_ADMIN_REALM"),
		UserRealm:    os.Getenv("KEYCLOAK_USER_REALM"),
		ClientID:     os.Getenv("KEYCLOAK_CLIENT_ID"),
		ClientSecret: os.Getenv("KEYCLOAK_CLIENT_SECRET"),
	}
	keycloakService := service.NewKeycloakService(keycloakCfg)

	// Initialize Handlers
	orgHandler := handler.NewOrganizationHandler(keycloakService)

	r := gin.Default()
	r.GET("/", handler.Healthcheck)

	v1 := r.Group("/api/v1")
	{
		v1.GET("/", handler.Healthcheck)
	}
	orgs := v1.Group("/organizations")
	{
		orgs.POST("/", orgHandler.CreateOrganization)
	}

	err := r.Run(fmt.Sprintf(":%d", *port))
	if err != nil {
		log.Fatal(err)
		return
	}
}
