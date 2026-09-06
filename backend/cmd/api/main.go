package main

import (
	"backend/internal/db"
	"backend/internal/handler"
	"backend/internal/middleware"
	"backend/internal/service"
	"flag"
	"fmt"
	"log"
	"os"

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

	authConfig, err := middleware.LoadAuthenticationConfig()
	if err != nil {
		log.Fatal(err)
	}
	authenticator, err := middleware.NewAuthenticator(authConfig)
	if err != nil {
		log.Fatal(err)
	}

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
	{ // hier routen registrieren
		v1.GET("/", handler.Healthcheck)

		protected := v1.Group("")
		protected.Use(authenticator.Middleware())
		protected.GET("/users/me", handler.CurrentUser)
	}
	orgs := v1.Group("/organizations")
	orgs.Use(authenticator.Middleware(), middleware.RequireGlobalRole(middleware.RoleAdmin))
	{
		orgs.POST("/", orgHandler.CreateOrganization)
	}

	err = r.Run(fmt.Sprintf(":%d", *port))
	if err != nil {
		log.Fatal(err)
		return
	}
}
