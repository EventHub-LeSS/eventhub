// Package keycloakmock provides an in-memory mock of the Keycloak admin API
// used by the organization role endpoints. It is shared by handler and
// service tests so both exercise the same wire behavior (EVENTHUB-188).
package keycloakmock

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"backend/internal/service"
)

// Fake serves the Keycloak admin endpoints reached through the backend
// service account. All state is protected by Mu and safe for concurrent use.
type Fake struct {
	Mu           sync.Mutex
	Users        map[string]string            // username -> keycloak user id
	OrgGroups    map[string]map[string]string // org id -> group name -> group id
	OrgAliases   map[string]string            // org alias -> org id
	OrgMembers   map[string]map[string]bool   // org id -> member user ids
	GroupMembers map[string]map[string]bool   // group id -> member user ids

	Grants        []string // "groupID/userID" in order
	Revokes       []string // "groupID/userID" in order
	AdminRequests []string // "METHOD path" in order, token endpoint excluded
	TokenRequests int      // service-account logins served

	// FailAdminRequest, when set, makes matching admin requests fail with 500.
	// It is consulted under Mu and may be mutated between requests.
	FailAdminRequest func(method, path string) bool
}

// New returns a fake with two members: alice (org_admin of org-1) and bob
// (event_manager of org-1). carol exists in the realm but is not a member.
func New() *Fake {
	return &Fake{
		Users: map[string]string{
			"alice": "u-alice",
			"bob":   "u-bob",
			"carol": "u-carol",
		},
		OrgGroups: map[string]map[string]string{
			"org-1": {"org_admin": "g-admin", "event_manager": "g-manager", "finance_viewer": "g-finance"},
		},
		OrgAliases: map[string]string{
			"alias-1": "org-1",
		},
		OrgMembers: map[string]map[string]bool{
			"org-1": {"u-alice": true, "u-bob": true},
		},
		GroupMembers: map[string]map[string]bool{
			"g-admin":   {"u-alice": true},
			"g-manager": {"u-bob": true},
		},
	}
}

