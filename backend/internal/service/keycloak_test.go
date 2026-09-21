package service

import (
	"context"
	"encoding/json"
	"fmt"
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

// fakeServiceAccountAdmin serves the client and mapping admin endpoints used by EnsureServiceAccount.
type fakeServiceAccountAdmin struct {
	mu             sync.Mutex
	backend        map[string]any
	roleMappings   []string
	scopeMappings  []string
	clientPuts     []map[string]any
	rolePosts      [][]string
	scopePosts     [][]string
	realmMgmtRoles []map[string]any
}

func newFakeServiceAccountAdmin(enabled bool, roleMappings, scopeMappings []string) *fakeServiceAccountAdmin {
	return &fakeServiceAccountAdmin{
		backend: map[string]any{
			"id": "client-backend", "clientId": "backend", "secret": "dev-secret",
			"serviceAccountsEnabled": enabled, "fullScopeAllowed": false,
		},
		roleMappings:  roleMappings,
		scopeMappings: scopeMappings,
		realmMgmtRoles: []map[string]any{
			{"id": "r-orgs", "name": "manage-organizations"},
			{"id": "r-users", "name": "manage-users"},
			{"id": "r-admin", "name": "realm-admin"},
		},
	}
}

func roleNames(names []string) []map[string]any {
	out := make([]map[string]any, 0, len(names))
	for _, name := range names {
		out = append(out, map[string]any{"name": name})
	}
	return out
}

func decodeRoleNames(r *http.Request) []string {
	var roles []map[string]any
	_ = json.NewDecoder(r.Body).Decode(&roles)
	names := make([]string, 0, len(roles))
	for _, role := range roles {
		names = append(names, role["name"].(string))
	}
	return names
}

func (f *fakeServiceAccountAdmin) server(t *testing.T) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("GET /admin/realms/eventhub/clients", func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		switch r.URL.Query().Get("clientId") {
		case "backend":
			json.NewEncoder(w).Encode([]map[string]any{f.backend})
		case "realm-management":
			json.NewEncoder(w).Encode([]map[string]any{{"id": "client-rm", "clientId": "realm-management"}})
		default:
			json.NewEncoder(w).Encode([]map[string]any{})
		}
	})
	mux.HandleFunc("PUT /admin/realms/eventhub/clients/client-backend", func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		f.mu.Lock()
		f.clientPuts = append(f.clientPuts, body)
		f.backend = body
		f.mu.Unlock()
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("GET /admin/realms/eventhub/clients/client-backend/service-account-user", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{"id": "sa-user", "username": "service-account-backend"})
	})
	mux.HandleFunc("GET /admin/realms/eventhub/clients/client-rm/roles", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(f.realmMgmtRoles)
	})
	mux.HandleFunc("GET /admin/realms/eventhub/users/sa-user/role-mappings/clients/client-rm", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(roleNames(f.roleMappings))
	})
	mux.HandleFunc("POST /admin/realms/eventhub/users/sa-user/role-mappings/clients/client-rm", func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		f.rolePosts = append(f.rolePosts, decodeRoleNames(r))
		f.mu.Unlock()
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("GET /admin/realms/eventhub/clients/client-backend/scope-mappings/clients/client-rm", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(roleNames(f.scopeMappings))
	})
	mux.HandleFunc("POST /admin/realms/eventhub/clients/client-backend/scope-mappings/clients/client-rm", func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		f.scopePosts = append(f.scopePosts, decodeRoleNames(r))
		f.mu.Unlock()
		w.WriteHeader(http.StatusNoContent)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func TestEnsureServiceAccount_RepairsRealmWithoutServiceAccount(t *testing.T) {
	fake := newFakeServiceAccountAdmin(false, nil, nil)
	kc := NewKeycloakService(KeycloakClientConfig{Host: fake.server(t).URL, ClientID: "backend"})

	updated, err := kc.EnsureServiceAccount(context.Background(), "admin-token", "eventhub")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !updated {
		t.Fatal("expected the service account to be repaired")
	}
	if len(fake.clientPuts) != 1 || fake.clientPuts[0]["serviceAccountsEnabled"] != true {
		t.Fatalf("expected one client PUT enabling the service account, got %v", fake.clientPuts)
	}
	if fake.clientPuts[0]["secret"] != "dev-secret" || fake.clientPuts[0]["fullScopeAllowed"] != false {
		t.Errorf("client attributes were lost or changed: %v", fake.clientPuts[0])
	}
	want := "[manage-organizations manage-users]"
	if len(fake.rolePosts) != 1 || fmt.Sprint(fake.rolePosts[0]) != want {
		t.Errorf("role mapping posts = %v, want one post with %s", fake.rolePosts, want)
	}
	// Without the scope mapping the roles never reach the token (fullScopeAllowed=false).
	if len(fake.scopePosts) != 1 || fmt.Sprint(fake.scopePosts[0]) != want {
		t.Errorf("scope mapping posts = %v, want one post with %s", fake.scopePosts, want)
	}
}

func TestEnsureServiceAccount_AddsOnlyMissingRoles(t *testing.T) {
	fake := newFakeServiceAccountAdmin(true, []string{"manage-users"}, []string{"manage-organizations", "manage-users"})
	kc := NewKeycloakService(KeycloakClientConfig{Host: fake.server(t).URL, ClientID: "backend"})

	updated, err := kc.EnsureServiceAccount(context.Background(), "admin-token", "eventhub")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !updated {
		t.Fatal("expected the missing role to be added")
	}
	if len(fake.clientPuts) != 0 {
		t.Errorf("client was rewritten although the service account was enabled: %v", fake.clientPuts)
	}
	if len(fake.rolePosts) != 1 || fmt.Sprint(fake.rolePosts[0]) != "[manage-organizations]" {
		t.Errorf("role mapping posts = %v, want only manage-organizations", fake.rolePosts)
	}
	if len(fake.scopePosts) != 0 {
		t.Errorf("scope mapping was complete but got posts: %v", fake.scopePosts)
	}
}

func TestEnsureServiceAccount_LeavesConfiguredRealmAlone(t *testing.T) {
	all := []string{"manage-organizations", "manage-users"}
	fake := newFakeServiceAccountAdmin(true, all, all)
	kc := NewKeycloakService(KeycloakClientConfig{Host: fake.server(t).URL, ClientID: "backend"})

	updated, err := kc.EnsureServiceAccount(context.Background(), "admin-token", "eventhub")
	if err != nil || updated {
		t.Fatalf("expected no error and no update, got updated=%v err=%v", updated, err)
	}
	if len(fake.clientPuts)+len(fake.rolePosts)+len(fake.scopePosts) != 0 {
		t.Errorf("expected no writes, got puts=%v roles=%v scopes=%v", fake.clientPuts, fake.rolePosts, fake.scopePosts)
	}
}
