package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"backend/internal/db"
	"backend/internal/repository"
	"backend/internal/seed"
	"backend/internal/service"

	"github.com/joho/godotenv"
	"gorm.io/gorm"
)

func main() {
	// A missing .env is normal outside local development, where real env vars
	// are set. Anything else means the file is there but could not be read.
	if err := godotenv.Load(); err != nil && !errors.Is(err, fs.ErrNotExist) {
		log.Printf("ignoring unreadable .env: %v", err)
	}

	// ./seed is the single seed entry point for every deployment: which content it
	// seeds is baked into the image as ENV DEPLOYMENT (build arg of the CI pipelines),
	// so the deployment stacks all run the same command without any configuration —
	// the same pattern as the per-deployment realm file. Local dev composes may
	// override the baked value with an explicit DEPLOYMENT environment variable.
	deployment := os.Getenv("DEPLOYMENT")
	switch deployment {
	case "", "ref", "live":
	default:
		log.Fatalf("unknown DEPLOYMENT %q: expected \"ref\" or \"live\"", deployment)
	}
	if deployment == "" {
		deployment = "ref"
	}
	log.Printf("seeding the %q deployment...", deployment)

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	database, err := db.Connect()
	if err != nil {
		log.Fatalf("connect database: %v", err)
	}

	keycloakCfg := service.KeycloakClientConfig{
		Host:             os.Getenv("KEYCLOAK_HOST"),
		AdminRealm:       firstNonEmpty(os.Getenv("KEYCLOAK_REALM"), os.Getenv("KEYCLOAK_USER_REALM")),
		UserRealm:        firstNonEmpty(os.Getenv("KEYCLOAK_USER_REALM"), os.Getenv("KEYCLOAK_REALM")),
		ClientID:         firstNonEmpty(os.Getenv("KEYCLOAK_CLIENT_ID"), "backend"),
		ClientSecret:     os.Getenv("KEYCLOAK_CLIENT_SECRET"),
		FrontendClientID: firstNonEmpty(os.Getenv("KEYCLOAK_FRONTEND_CLIENT_ID"), "frontend"),
	}
	if keycloakCfg.Host == "" || keycloakCfg.UserRealm == "" {
		log.Fatal("KEYCLOAK_HOST and KEYCLOAK_REALM are required")
	}
	if keycloakCfg.ClientSecret == "" {
		log.Fatalf("KEYCLOAK_CLIENT_SECRET is required: the seeder authenticates as the %q service account via client credentials, not as a realm user", keycloakCfg.ClientID)
	}
	keycloakService := service.NewKeycloakService(keycloakCfg)

	// The seeder only performs Keycloak admin operations, so it authenticates as the
	// backend service account (client credentials) exactly like the API does. A realm
	// user login would additionally depend on a demo account existing in the deployed
	// realm and on direct access grants staying enabled on the frontend client.
	log.Printf("authenticating as service account %q against realm %q at %s", keycloakCfg.ClientID, keycloakCfg.AdminRealm, keycloakCfg.Host)
	accessToken := authenticate(ctx, keycloakService, keycloakCfg)

	fmt.Println()
	switch deployment {
	case "live":
		runLiveSeed(ctx, database, keycloakService, keycloakCfg, accessToken)
	default:
		runMockSeed(ctx, database, keycloakService, keycloakCfg, accessToken)
	}
}

func runLiveSeed(ctx context.Context, database *gorm.DB, keycloakService *service.KeycloakService, keycloakCfg service.KeycloakClientConfig, accessToken string) {
	// Fail fast when the target still carries mock seed data instead of stacking live
	// content on top of demo data (see seed.EnsureNoMockData for the cleanup).
	if err := seed.EnsureNoMockData(ctx, database, keycloakService, keycloakCfg.UserRealm, accessToken); err != nil {
		log.Fatalf("live seed: %v", err)
	}

	liveSeeder := seed.NewLiveSeeder(database, keycloakService, repository.NewTransactor(database), keycloakCfg.UserRealm, accessToken)
	if err := liveSeeder.SeedAll(ctx); err != nil {
		log.Fatalf("live seed failed: %v", err)
	}

	log.Println("Seed complete: live content created in Keycloak and database")
}

