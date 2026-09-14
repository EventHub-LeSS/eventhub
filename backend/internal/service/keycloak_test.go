package service

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
)

// fakeKeycloakAdmin serves the client-scope admin endpoints used by EnsureOrganizationClaimName.
// Like the real Keycloak 26.7, the .../protocol-mappers/models endpoint fills in claim.name as a
// default, while GET /client-scopes/{id} returns the stored config.
type fakeKeycloakAdmin struct {
	mu      sync.Mutex
	scopes  []map[string]any
	mappers []map[string]any
	puts    []map[string]any
}

func (f *fakeKeycloakAdmin) server(t *testing.T) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("GET /admin/realms/eventhub/client-scopes", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(f.scopes)
	})
	mux.HandleFunc("GET /admin/realms/eventhub/client-scopes/scope-org", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{"id": "scope-org", "name": "organization", "protocolMappers": f.mappers})
	})
	mux.HandleFunc("GET /admin/realms/eventhub/client-scopes/scope-org/protocol-mappers/models", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(withDefaultClaimName(f.mappers))
	})
	mux.HandleFunc("PUT /admin/realms/eventhub/client-scopes/scope-org/protocol-mappers/models/{id}", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer admin-token" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		f.mu.Lock()
		f.puts = append(f.puts, body)
		f.mu.Unlock()
		w.WriteHeader(http.StatusNoContent)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

// withDefaultClaimName mimics Keycloak's models endpoint, which reports claim.name even if unset.
func withDefaultClaimName(mappers []map[string]any) []map[string]any {
	out := make([]map[string]any, 0, len(mappers))
	for _, m := range mappers {
		config := map[string]any{"claim.name": "organization"}
		if c, ok := m["config"].(map[string]any); ok {
			for k, v := range c {
				config[k] = v
			}
		}
		copied := map[string]any{}
		for k, v := range m {
			copied[k] = v
		}
		copied["config"] = config
		out = append(out, copied)
	}
	return out
}

func organizationScopes() []map[string]any {
	return []map[string]any{
		{"id": "scope-profile", "name": "profile"},
		{"id": "scope-org", "name": "organization"},
	}
}

func membershipMapper(config map[string]any) map[string]any {
	return map[string]any{
		"id":             "mapper-membership",
		"name":           "organization membership",
		"protocol":       "openid-connect",
		"protocolMapper": "oidc-organization-membership-mapper",
		"config":         config,
	}
}

func groupsMapper() map[string]any {
	return map[string]any{
		"id":             "mapper-groups",
		"name":           "organization groups",
		"protocol":       "openid-connect",
		"protocolMapper": "oidc-organization-group-membership-mapper",
		"config":         map[string]any{"claim.name": "organization", "access.token.claim": "true"},
	}
}

func TestEnsureOrganizationClaimName_SetsMissingClaimNameAndKeepsConfig(t *testing.T) {
	fake := &fakeKeycloakAdmin{
		scopes: organizationScopes(),
		mappers: []map[string]any{
			groupsMapper(),
			membershipMapper(map[string]any{
				"addOrganizationId":         "true",
				"access.token.claim":        "true",
				"introspection.token.claim": "true",
			}),
		},
	}
	kc := NewKeycloakService(KeycloakClientConfig{Host: fake.server(t).URL})

	updated, err := kc.EnsureOrganizationClaimName(context.Background(), "admin-token", "eventhub")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !updated {
		t.Fatal("expected the membership mapper to be updated (the models endpoint must not hide the missing claim.name)")
	}
	if len(fake.puts) != 1 {
		t.Fatalf("expected exactly one PUT, got %d", len(fake.puts))
	}
	put := fake.puts[0]
	if put["id"] != "mapper-membership" {
		t.Errorf("updated wrong mapper: %v", put["id"])
	}
	config, _ := put["config"].(map[string]any)
	if config["claim.name"] != "organization" {
		t.Errorf("claim.name = %v, want organization", config["claim.name"])
	}
	for _, key := range []string{"addOrganizationId", "access.token.claim", "introspection.token.claim"} {
		if config[key] != "true" {
			t.Errorf("config key %q was lost or changed: %v", key, config[key])
		}
	}
}

func TestEnsureOrganizationClaimName_LeavesFixedMapperAlone(t *testing.T) {
	fake := &fakeKeycloakAdmin{
		scopes: organizationScopes(),
		mappers: []map[string]any{
			groupsMapper(),
			membershipMapper(map[string]any{"claim.name": "organization", "addOrganizationId": "true"}),
		},
	}
	kc := NewKeycloakService(KeycloakClientConfig{Host: fake.server(t).URL})

	updated, err := kc.EnsureOrganizationClaimName(context.Background(), "admin-token", "eventhub")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if updated || len(fake.puts) != 0 {
		t.Fatalf("expected no update, got updated=%v puts=%d", updated, len(fake.puts))
	}
}

func TestEnsureOrganizationClaimName_WithoutOrganizationScope(t *testing.T) {
	fake := &fakeKeycloakAdmin{scopes: []map[string]any{{"id": "scope-profile", "name": "profile"}}}
	kc := NewKeycloakService(KeycloakClientConfig{Host: fake.server(t).URL})

	updated, err := kc.EnsureOrganizationClaimName(context.Background(), "admin-token", "eventhub")
	if err != nil || updated {
		t.Fatalf("expected no error and no update, got updated=%v err=%v", updated, err)
	}
}
