package middleware

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/go-resty/resty/v2"
	"github.com/golang-jwt/jwt/v5"
	"github.com/henning-kln/gocloak"
)

const (
	principalContextKey  = "authenticated-principal"
	maxBearerTokenLength = 16 * 1024
	maxKeycloakResponse  = 1024 * 1024
)

var (
	ErrInvalidToken                = errors.New("invalid access token")
	ErrIdentityProviderUnavailable = errors.New("identity provider unavailable")
)

type AuthenticationConfig struct {
	Host             string
	Realm            string
	ClientID         string
	ClientSecret     string
	Audience         string
	AllowedAZP       string
	SigningAlgorithm string
	RequestTimeout   time.Duration
}

func LoadAuthenticationConfig() (AuthenticationConfig, error) {
	timeout := 2 * time.Second
	if value := os.Getenv("KEYCLOAK_REQUEST_TIMEOUT"); value != "" {
		parsed, err := time.ParseDuration(value)
		if err != nil || parsed <= 0 {
			return AuthenticationConfig{}, fmt.Errorf("KEYCLOAK_REQUEST_TIMEOUT must be a positive duration")
		}
		timeout = parsed
	}

	config := AuthenticationConfig{
		Host:             strings.TrimRight(os.Getenv("KEYCLOAK_HOST"), "/"),
		Realm:            os.Getenv("KEYCLOAK_REALM"),
		ClientID:         os.Getenv("KEYCLOAK_CLIENT_ID"),
		ClientSecret:     os.Getenv("KEYCLOAK_CLIENT_SECRET"),
		Audience:         os.Getenv("KEYCLOAK_AUDIENCE"),
		AllowedAZP:       os.Getenv("KEYCLOAK_ALLOWED_AZP"),
		SigningAlgorithm: os.Getenv("KEYCLOAK_SIGNING_ALGORITHM"),
		RequestTimeout:   timeout,
	}
	if config.SigningAlgorithm == "" {
		config.SigningAlgorithm = jwt.SigningMethodRS256.Alg()
	}
	if err := config.validate(); err != nil {
		return AuthenticationConfig{}, err
	}
	return config, nil
}

func (config AuthenticationConfig) validate() error {
	for name, value := range map[string]string{
		"KEYCLOAK_HOST":          config.Host,
		"KEYCLOAK_REALM":         config.Realm,
		"KEYCLOAK_CLIENT_ID":     config.ClientID,
		"KEYCLOAK_CLIENT_SECRET": config.ClientSecret,
		"KEYCLOAK_AUDIENCE":      config.Audience,
		"KEYCLOAK_ALLOWED_AZP":   config.AllowedAZP,
	} {
		if value == "" {
			return fmt.Errorf("%s is required", name)
		}
	}
	parsedHost, err := url.Parse(config.Host)
	if err != nil || parsedHost.Host == "" || (parsedHost.Scheme != "http" && parsedHost.Scheme != "https") {
		return fmt.Errorf("KEYCLOAK_HOST must be an absolute HTTP(S) URL")
	}
	if parsedHost.Scheme != "https" && parsedHost.Hostname() != "localhost" && parsedHost.Hostname() != "127.0.0.1" {
		return fmt.Errorf("KEYCLOAK_HOST must use HTTPS outside local development")
	}
	if config.SigningAlgorithm != jwt.SigningMethodRS256.Alg() {
		return fmt.Errorf("KEYCLOAK_SIGNING_ALGORITHM must be RS256")
	}
	if config.RequestTimeout <= 0 {
		return fmt.Errorf("KEYCLOAK_REQUEST_TIMEOUT must be positive")
	}
	return nil
}

type keycloakTokenClient interface {
	RetrospectToken(context.Context, string, string, string, string) (*gocloak.IntroSpectTokenResult, error)
	DecodeAccessTokenCustomClaims(context.Context, string, string, jwt.Claims) (*jwt.Token, error)
}

type Authenticator struct {
	client keycloakTokenClient
	config AuthenticationConfig
}

func NewAuthenticator(config AuthenticationConfig) (*Authenticator, error) {
	if err := config.validate(); err != nil {
		return nil, err
	}
	client := gocloak.NewClient(config.Host)
	client.SetRestyClient(newKeycloakHTTPClient(config.RequestTimeout))
	return newAuthenticator(config, client), nil
}

func newAuthenticator(config AuthenticationConfig, client keycloakTokenClient) *Authenticator {
	return &Authenticator{client: client, config: config}
}

func newKeycloakHTTPClient(timeout time.Duration) *resty.Client {
	transport := &http.Transport{
		Proxy:                 http.ProxyFromEnvironment,
		DialContext:           (&net.Dialer{Timeout: timeout}).DialContext,
		TLSClientConfig:       &tls.Config{MinVersion: tls.VersionTLS12},
		TLSHandshakeTimeout:   timeout,
		ResponseHeaderTimeout: timeout,
		IdleConnTimeout:       90 * time.Second,
		MaxIdleConns:          20,
		MaxIdleConnsPerHost:   10,
	}
	return resty.New().
		SetTransport(transport).
		SetTimeout(timeout).
		SetResponseBodyLimit(maxKeycloakResponse).
		SetRedirectPolicy(resty.NoRedirectPolicy())
}

type resourceRoles struct {
	Roles []string `json:"roles"`
}