func runMockSeed(ctx context.Context, database *gorm.DB, keycloakService *service.KeycloakService, keycloakCfg service.KeycloakClientConfig, accessToken string) {
	// Fail fast when the target already contains live deployment content instead of
	// polluting it with demo data (see seed.EnsureNoLiveData).
	if err := seed.EnsureNoLiveData(ctx, database, keycloakService, keycloakCfg.UserRealm, accessToken); err != nil {
		log.Fatalf("mock seed: %v", err)
	}

	seeder := seed.NewSeeder(database, keycloakService)
	if err := seeder.SeedAll(ctx, keycloakCfg.UserRealm, accessToken); err != nil {
		log.Fatalf("seed failed: %v", err)
	}

	log.Println("Seed complete: mock data created in Keycloak and database")
}

// authenticate obtains the service account token, retrying while Keycloak is still
// coming up, and logs a raw diagnostics probe after every failed attempt.
func authenticate(ctx context.Context, keycloakService *service.KeycloakService, keycloakCfg service.KeycloakClientConfig) string {
	maxRetries := 10
	for i := 0; i < maxRetries; i++ {
		token, err := keycloakService.ServiceAccountToken(ctx)
		if err == nil {
			return token
		}
		log.Printf("  login attempt %d/%d failed: %v", i+1, maxRetries, err)
		if i < maxRetries-1 {
			diagnoseTokenEndpoint(ctx, keycloakCfg)
			time.Sleep(5 * time.Second)
		}
	}
	log.Fatal("failed to authenticate against Keycloak after retries")
	return ""
}

// diagnoseTokenEndpoint repeats the service-account token request as a plain HTTP call
// and logs the raw response, so a denial is visible even when the client library only
// reports a status code. Distinguishes Keycloak's own rejections (which answer with a
// JSON error body) from answers by anything in between, such as a reverse proxy.
func diagnoseTokenEndpoint(ctx context.Context, cfg service.KeycloakClientConfig) {
	endpoint := strings.TrimRight(cfg.Host, "/") + "/realms/" + url.PathEscape(cfg.AdminRealm) + "/protocol/openid-connect/token"
	form := url.Values{
		"grant_type":    {"client_credentials"},
		"client_id":     {cfg.ClientID},
		"client_secret": {cfg.ClientSecret},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		log.Printf("  diagnostic: token endpoint %s is unreachable: %v", endpoint, err)
		return
	}
	defer resp.Body.Close()
	bodyBytes, err := io.ReadAll(io.LimitReader(resp.Body, 4096))
	body := "<unreadable: " + fmt.Sprint(err) + ">"
	if err == nil {
		body = string(bodyBytes)
	}
	log.Printf("  diagnostic: token endpoint %s answered HTTP %d (%s) with body: %.500s",
		endpoint, resp.StatusCode, strings.TrimSpace(resp.Header.Get("Content-Type")), strings.TrimSpace(body))

	switch {
	case resp.StatusCode == http.StatusForbidden && strings.TrimSpace(body) == "":
		log.Printf("  diagnostic: a 403 without a Keycloak error body usually comes from a proxy in front of Keycloak; check that %s reaches the Keycloak server directly", cfg.Host)
	case strings.Contains(body, "HTTPS required"):
		log.Printf("  diagnostic: the realm requires HTTPS; set Realm Settings > SSL Policy back to none, or make KEYCLOAK_HOST use https")
	case strings.Contains(body, "Realm not enabled"):
		log.Printf("  diagnostic: the realm %q is disabled in Keycloak", cfg.AdminRealm)
	case strings.Contains(body, "invalid_client"):
		log.Printf("  diagnostic: Keycloak rejected the client; verify KEYCLOAK_CLIENT_SECRET matches the secret of the %q client in realm %q", cfg.ClientID, cfg.AdminRealm)
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
