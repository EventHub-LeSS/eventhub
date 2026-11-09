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
	"backend/internal/seed"
	"backend/internal/service"

	"github.com/joho/godotenv"
)

func main() {
	// A missing .env is normal outside local development, where real env vars
	// are set. Anything else means the file is there but could not be read.
	if err := godotenv.Load(); err != nil && !errors.Is(err, fs.ErrNotExist) {
		log.Printf("ignoring unreadable .env: %v", err)
	}

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
	var accessToken string
	maxRetries := 10
	for i := 0; i < maxRetries; i++ {
		token, err := keycloakService.ServiceAccountToken(ctx)
		if err == nil {
			accessToken = token
			break
		}
		log.Printf("  login attempt %d/%d failed: %v", i+1, maxRetries, err)
		if i < maxRetries-1 {
			diagnoseTokenEndpoint(ctx, keycloakCfg)
			time.Sleep(5 * time.Second)
		}
	}
	if accessToken == "" {
		log.Fatal("failed to authenticate against Keycloak after retries")
	}

	seeder := seed.NewSeeder(database, keycloakService)
	if err := seeder.SeedAll(ctx, keycloakCfg.UserRealm, accessToken); err != nil {
		log.Fatalf("seed failed: %v", err)
	}

	fmt.Println()
	log.Println("Seed complete: mock data created in Keycloak and database")
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
