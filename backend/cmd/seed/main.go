package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	"backend/internal/db"
	"backend/internal/seed"
	"backend/internal/service"

	"github.com/joho/godotenv"
)

func main() {
	godotenv.Load()

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	database, err := db.Connect()
	if err != nil {
		log.Fatalf("connect database: %v", err)
	}

	keycloakCfg := service.KeycloakClientConfig{
		Host:             os.Getenv("KEYCLOAK_HOST"),
		UserRealm:        firstNonEmpty(os.Getenv("KEYCLOAK_USER_REALM"), os.Getenv("KEYCLOAK_REALM")),
		FrontendClientID: firstNonEmpty(os.Getenv("KEYCLOAK_FRONTEND_CLIENT_ID"), "frontend"),
	}
	if keycloakCfg.Host == "" || keycloakCfg.UserRealm == "" {
		log.Fatal("KEYCLOAK_HOST and KEYCLOAK_REALM are required")
	}
	keycloakService := service.NewKeycloakService(keycloakCfg)

	seedUsername := firstNonEmpty(os.Getenv("SEED_USERNAME"), "großmeister_finn")
	seedPassword := firstNonEmpty(os.Getenv("SEED_PASSWORD"), "password")

	log.Printf("logging in as %q to realm %q", seedUsername, keycloakCfg.UserRealm)
	var accessToken string
	maxRetries := 10
	for i := 0; i < maxRetries; i++ {
		jwt, err := keycloakService.LoginUser(ctx, seedUsername, seedPassword)
		if err == nil {
			accessToken = jwt.AccessToken
			break
		}
		log.Printf("  login attempt %d/%d failed: %v", i+1, maxRetries, err)
		if i < maxRetries-1 {
			time.Sleep(5 * time.Second)
		}
	}
	if accessToken == "" {
		log.Fatal("failed to login to Keycloak after retries")
	}

	seeder := seed.NewSeeder(database, keycloakService)
	if err := seeder.SeedAll(ctx, keycloakCfg.UserRealm, accessToken); err != nil {
		log.Fatalf("seed failed: %v", err)
	}

	fmt.Println()
	log.Println("Seed complete: mock data created in Keycloak and database")
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}
