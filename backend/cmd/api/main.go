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
	"os"

	_ "backend/docs"

	"github.com/gin-gonic/gin"
	"github.com/joho/godotenv"
	swagFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"
)

// @title           EventHub API
// @version         1.0
// @description     REST API for the EventHub platform
// @BasePath        /api/v1
// @securityDefinitions.apikey BearerAuth
// @in header
// @name Authorization
// @description Enter "Bearer {token}" where {token} is a Keycloak access token
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
		Host:             os.Getenv("KEYCLOAK_HOST"),
		AdminRealm:       os.Getenv("KEYCLOAK_ADMIN_REALM"),
		UserRealm:        firstNonEmpty(os.Getenv("KEYCLOAK_USER_REALM"), os.Getenv("KEYCLOAK_REALM")),
		ClientID:         os.Getenv("KEYCLOAK_CLIENT_ID"),
		ClientSecret:     os.Getenv("KEYCLOAK_CLIENT_SECRET"),
		FrontendClientID: firstNonEmpty(os.Getenv("KEYCLOAK_FRONTEND_CLIENT_ID"), "frontend"),
	}
	keycloakService := service.NewKeycloakService(keycloakCfg)

	// Initialize Repositories
	orgRepo := repository.NewOrganizationRepository(db)
	userRepo := repository.NewUserRepository(db)

	// Initialize Handlers
	orgHandler := handler.NewOrganizationHandler(keycloakService, orgRepo, userRepo)

	r := gin.Default()
	r.GET("/", handler.Healthcheck)
	r.GET("/swagger/*any", ginSwagger.WrapHandler(swagFiles.Handler))

	v1 := r.Group("/api/v1")
	{ // hier routen registrieren
		v1.GET("/", handler.Healthcheck)

		// DEBUG routes — disabled in production
		if os.Getenv("DEBUG_ENABLED") == "true" {
			debugHandler := handler.NewDebugHandler(keycloakService)
			v1.POST("/debug/token", debugHandler.GetToken)
			log.Println("WARNING: debug routes enabled (DEBUG_ENABLED=true) — do not use in production")
		}

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

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}
