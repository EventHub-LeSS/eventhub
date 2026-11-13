package main

import (
	"backend/internal/audit"
	"backend/internal/db"
	"backend/internal/handler"
	"backend/internal/middleware"
	"backend/internal/repository"
	"backend/internal/seed"
	"backend/internal/service"
	"context"
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"log"
	"log/slog"
	"os"
	"time"

	"gorm.io/gorm"

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
	// A missing .env is normal outside local development, where real env vars
	// are set. Anything else means the file is there but could not be read.
	if err := godotenv.Load(); err != nil && !errors.Is(err, fs.ErrNotExist) {
		log.Printf("ignoring unreadable .env: %v", err)
	}

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
		AdminRealm:       os.Getenv("KEYCLOAK_REALM"),
		UserRealm:        firstNonEmpty(os.Getenv("KEYCLOAK_USER_REALM"), os.Getenv("KEYCLOAK_REALM")),
		ClientID:         os.Getenv("KEYCLOAK_CLIENT_ID"),
		ClientSecret:     os.Getenv("KEYCLOAK_CLIENT_SECRET"),
		FrontendClientID: firstNonEmpty(os.Getenv("KEYCLOAK_FRONTEND_CLIENT_ID"), "frontend"),
	}
	keycloakService := service.NewKeycloakService(keycloakCfg).WithAuditLog(repository.NewAuditLogRepository(db))

	// Initialize Repositories
	orgRepo := repository.NewOrganizationRepository(db)
	userRepo := repository.NewUserRepository(db)
	eventRepo := repository.NewEventRepository(db)
	bookingRepo := repository.NewBookingRepository(db)
	notificationRepo := repository.NewNotificationRepository(db)
	paymentRepo := repository.NewPaymentRepository(db)
	cartRepo := repository.NewCartRepository(db)
	tx := repository.NewTransactor(db)

	// Initialize Services
	eventService := service.NewEventService(eventRepo, orgRepo, tx)
	locationService := service.NewLocationService(orgRepo, repository.NewLocationRepository(db))
	bookingService := service.NewBookingService(tx, bookingRepo, paymentRepo)
	paymentService := service.NewPaymentService(tx, bookingRepo, paymentRepo)
	cartService := service.NewCartService(tx, bookingRepo, cartRepo, paymentRepo, service.DefaultReservationTTL)
	ownBookingService := service.NewOwnBookingService(repository.NewOwnBookingRepository(db))
	// EVENTHUB-235: fail pending payments of expired reservations, so no payment stays open.
	go paymentService.RunPaymentCleanup(context.Background(), service.PaymentCleanupInterval)

	// Converge the database with Keycloak at startup: the realm-sync on the
	// Keycloak container applies realm-file changes to Keycloak only. Mirror
	// those organizations and users into the database here, so a restart is
	// enough for realm.json changes to reach the API. Retries while Keycloak
	// boots; a failure never prevents the API from serving.
	go runStartupSync(db, keycloakService, keycloakCfg.UserRealm)
	recommendationsService := service.NewRecommendationsService(repository.NewRecommendationRepository(db))
	ratingService := service.NewRatingService(repository.NewRatingRepository(db), eventRepo, orgRepo)
	moderationService := service.NewModerationService(tx, repository.NewModerationRepository(db))

	notificationService := service.NewNotificationService(notificationRepo, tx)

	// Initialize Handlers
	orgService := service.NewOrganizationService(keycloakService, orgRepo, userRepo)
	orgHandler := handler.NewOrganizationHandler(keycloakService, orgService, orgRepo, userRepo)
	// EVENTHUB-294: users apply via contact form to become an organizer, admins review.
	applicationHandler := handler.NewOrganizerApplicationHandler(service.NewOrganizerApplicationService(tx, repository.NewOrganizerApplicationRepository(db), userRepo, orgRepo, orgService), userRepo)
	auditLogHandler := handler.NewAuditLogHandler(keycloakService, repository.NewAuditLogRepository(db))
	userAdminHandler := handler.NewUserAdminHandler(keycloakService, userRepo)
	orgAdminHandler := handler.NewOrganizationAdminHandler(orgRepo, service.NewOrganizationApprovalService(tx))
	eventHandler := handler.NewEventHandler(eventService)
	locationHandler := handler.NewLocationHandler(locationService)
	notificationHandler := handler.NewNotificationHandler(notificationService, userRepo)
	paymentHandler := handler.NewPaymentHandler(paymentService, userRepo)
	cartHandler := handler.NewCartHandler(cartService, userRepo)
	ownBookingHandler := handler.NewOwnBookingHandler(ownBookingService, userRepo)
	recommendationsHandler := handler.NewRecommendationsHandler(recommendationsService, userRepo)
	ratingHandler := handler.NewRatingHandler(ratingService, userRepo)
	moderationHandler := handler.NewModerationHandler(moderationService)

	r := gin.Default()
	r.GET("/", handler.Healthcheck)
	r.GET("/swagger/*any", ginSwagger.WrapHandler(swagFiles.Handler))

	v1 := r.Group("/api/v1")
	{ // hier routen registrieren
		v1.GET("/", handler.Healthcheck)
		v1.GET("/events", eventHandler.ListPublishedEventsHandler)
		v1.GET("/events/:eventId", eventHandler.GetPublishedEventDetailsHandler)
		v1.GET("/events/:eventId/image", eventHandler.GetEventImageHandler)
		v1.GET("/events/:eventId/rating-summary", ratingHandler.GetEventRatingSummaryHandler)
		v1.GET("/events/:eventId/ratings", ratingHandler.ListEventRatingsHandler)
		v1.GET("/organizations/:organizationID/rating-summary", ratingHandler.GetOrganizerRatingSummaryHandler)

		// DEBUG routes — disabled in production
		if os.Getenv("DEBUG_ENABLED") == "true" {
			debugHandler := handler.NewDebugHandler(keycloakService)
			v1.POST("/debug/token", debugHandler.GetToken)
			log.Println("WARNING: debug routes enabled (DEBUG_ENABLED=true) — do not use in production")
		}

		protected := v1.Group("")
		protected.Use(authenticator.Middleware(), middleware.ProvisionAPIUser(userRepo, keycloakService, keycloakService))
		protected.GET("/users/me", handler.CurrentUser)
		protected.GET("/notifications", notificationHandler.ListNotificationsHandler)
		protected.PATCH("/notifications/:notificationId/read", notificationHandler.MarkNotificationReadHandler)
		protected.POST("/bookings/:bookingId/reminder", notificationHandler.SimulateReminderHandler)

		protected.GET("/recommendations", recommendationsHandler.GetEventRecommendations)

		// EVENTHUB-294: contact form to become an organizer
		protected.POST("/organizer-applications", applicationHandler.SubmitApplicationHandler)

		// events
		events := protected.Group("/events")
		{
			events.POST("/:id/ratings", ratingHandler.CreateEventRatingHandler)
			events.POST("/draft", middleware.Audit(audit.EventCreated), eventHandler.SaveEventAsDraftHandler)
			events.GET("/self", eventHandler.ListOwnEventsHandler)
			events.GET("/org/:id", eventHandler.ListOrganizationEventsHandler)
			events.PUT("/:id", middleware.Audit(audit.EventUpdated), eventHandler.UpdateEventHandler)
			events.PUT("/:id/image", middleware.Audit(audit.EventImageChanged), eventHandler.PutEventImageHandler)
			events.POST("/:id/publish", middleware.Audit(audit.EventPublished), eventHandler.PublishEventHandler)
			events.POST("/:id/restore-draft", middleware.Audit(audit.EventRestoredAsDraft), eventHandler.RestoreEventAsDraftHandler)
			events.POST("/:id/withdraw", middleware.Audit(audit.EventCancelled), eventHandler.WithdrawEventHandler)
			events.DELETE("/:id", middleware.Audit(audit.EventDeleted), eventHandler.DeleteEventHandler)
			events.GET("/:eventId/sold-tickets", eventHandler.GetSoldTicketsHandler)
			events.GET("/:eventId/available-seats", eventHandler.GetAvailableSeatsHandler)
			events.GET("/:eventId/utilization-percentage", eventHandler.GetUtilizationPercentageHandler)
		}
		protected.GET("/locations", locationHandler.ListLocationsHandler)
		adminUsers := v1.Group("/admin/users")
		adminUsers.Use(authenticator.Middleware(), middleware.RequireGlobalRole(middleware.RoleAdmin))
		{
			adminUsers.GET("", userAdminHandler.ListUsers)
			adminUsers.GET("/:userID/roles", userAdminHandler.GetUserRoles)
			adminUsers.PUT("/:userID/roles", userAdminHandler.UpdateUserRoles)
		}

		// EVENTHUB-294: global admins review organizer applications of others; approval creates the
		// organization with the service account. EVENTHUB-304: org admins get no access, the
		// applications belong to organizations they are not a member of.
		adminApplications := v1.Group("/admin/organizer-applications")
		adminApplications.Use(authenticator.Middleware(), middleware.RequireGlobalRole(middleware.RoleAdmin))
		{
			adminApplications.GET("", applicationHandler.ListApplicationsHandler)
			adminApplications.GET("/:applicationId", applicationHandler.GetApplicationHandler)
			adminApplications.POST("/:applicationId/approve", middleware.Audit(audit.OrganizerApplicationApproved), applicationHandler.ApproveApplicationHandler)
			adminApplications.POST("/:applicationId/reject", middleware.Audit(audit.OrganizerApplicationRejected), applicationHandler.RejectApplicationHandler)
		}

		// EVENTHUB-293: admins review and approve organizations requested via the contact form
		adminOrgs := v1.Group("/admin/organizations")
		adminOrgs.Use(authenticator.Middleware(), middleware.RequireGlobalRole(middleware.RoleAdmin))
		{
			adminOrgs.GET("/pending", orgAdminHandler.ListPendingOrganizations)
			adminOrgs.PATCH("/:organizationID/approve", middleware.Audit(audit.OrganizationApproved), orgAdminHandler.ApproveOrganization)
		}

		// Platform moderation of user content by global admins and moderators.
		moderation := v1.Group("/moderation")
		moderation.Use(authenticator.Middleware(), middleware.RequireGlobalRole(middleware.RoleAdmin, middleware.RoleModerator))
		{
			moderation.GET("/ratings", moderationHandler.ListRatingsHandler)
			moderation.PUT("/ratings/:ratingId/visibility", middleware.Audit(audit.RatingVisibilityChanged), moderationHandler.SetRatingVisibilityHandler)
			moderation.GET("/events", moderationHandler.ListEventsHandler)
			moderation.GET("/events/:eventId", moderationHandler.GetEventHandler)
			moderation.PATCH("/events/:eventId/content", middleware.Audit(audit.EventModerated), moderationHandler.EditEventContentHandler)
			moderation.POST("/events/:eventId/withdraw", middleware.Audit(audit.EventModerated), moderationHandler.WithdrawEventHandler)
		}

		// bookings
		protected.POST("/bookings/:bookingId/cancel", handler.CancelBooking(bookingService, userRepo))

		// 282: Warenkorb — Reservierungen ohne Zahlung, ein Checkout zahlt alle zusammen
		cart := protected.Group("/cart")
		{
			cart.GET("", cartHandler.GetCartHandler)
			cart.DELETE("", cartHandler.ClearCartHandler)
			cart.POST("/items", cartHandler.AddCartItemHandler)
			cart.PATCH("/items/:bookingId", cartHandler.UpdateCartItemHandler)
			cart.DELETE("/items/:bookingId", cartHandler.RemoveCartItemHandler)
			cart.POST("/checkout", cartHandler.CheckoutHandler)
		}

		// EVENTHUB-235: simulated payment process of a checkout, no real payment provider
		payments := protected.Group("/payments")
		{
			payments.GET("/:paymentId", paymentHandler.GetPaymentHandler)
			payments.POST("/:paymentId/simulate", paymentHandler.SimulatePaymentHandler)
		}

		// EVENTHUB-236: Meine Tickets und Buchungshistorie, always the bookings of the token user
		me := protected.Group("/me")
		{
			me.GET("/tickets", ownBookingHandler.ListOwnTicketsHandler)
			me.GET("/bookings", ownBookingHandler.ListOwnBookingsHandler)
			me.GET("/bookings/:bookingId", ownBookingHandler.GetOwnBookingHandler)
			me.GET("/organizer-applications", applicationHandler.ListOwnApplicationsHandler)
		}
	}
	// Any signed-in user may request an organization. The row is inactive until an admin approves it.
	orgs := v1.Group("/organizations")
	orgs.Use(authenticator.Middleware(), middleware.ProvisionAPIUser(userRepo, keycloakService, keycloakService))
	{
		orgs.POST("/", orgHandler.CreateOrganization)
	}

	// EVENTHUB-188: organization admins configure the roles of their members. The handler checks
	// the org_admin role itself after resolving the organization, which the path may address by
	// ID or alias while tokens only carry the alias.
	orgRoles := v1.Group("/organizations")
	orgRoles.Use(authenticator.Middleware())
	{
		// Organization details are read by any member and changed by org admins; the handler
		// authorizes against the resolved organization like the role endpoint above.
		orgRoles.GET("/:organizationID", orgHandler.GetOrganization)
		orgRoles.PUT("/:organizationID", middleware.Audit(audit.OrganizationUpdated), orgHandler.UpdateOrganization)
		orgRoles.GET("/:organizationID/members", orgHandler.ListOrganizationMembers)
		orgRoles.POST("/:organizationID/members/invitations", orgHandler.InviteOrganizationMember)
		orgRoles.GET("/:organizationID/dashboard", eventHandler.GetSalesDashboardHandler)
		orgRoles.PUT("/:organizationID/members/:username/roles", middleware.Audit(audit.OrganizationMemberRolesChange), orgHandler.ConfigureMemberRoles)
		// Organization admins read the audit log of their organization; the handler authorizes
		// against the resolved organization like the role endpoint above.
		orgRoles.GET("/:organizationID/audit-logs", auditLogHandler.ListAuditLogs)
	}

	err = r.Run(fmt.Sprintf(":%d", *port))
	if err != nil {
		log.Fatal(err)
		return
	}
}

