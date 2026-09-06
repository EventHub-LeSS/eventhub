package middleware

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/henning-kln/gocloak"
)

type fakeKeycloakClient struct {
	active        bool
	introspectErr error
	decodeErr     error
	claims        *accessClaims
	method        jwt.SigningMethod
}

func (f *fakeKeycloakClient) RetrospectToken(context.Context, string, string, string, string) (*gocloak.IntroSpectTokenResult, error) {
	if f.introspectErr != nil {
		return nil, f.introspectErr
	}
	return &gocloak.IntroSpectTokenResult{Active: &f.active}, nil
}

func (f *fakeKeycloakClient) DecodeAccessTokenCustomClaims(_ context.Context, _ string, _ string, claims jwt.Claims) (*jwt.Token, error) {
	if f.decodeErr != nil {
		return nil, f.decodeErr
	}
	target := claims.(*accessClaims)
	*target = *f.claims
	method := f.method
	if method == nil {
		method = jwt.SigningMethodRS256
	}
	return &jwt.Token{Method: method, Valid: true}, nil
}

func testConfig() AuthenticationConfig {
	return AuthenticationConfig{
		Host:             "https://auth.example.test",
		Realm:            "eventhub",
		ClientID:         "backend",
		ClientSecret:     "secret",
		Audience:         "backend",
		AllowedAZP:       "frontend",
		SigningAlgorithm: "RS256",
		RequestTimeout:   time.Second,
	}
}

func validClaims() *accessClaims {
	now := time.Now()
	return &accessClaims{
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    "https://auth.example.test/realms/eventhub",
			Subject:   "user-123",
			Audience:  jwt.ClaimStrings{"backend"},
			ExpiresAt: jwt.NewNumericDate(now.Add(time.Hour)),
			IssuedAt:  jwt.NewNumericDate(now),
		},
		PreferredUsername: "alice",
		AuthorizedParty:   "frontend",
		ResourceAccess: map[string]resourceRoles{
			"backend": {Roles: []string{"visitor", "untrusted-role"}},
		},
		Organizations: map[string]organizationClaim{
			"acme": {ID: "org-123", Groups: []string{"/roles/event_manager", "/untrusted"}},
		},
	}
}

func TestAuthenticateBuildsAllowlistedPrincipal(t *testing.T) {
	client := &fakeKeycloakClient{active: true, claims: validClaims()}
	principal, err := newAuthenticator(testConfig(), client).Authenticate(context.Background(), "token")
	if err != nil {
		t.Fatalf("Authenticate() error = %v", err)
	}
	if principal.Subject != "user-123" || principal.Username != "alice" {
		t.Fatalf("unexpected principal identity: %#v", principal)
	}
	if !principal.HasGlobalRole(RoleVisitor) || principal.HasGlobalRole(RoleAdmin) {
		t.Fatalf("unexpected global roles: %#v", principal.GlobalRoles)
	}
	if principal.ActiveOrganization == nil || principal.ActiveOrganization.ID != "org-123" {
		t.Fatalf("unexpected organization: %#v", principal.ActiveOrganization)
	}
	if !principal.HasOrganizationRole(RoleEventManager) || principal.HasOrganizationRole(RoleFinanceViewer) {
		t.Fatalf("unexpected organization roles: %#v", principal.ActiveOrganization.Roles)
	}
}

func TestAuthenticateRejectsInactiveToken(t *testing.T) {
	client := &fakeKeycloakClient{active: false, claims: validClaims()}
	_, err := newAuthenticator(testConfig(), client).Authenticate(context.Background(), "token")
	if !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("Authenticate() error = %v, want ErrInvalidToken", err)
	}
}

func TestAuthenticateTreatsIntrospectionFailureAsUnavailable(t *testing.T) {
	client := &fakeKeycloakClient{introspectErr: errors.New("timeout")}
	_, err := newAuthenticator(testConfig(), client).Authenticate(context.Background(), "token")
	if !errors.Is(err, ErrIdentityProviderUnavailable) {
		t.Fatalf("Authenticate() error = %v, want ErrIdentityProviderUnavailable", err)
	}
}

func TestAuthenticateTreatsActiveTokenDecodeFailureAsUnavailable(t *testing.T) {
	client := &fakeKeycloakClient{active: true, decodeErr: errors.New("could not fetch JWKS")}
	_, err := newAuthenticator(testConfig(), client).Authenticate(context.Background(), "token")
	if !errors.Is(err, ErrIdentityProviderUnavailable) {
		t.Fatalf("Authenticate() error = %v, want ErrIdentityProviderUnavailable", err)
	}
}

func TestAuthenticateAllowsNoOrganizationForGlobalAccess(t *testing.T) {
	claims := validClaims()
	claims.Organizations = nil
	client := &fakeKeycloakClient{active: true, claims: claims}
	principal, err := newAuthenticator(testConfig(), client).Authenticate(context.Background(), "token")
	if err != nil {
		t.Fatalf("Authenticate() error = %v", err)
	}
	if principal.ActiveOrganization != nil {
		t.Fatalf("ActiveOrganization = %#v, want nil", principal.ActiveOrganization)
	}
}