// Server starts the fake Keycloak and registers its shutdown with the test cleanup.
func (f *Fake) Server(t testing.TB) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()

	mux.HandleFunc("POST /realms/master/protocol/openid-connect/token", func(w http.ResponseWriter, r *http.Request) {
		f.Mu.Lock()
		defer f.Mu.Unlock()
		f.TokenRequests++
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"access_token": "svc-token", "token_type": "Bearer", "expires_in": 300})
	})

	// Organization operations run with the backend service account.
	admin := func(h http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			if r.Header.Get("Authorization") != "Bearer svc-token" {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			f.Mu.Lock()
			f.AdminRequests = append(f.AdminRequests, r.Method+" "+r.URL.Path)
			fail := f.FailAdminRequest != nil && f.FailAdminRequest(r.Method, r.URL.Path)
			f.Mu.Unlock()
			if fail {
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
			h(w, r)
		}
	}

	mux.HandleFunc("GET /admin/realms/eventhub/users", admin(func(w http.ResponseWriter, r *http.Request) {
		f.Mu.Lock()
		defer f.Mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		username := r.URL.Query().Get("username")
		userID, ok := f.Users[username]
		if !ok {
			_ = json.NewEncoder(w).Encode([]any{})
			return
		}
		_ = json.NewEncoder(w).Encode([]map[string]any{{"id": userID, "username": username}})
	}))

	// Like Keycloak, the by-ID endpoint only accepts organization IDs; aliases
	// are resolved through the organization listing.
	mux.HandleFunc("GET /admin/realms/eventhub/organizations/{orgID}", admin(func(w http.ResponseWriter, r *http.Request) {
		f.Mu.Lock()
		defer f.Mu.Unlock()
		orgID := r.PathValue("orgID")
		if _, ok := f.OrgGroups[orgID]; !ok {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"id": orgID})
	}))

	mux.HandleFunc("GET /admin/realms/eventhub/organizations", admin(func(w http.ResponseWriter, r *http.Request) {
		f.Mu.Lock()
		defer f.Mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		list := make([]map[string]any, 0, len(f.OrgAliases))
		for alias, orgID := range f.OrgAliases {
			list = append(list, map[string]any{"id": orgID, "alias": alias})
		}
		_ = json.NewEncoder(w).Encode(list)
	}))

	mux.HandleFunc("GET /admin/realms/eventhub/organizations/{orgID}/groups", admin(func(w http.ResponseWriter, r *http.Request) {
		f.Mu.Lock()
		defer f.Mu.Unlock()
		groups, ok := f.OrgGroups[r.PathValue("orgID")]
		if !ok {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		list := make([]map[string]any, 0, len(groups))
		for name, id := range groups {
			list = append(list, map[string]any{"id": id, "name": name})
		}
		_ = json.NewEncoder(w).Encode(list)
	}))

	mux.HandleFunc("GET /admin/realms/eventhub/organizations/{orgID}/members/{userID}", admin(func(w http.ResponseWriter, r *http.Request) {
		f.Mu.Lock()
		defer f.Mu.Unlock()
		if f.OrgMembers[r.PathValue("orgID")][r.PathValue("userID")] {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"id": r.PathValue("userID")})
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))

	mux.HandleFunc("GET /admin/realms/eventhub/users/{userID}/groups", admin(func(w http.ResponseWriter, r *http.Request) {
		f.Mu.Lock()
		defer f.Mu.Unlock()
		// The fake stores memberships per group, so the user's groups are
		// resolved by a reverse lookup. First/Max are ignored; the datasets in
		// tests stay small enough that callers never page.
		groupNames := make(map[string]string)
		for _, groups := range f.OrgGroups {
			for name, id := range groups {
				groupNames[id] = name
			}
		}
		userID := r.PathValue("userID")
		w.Header().Set("Content-Type", "application/json")
		list := make([]map[string]any, 0)
		for groupID, members := range f.GroupMembers {
			if members[userID] {
				list = append(list, map[string]any{"id": groupID, "name": groupNames[groupID]})
			}
		}
		_ = json.NewEncoder(w).Encode(list)
	}))

	mux.HandleFunc("GET /admin/realms/eventhub/organizations/{orgID}/groups/{groupID}/members", admin(func(w http.ResponseWriter, r *http.Request) {
		f.Mu.Lock()
		defer f.Mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		list := make([]map[string]any, 0)
		for userID := range f.GroupMembers[r.PathValue("groupID")] {
			list = append(list, map[string]any{"id": userID})
		}
		_ = json.NewEncoder(w).Encode(list)
	}))

	mux.HandleFunc("PUT /admin/realms/eventhub/organizations/{orgID}/groups/{groupID}/members/{userID}", admin(func(w http.ResponseWriter, r *http.Request) {
		f.Mu.Lock()
		defer f.Mu.Unlock()
		groupID, userID := r.PathValue("groupID"), r.PathValue("userID")
		f.Grants = append(f.Grants, groupID+"/"+userID)
		if f.GroupMembers[groupID] == nil {
			f.GroupMembers[groupID] = map[string]bool{}
		}
		f.GroupMembers[groupID][userID] = true
		w.WriteHeader(http.StatusNoContent)
	}))

	mux.HandleFunc("DELETE /admin/realms/eventhub/organizations/{orgID}/groups/{groupID}/members/{userID}", admin(func(w http.ResponseWriter, r *http.Request) {
		f.Mu.Lock()
		defer f.Mu.Unlock()
		groupID, userID := r.PathValue("groupID"), r.PathValue("userID")
		f.Revokes = append(f.Revokes, groupID+"/"+userID)
		delete(f.GroupMembers[groupID], userID)
		w.WriteHeader(http.StatusNoContent)
	}))

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

// KeycloakService returns a service wired to a fresh fake server.
func (f *Fake) KeycloakService(t testing.TB) *service.KeycloakService {
	t.Helper()
	return service.NewKeycloakService(service.KeycloakClientConfig{
		Host:         f.Server(t).URL,
		AdminRealm:   "master",
		UserRealm:    "eventhub",
		ClientID:     "backend",
		ClientSecret: "dev-secret",
	})
}