// runStartupSync mirrors Keycloak organizations and users into the database
// once at API startup (see the goroutine in main). It retries the Keycloak
// login while the Keycloak container is still booting and gives up with a log
// line instead of taking the API down.
func runStartupSync(database *gorm.DB, keycloakService *service.KeycloakService, realm string) {
	ctx := context.Background()
	seeder := seed.NewSeeder(database, keycloakService)

	syncOnce := func() bool {
		token, err := keycloakService.ServiceAccountToken(ctx)
		if err != nil {
			log.Printf("startup sync: keycloak login failed: %v", err)
			return false
		}
		if err := seeder.SyncDB(ctx, realm, token); err != nil {
			log.Printf("startup sync: failed (run 'docker compose up api-seed' manually): %v", err)
			return false
		}
		return true
	}

	// Retry while the Keycloak container boots (up to 2 minutes in 5s steps).
	for attempt := 1; attempt <= 24; attempt++ {
		if syncOnce() {
			break
		}
		time.Sleep(5 * time.Second)
	}

	// The Keycloak entrypoint applies the realm file a few seconds AFTER the
	// server answers (realm-sync runs as a background job). A first sync that
	// wins that race has converged against the pre-sync state, so re-check
	// once after a short settle delay. Both passes are no-ops when the
	// database already matches Keycloak.
	time.Sleep(15 * time.Second)
	if syncOnce() {
		log.Println("startup sync: database converged with Keycloak")
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