func TestAuthenticateAllowsRequiredAudienceAmongMultipleAudiences(t *testing.T) {
	claims := validClaims()
	claims.Audience = jwt.ClaimStrings{"account", "backend"}
	client := &fakeKeycloakClient{active: true, claims: claims}
	if _, err := newAuthenticator(testConfig(), client).Authenticate(context.Background(), "token"); err != nil {
		t.Fatalf("Authenticate() error = %v", err)
	}
}

func TestAuthenticateRejectsInvalidClaims(t *testing.T) {
	tests := map[string]func(*accessClaims){
		"wrong issuer":           func(claims *accessClaims) { claims.Issuer = "https://attacker.test" },
		"wrong audience":         func(claims *accessClaims) { claims.Audience = jwt.ClaimStrings{"other"} },
		"wrong authorized party": func(claims *accessClaims) { claims.AuthorizedParty = "other" },
		"missing subject":        func(claims *accessClaims) { claims.Subject = "" },
		"missing expiration":     func(claims *accessClaims) { claims.ExpiresAt = nil },
		"expired":                func(claims *accessClaims) { claims.ExpiresAt = jwt.NewNumericDate(time.Now().Add(-time.Minute)) },
		"multiple organizations": func(claims *accessClaims) {
			claims.Organizations["other"] = organizationClaim{ID: "org-456"}
		},
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			claims := validClaims()
			mutate(claims)
			client := &fakeKeycloakClient{active: true, claims: claims}
			_, err := newAuthenticator(testConfig(), client).Authenticate(context.Background(), "token")
			if !errors.Is(err, ErrInvalidToken) {
				t.Fatalf("Authenticate() error = %v, want ErrInvalidToken", err)
			}
		})
	}
}

func TestAuthenticateRejectsUnexpectedSigningAlgorithm(t *testing.T) {
	client := &fakeKeycloakClient{active: true, claims: validClaims(), method: jwt.SigningMethodRS512}
	_, err := newAuthenticator(testConfig(), client).Authenticate(context.Background(), "token")
	if !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("Authenticate() error = %v, want ErrInvalidToken", err)
	}
}

func TestNewAuthenticatorRejectsInvalidConfig(t *testing.T) {
	config := testConfig()
	config.SigningAlgorithm = "HS256"
	if _, err := NewAuthenticator(config); err == nil {
		t.Fatal("NewAuthenticator() error = nil")
	}
}

func TestAuthenticationMiddlewareResponses(t *testing.T) {
	gin.SetMode(gin.TestMode)
	tests := []struct {
		name       string
		header     string
		client     *fakeKeycloakClient
		wantStatus int
		wantCode   string
	}{
		{name: "missing token", wantStatus: http.StatusUnauthorized, wantCode: "UNAUTHENTICATED"},
		{name: "inactive token", header: "Bearer token", client: &fakeKeycloakClient{active: false}, wantStatus: http.StatusUnauthorized, wantCode: "UNAUTHENTICATED"},
		{name: "keycloak unavailable", header: "Bearer token", client: &fakeKeycloakClient{introspectErr: errors.New("timeout")}, wantStatus: http.StatusServiceUnavailable, wantCode: "IDENTITY_PROVIDER_UNAVAILABLE"},
		{name: "JWKS unavailable", header: "Bearer token", client: &fakeKeycloakClient{active: true, decodeErr: errors.New("timeout")}, wantStatus: http.StatusServiceUnavailable, wantCode: "IDENTITY_PROVIDER_UNAVAILABLE"},
		{name: "valid token", header: "Bearer token", client: &fakeKeycloakClient{active: true, claims: validClaims()}, wantStatus: http.StatusNoContent},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := tt.client
			if client == nil {
				client = &fakeKeycloakClient{}
			}
			router := gin.New()
			router.Use(newAuthenticator(testConfig(), client).Middleware())
			router.GET("/protected", func(c *gin.Context) {
				if _, ok := PrincipalFromContext(c); !ok {
					t.Fatal("principal missing from context")
				}
				c.Status(http.StatusNoContent)
			})

			request := httptest.NewRequest(http.MethodGet, "/protected", nil)
			if tt.header != "" {
				request.Header.Set("Authorization", tt.header)
			}
			response := httptest.NewRecorder()
			router.ServeHTTP(response, request)

			if response.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d; body = %s", response.Code, tt.wantStatus, response.Body.String())
			}
			if tt.wantCode != "" && !strings.Contains(response.Body.String(), tt.wantCode) {
				t.Fatalf("body = %q, want code %q", response.Body.String(), tt.wantCode)
			}
		})
	}
}

func TestBearerTokenRejectsAmbiguousAndOversizedHeaders(t *testing.T) {
	for name, headers := range map[string][]string{
		"duplicate":    {"Bearer one", "Bearer two"},
		"wrong scheme": {"Basic token"},
		"oversized":    {"Bearer " + strings.Repeat("a", maxBearerTokenLength+1)},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := bearerToken(headers); err == nil {
				t.Fatal("bearerToken() error = nil")
			}
		})
	}
}
