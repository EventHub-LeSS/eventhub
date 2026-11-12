// Package keycloakmock provides an in-memory mock of the Keycloak admin API
// used by the organization role endpoints. It is shared by handler and
// service tests so both exercise the same wire behavior (EVENTHUB-188).
package keycloakmock

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"backend/internal/model"
	"backend/internal/service"

	"github.com/google/uuid"
)

// Fake serves the Keycloak admin endpoints reached through the backend
// service account. All state is protected by Mu and safe for concurrent use.
type Fake struct {
	Mu           sync.Mutex
	Users        map[string]string            // username -> keycloak user id
	OrgGroups    map[string]map[string]string // org id -> group name -> group id
	OrgAliases   map[string]string            // org alias -> org id
	OrgNames     map[string]string            // org id -> stored name
	OrgDetails   map[string]map[string]any    // org id -> additional Keycloak fields
	OrgMembers   map[string]map[string]bool   // org id -> member user ids
	GroupMembers map[string]map[string]bool   // group id -> member user ids

	Grants        []string // "groupID/userID" in order
	Revokes       []string // "groupID/userID" in order
	AdminRequests []string // "METHOD path" in order, token endpoint excluded
	TokenRequests int      // service-account logins served

	// FailAdminRequest, when set, makes matching admin requests fail with 500.
	// It is consulted under Mu and may be mutated between requests.
	FailAdminRequest func(method, path string) bool
	// RejectAdminRequests makes the next n admin requests fail with 401, as Keycloak does for a
	// service-account token it no longer accepts.
	RejectAdminRequests int
	// AdminDelay delays every admin response, e.g. to exercise timeouts.
	AdminDelay time.Duration
	// Audit records the audited operations of the service returned by KeycloakService.
	Audit AuditLog
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
		OrgNames:   map[string]string{},
		OrgDetails: map[string]map[string]any{},
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
			reject := f.RejectAdminRequests > 0
			if reject {
				f.RejectAdminRequests--
			}
			delay := f.AdminDelay
			f.Mu.Unlock()
			if delay > 0 {
				select {
				case <-time.After(delay):
				case <-r.Context().Done():
					return
				}
			}
			if reject {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
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
		org := map[string]any{"id": orgID, "name": "Acme", "enabled": true}
		for key, value := range f.OrgDetails[orgID] {
			org[key] = value
		}
		if name, ok := f.OrgNames[orgID]; ok {
			org["name"] = name
		}
		for alias, id := range f.OrgAliases {
			if id == orgID {
				org["alias"] = alias
			}
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(org)
	}))

	// PUT sets the stored fields of the organization, like Keycloak's update
	// endpoint: the request body replaces the representation.
	mux.HandleFunc("PUT /admin/realms/eventhub/organizations/{orgID}", admin(func(w http.ResponseWriter, r *http.Request) {
		f.Mu.Lock()
		defer f.Mu.Unlock()
		orgID := r.PathValue("orgID")
		if _, ok := f.OrgGroups[orgID]; !ok {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		var org map[string]any
		if err := json.NewDecoder(r.Body).Decode(&org); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		name, nameOK := org["name"].(string)
		alias, aliasOK := org["alias"].(string)
		if !nameOK || name == "" || !aliasOK || alias == "" {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		for oldAlias, id := range f.OrgAliases {
			if id == orgID {
				delete(f.OrgAliases, oldAlias)
			}
		}
		f.OrgAliases[alias] = orgID
		f.OrgNames[orgID] = name
		delete(org, "id")
		delete(org, "name")
		delete(org, "alias")
		f.OrgDetails[orgID] = org
		w.WriteHeader(http.StatusNoContent)
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

	mux.HandleFunc("GET /admin/realms/eventhub/organizations/{orgID}/members", admin(func(w http.ResponseWriter, r *http.Request) {
		f.Mu.Lock()
		defer f.Mu.Unlock()
		members, ok := f.OrgMembers[r.PathValue("orgID")]
		if !ok {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		list := make([]map[string]any, 0, len(members))
		for username, userID := range f.Users {
			if members[userID] {
				list = append(list, map[string]any{"id": userID, "username": username})
			}
		}
		w.Header().Set("Content-Type", "application/json")
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
		// Real Keycloak filters organization groups out of the per-user groups
		// endpoint by group type, so a user whose only groups are org role
		// groups gets an empty list. The endpoint stays registered to mirror
		// that behavior and to let tests catch code that relies on it.
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode([]any{})
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

	// Organization creation (EVENTHUB-294): like Keycloak, the new ID is returned in the Location
	// header and a taken alias or name is a conflict.
	mux.HandleFunc("POST /admin/realms/eventhub/organizations", admin(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Name  string `json:"name"`
			Alias string `json:"alias"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		f.Mu.Lock()
		defer f.Mu.Unlock()
		if _, taken := f.OrgAliases[body.Alias]; taken {
			w.WriteHeader(http.StatusConflict)
			return
		}
		for _, name := range f.OrgNames {
			if name == body.Name {
				w.WriteHeader(http.StatusConflict)
				return
			}
		}
		orgID := "org-" + uuid.NewString()
		f.OrgAliases[body.Alias] = orgID
		f.OrgNames[orgID] = body.Name
		f.OrgGroups[orgID] = map[string]string{}
		f.OrgMembers[orgID] = map[string]bool{}
		w.Header().Set("Location", r.URL.Path+"/"+orgID)
		w.WriteHeader(http.StatusCreated)
	}))

	mux.HandleFunc("POST /admin/realms/eventhub/organizations/{orgID}/groups", admin(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Name string `json:"name"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		f.Mu.Lock()
		defer f.Mu.Unlock()
		groups, ok := f.OrgGroups[r.PathValue("orgID")]
		if !ok {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		groupID := "g-" + uuid.NewString()
		groups[body.Name] = groupID
		w.Header().Set("Location", r.URL.Path+"/"+groupID)
		w.WriteHeader(http.StatusCreated)
	}))

	mux.HandleFunc("POST /admin/realms/eventhub/organizations/{orgID}/members", admin(func(w http.ResponseWriter, r *http.Request) {
		raw, err := io.ReadAll(r.Body)
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		userID := strings.Trim(strings.TrimSpace(string(raw)), `"`)
		f.Mu.Lock()
		defer f.Mu.Unlock()
		members, ok := f.OrgMembers[r.PathValue("orgID")]
		if !ok {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		members[userID] = true
		w.WriteHeader(http.StatusCreated)
	}))

	mux.HandleFunc("DELETE /admin/realms/eventhub/organizations/{orgID}", admin(func(w http.ResponseWriter, r *http.Request) {
		f.Mu.Lock()
		defer f.Mu.Unlock()
		orgID := r.PathValue("orgID")
		for alias, id := range f.OrgAliases {
			if id == orgID {
				delete(f.OrgAliases, alias)
			}
		}
		delete(f.OrgNames, orgID)
		delete(f.OrgGroups, orgID)
		delete(f.OrgMembers, orgID)
		w.WriteHeader(http.StatusNoContent)
	}))

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

// KeycloakService returns a service wired to a fresh fake server and to the in-memory audit log of
// the fake.
func (f *Fake) KeycloakService(t testing.TB) *service.KeycloakService {
	t.Helper()
	return service.NewKeycloakService(service.KeycloakClientConfig{
		Host:         f.Server(t).URL,
		AdminRealm:   "master",
		UserRealm:    "eventhub",
		ClientID:     "backend",
		ClientSecret: "dev-secret",
	}).WithAuditLog(&f.Audit)
}

// AuditLog is an in-memory repository.AuditLogRepository. Fail, when set, makes Append fail for
// the entries it returns an error for.
type AuditLog struct {
	Mu      sync.Mutex
	Entries []model.AuditLogModel
	Fail    func(entry *model.AuditLogModel) error
}

func (a *AuditLog) Append(entry *model.AuditLogModel) error {
	a.Mu.Lock()
	defer a.Mu.Unlock()
	if a.Fail != nil {
		if err := a.Fail(entry); err != nil {
			return err
		}
	}
	stored := *entry
	// Round-trip through JSON like the database does, so tests see what a reader would see.
	raw, err := json.Marshal(entry.Changes)
	if err != nil {
		return err
	}
	stored.Changes = model.AuditChanges{}
	if err := json.Unmarshal(raw, &stored.Changes); err != nil {
		return err
	}
	stored.AuditLogID = uuid.New()
	// Strictly increasing at the cursor's microsecond precision, so that ordering by time is
	// unambiguous in tests: two entries never share an OccurredAt, which would make the
	// pagination tie-break fall back to the random UUID order of the append.
	stored.OccurredAt = time.Now().UTC().Truncate(time.Microsecond)
	if n := len(a.Entries); n > 0 && !stored.OccurredAt.After(a.Entries[n-1].OccurredAt) {
		stored.OccurredAt = a.Entries[n-1].OccurredAt.Add(time.Microsecond)
	}
	a.Entries = append(a.Entries, stored)
	return nil
}

func (a *AuditLog) ListByOrganization(keycloakOrgID string, limit int, cursor *model.AuditLogCursor) ([]model.AuditLogModel, error) {
	a.Mu.Lock()
	defer a.Mu.Unlock()
	var matching []model.AuditLogModel
	for i := len(a.Entries) - 1; i >= 0; i-- {
		e := a.Entries[i]
		if e.OrganizationID != keycloakOrgID {
			continue
		}
		if cursor != nil && !(e.OccurredAt.Before(cursor.OccurredAt) ||
			(e.OccurredAt.Equal(cursor.OccurredAt) && e.AuditLogID.String() < cursor.ID.String())) {
			continue
		}
		matching = append(matching, e)
		if len(matching) == limit {
			break
		}
	}
	if matching == nil {
		matching = []model.AuditLogModel{}
	}
	return matching, nil
}

// Snapshot returns a copy of the recorded entries in order.
func (a *AuditLog) Snapshot() []model.AuditLogModel {
	a.Mu.Lock()
	defer a.Mu.Unlock()
	return append([]model.AuditLogModel(nil), a.Entries...)
}