type organizationClaim struct {
	ID     string   `json:"id"`
	Groups []string `json:"groups"`
}

type accessClaims struct {
	jwt.RegisteredClaims
	PreferredUsername string                       `json:"preferred_username"`
	AuthorizedParty   string                       `json:"azp"`
	ResourceAccess    map[string]resourceRoles     `json:"resource_access"`
	Organizations     map[string]organizationClaim `json:"organization"`
}

func (a *Authenticator) Authenticate(ctx context.Context, rawToken string) (*Principal, error) {
	introspection, err := a.client.RetrospectToken(ctx, rawToken, a.config.ClientID, a.config.ClientSecret, a.config.Realm)
	if err != nil {
		return nil, fmt.Errorf("%w: token introspection failed", ErrIdentityProviderUnavailable)
	}
	if introspection == nil || introspection.Active == nil || !*introspection.Active {
		return nil, ErrInvalidToken
	}

	claims := &accessClaims{}
	token, err := a.client.DecodeAccessTokenCustomClaims(ctx, rawToken, a.config.Realm, claims)
	if err != nil || token == nil || !token.Valid {
		// Introspection already established that Keycloak considers this token active.
		// A subsequent verification failure indicates an inconsistent identity service.
		return nil, fmt.Errorf("%w: active token verification failed", ErrIdentityProviderUnavailable)
	}
	if token.Method == nil || token.Method.Alg() != a.config.SigningAlgorithm {
		return nil, ErrInvalidToken
	}
	if claims.ExpiresAt == nil || claims.Subject == "" || claims.AuthorizedParty != a.config.AllowedAZP {
		return nil, ErrInvalidToken
	}
	validator := jwt.NewValidator(
		jwt.WithIssuer(a.issuer()),
		jwt.WithAudience(a.config.Audience),
		jwt.WithExpirationRequired(),
		jwt.WithLeeway(30*time.Second),
	)
	if err := validator.Validate(claims); err != nil {
		return nil, ErrInvalidToken
	}

	principal, err := a.principalFromClaims(claims)
	if err != nil {
		return nil, ErrInvalidToken
	}
	return principal, nil
}

func (a *Authenticator) issuer() string {
	return a.config.Host + "/realms/" + a.config.Realm
}

func (a *Authenticator) principalFromClaims(claims *accessClaims) (*Principal, error) {
	principal := &Principal{
		Subject:         claims.Subject,
		Username:        claims.PreferredUsername,
		AuthorizedParty: claims.AuthorizedParty,
		ExpiresAt:       claims.ExpiresAt.Time,
		GlobalRoles:     make(map[GlobalRole]struct{}),
	}
	for _, role := range claims.ResourceAccess[a.config.Audience].Roles {
		switch GlobalRole(role) {
		case RoleAdmin, RoleModerator, RoleVisitor:
			principal.GlobalRoles[GlobalRole(role)] = struct{}{}
		}
	}

	if len(claims.Organizations) > 1 {
		return nil, errors.New("multiple active organizations")
	}
	for alias, organization := range claims.Organizations {
		if alias == "" || organization.ID == "" {
			return nil, errors.New("invalid organization claim")
		}
		access := &OrganizationAccess{
			ID:    organization.ID,
			Alias: alias,
			Roles: make(map[OrganizationRole]struct{}),
		}
		for _, group := range organization.Groups {
			switch group {
			case "/roles/org_admin":
				access.Roles[RoleOrganizationAdmin] = struct{}{}
			case "/roles/event_manager":
				access.Roles[RoleEventManager] = struct{}{}
			case "/roles/finance_viewer":
				access.Roles[RoleFinanceViewer] = struct{}{}
			}
		}
		principal.ActiveOrganization = access
	}

	return principal, nil
}

func (a *Authenticator) Middleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		rawToken, err := bearerToken(c.Request.Header.Values("Authorization"))
		if err != nil {
			abortAuthentication(c, http.StatusUnauthorized, "UNAUTHENTICATED", "Authentication is required")
			return
		}

		principal, err := a.Authenticate(c.Request.Context(), rawToken)
		if err != nil {
			if errors.Is(err, ErrIdentityProviderUnavailable) {
				abortAuthentication(c, http.StatusServiceUnavailable, "IDENTITY_PROVIDER_UNAVAILABLE", "Authentication service is temporarily unavailable")
				return
			}
			abortAuthentication(c, http.StatusUnauthorized, "UNAUTHENTICATED", "Authentication is required")
			return
		}

		c.Set(principalContextKey, principal)
		c.Next()
	}
}

func PrincipalFromContext(c *gin.Context) (*Principal, bool) {
	value, exists := c.Get(principalContextKey)
	if !exists {
		return nil, false
	}
	principal, ok := value.(*Principal)
	return principal, ok && principal != nil
}

func bearerToken(headers []string) (string, error) {
	if len(headers) != 1 {
		return "", ErrInvalidToken
	}
	parts := strings.Fields(headers[0])
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") || len(parts[1]) > maxBearerTokenLength {
		return "", ErrInvalidToken
	}
	return parts[1], nil
}

func abortAuthentication(c *gin.Context, status int, code, message string) {
	if status == http.StatusUnauthorized {
		c.Header("WWW-Authenticate", `Bearer realm="eventhub", error="invalid_token"`)
	}
	c.AbortWithStatusJSON(status, gin.H{"error": gin.H{"code": code, "message": message}})
}
