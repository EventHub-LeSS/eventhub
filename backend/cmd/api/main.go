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
	"log/slog"
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

	// Structured logs for audit events (EVENTHUB-188); plain text like the rest of the app.
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stdout, nil)))

	port := flag.Int("p", 8080, "port to listen on")
	flag.Parse()

	db, dbErr := db.Connect()
	if dbErr != nil {
		log.Fatal(dbErr)
	}

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
	eventRepo := repository.NewEventRepository(db)
	bookingRepo := repository.NewBookingRepository(db)

	// Initialize Services
	eventService := service.NewEventService(eventRepo, orgRepo)
	bookingService := service.NewBookingService(bookingRepo, service.DefaultReservationTTL)

	// Initialize Handlers
	orgHandler := handler.NewOrganizationHandler(keycloakService, orgRepo, userRepo)
	eventHandler := handler.NewEventHandler(eventService, keycloakService)

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

		// events
		events := protected.Group("/events")
		{
			events.PUT("/:id", eventHandler.UpdateEventHandler)
			events.GET("/self", eventHandler.ListOwnEventsHandler)
			events.POST("/:id/publish", eventHandler.PublishEventHandler)
			events.POST("/:id/withdraw", eventHandler.WithdrawEventHandler)
			events.GET("/:eventId/sold-tickets", eventHandler.GetSoldTicketsHandler)
			events.GET("/:eventId/available-seats", eventHandler.GetAvailableSeatsHandler)
		}

		// bookings
		protected.POST("/bookings", handler.CreateBookingHandler(bookingService, userRepo))
	}
	orgs := v1.Group("/organizations")
	orgs.Use(authenticator.Middleware(), middleware.RequireGlobalRole(middleware.RoleAdmin))
	{
		orgs.POST("/", orgHandler.CreateOrganization)
	}

	// EVENTHUB-188: organization admins configure the roles of their members. The handler checks
	// the org_admin role itself after resolving the organization, which the path may address by
	// ID or alias while tokens only carry the alias.
	orgRoles := v1.Group("/organizations")
	orgRoles.Use(authenticator.Middleware())
	{
		orgRoles.PUT("/:organizationID/members/:username/roles", orgHandler.ConfigureMemberRoles)
	}

	err = r.Run(fmt.Sprintf(":%d", *port))
	if err != nil {
		log.Fatal(err)
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
