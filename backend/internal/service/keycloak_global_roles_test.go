package service

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"sync/atomic"
	"testing"
)

func TestSetUserGlobalRolesRejectsRemovingLastGlobalAdmin(t *testing.T) {
	var mutations atomic.Int32
	mux := http.NewServeMux()
	mux.HandleFunc("POST /realms/eventhub/protocol/openid-connect/token", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if _, err := io.Copy(io.Discard, r.Body); err != nil {
			t.Fatalf("read token request: %v", err)
		}
		if _, err := io.WriteString(w, `{"access_token":"test-service-token","expires_in":300}`); err != nil {
			t.Fatalf("write token response: %v", err)
		}
	})
	mux.HandleFunc("GET /admin/realms/eventhub/clients", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if _, err := io.WriteString(w, `[{"id":"client-backend","clientId":"backend"}]`); err != nil {
			t.Fatalf("write clients response: %v", err)
		}
	})
	mux.HandleFunc("GET /admin/realms/eventhub/users/{id}/role-mappings/clients/client-backend", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if _, err := io.WriteString(w, `[{"id":"role-admin","name":"admin"}]`); err != nil {
			t.Fatalf("write user roles response: %v", err)
		}
	})
	mux.HandleFunc("GET /admin/realms/eventhub/clients/client-backend/roles", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if _, err := io.WriteString(w, `[{"id":"role-admin","name":"admin"},{"id":"role-moderator","name":"moderator"},{"id":"role-visitor","name":"visitor"}]`); err != nil {
			t.Fatalf("write available roles response: %v", err)
		}
	})
	mux.HandleFunc("GET /admin/realms/eventhub/clients/client-backend/roles/admin/users", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if _, err := io.WriteString(w, `[{"id":"user-1"}]`); err != nil {
			t.Fatalf("write admin users response: %v", err)
		}
	})
	mux.HandleFunc("POST /admin/realms/eventhub/users/user-1/role-mappings/clients/client-backend", func(http.ResponseWriter, *http.Request) {
		mutations.Add(1)
	})
	mux.HandleFunc("DELETE /admin/realms/eventhub/users/user-1/role-mappings/clients/client-backend", func(http.ResponseWriter, *http.Request) {
		mutations.Add(1)
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	kc := NewKeycloakService(KeycloakClientConfig{
		Host:         server.URL,
		AdminRealm:   "eventhub",
		UserRealm:    "eventhub",
		ClientID:     "backend",
		ClientSecret: "test-secret",
	})
	updated, err := kc.SetUserGlobalRoles(context.Background(), "user-1", []string{"visitor"})
	if !errors.Is(err, ErrLastGlobalAdmin) {
		t.Fatalf("err=%v, want ErrLastGlobalAdmin", err)
	}
	if updated != nil {
		t.Fatalf("updated=%v, want nil", updated)
	}
	if got := mutations.Load(); got != 0 {
		t.Fatalf("unexpected mutation calls=%d", got)
	}
}

func TestSetUserGlobalRolesInvalidatesRejectedToken(t *testing.T) {
	var attempts atomic.Int32
	mux := http.NewServeMux()
	mux.HandleFunc("POST /realms/eventhub/protocol/openid-connect/token", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if _, err := io.Copy(io.Discard, r.Body); err != nil {
			t.Fatalf("read token request: %v", err)
		}
		if _, err := io.WriteString(w, `{"access_token":"test-service-token","expires_in":300}`); err != nil {
			t.Fatalf("write token response: %v", err)
		}
	})
	mux.HandleFunc("GET /admin/realms/eventhub/clients", func(w http.ResponseWriter, r *http.Request) {
		if attempts.Add(1) == 1 {
			http.Error(w, `{"error":"expired token"}`, http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		if _, err := io.WriteString(w, `[{"id":"client-backend","clientId":"backend"}]`); err != nil {
			t.Fatalf("write clients response: %v", err)
		}
	})
	mux.HandleFunc("GET /admin/realms/eventhub/users/{id}/role-mappings/clients/client-backend", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if _, err := io.WriteString(w, `[{"id":"role-admin","name":"admin"}]`); err != nil {
			t.Fatalf("write user roles response: %v", err)
		}
	})
	mux.HandleFunc("GET /admin/realms/eventhub/clients/client-backend/roles", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if _, err := io.WriteString(w, `[{"id":"role-admin","name":"admin"},{"id":"role-moderator","name":"moderator"},{"id":"role-visitor","name":"visitor"}]`); err != nil {
			t.Fatalf("write available roles response: %v", err)
		}
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	kc := NewKeycloakService(KeycloakClientConfig{
		Host:         server.URL,
		AdminRealm:   "eventhub",
		UserRealm:    "eventhub",
		ClientID:     "backend",
		ClientSecret: "test-secret",
	})
	if _, err := kc.SetUserGlobalRoles(context.Background(), "user-1", []string{"admin"}); !isKeycloakStatus(err, http.StatusUnauthorized) {
		t.Fatalf("first call error=%v, want 401", err)
	}
	if updated, err := kc.SetUserGlobalRoles(context.Background(), "user-1", []string{"admin"}); err != nil || !reflect.DeepEqual(updated, []string{"admin"}) {
		t.Fatalf("second call updated=%v err=%v, want admin/ok", updated, err)
	}
	if got := attempts.Load(); got != 2 {
		t.Fatalf("client lookup attempts=%d, want 2", got)
	}
}
