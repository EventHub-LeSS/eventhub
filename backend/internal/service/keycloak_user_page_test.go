package service

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type userPageKeycloak struct {
	mu       sync.Mutex
	logins   int
	clients  int
	requests []string
	login    http.HandlerFunc
	client   http.HandlerFunc
	roles    http.HandlerFunc
}

func writeUserPageResponse(t *testing.T, w http.ResponseWriter, body string) {
	t.Helper()
	if _, err := io.WriteString(w, body); err != nil {
		t.Errorf("write Keycloak test response: %v", err)
	}
}

func (f *userPageKeycloak) service(t *testing.T) *KeycloakService {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("POST /realms/eventhub/protocol/openid-connect/token", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if _, err := io.Copy(io.Discard, r.Body); err != nil {
			t.Errorf("read token request: %v", err)
			return
		}
		f.mu.Lock()
		f.logins++
		f.mu.Unlock()
		if f.login != nil {
			f.login(w, r)
			return
		}
		writeUserPageResponse(t, w, `{"access_token":"test-service-token","expires_in":300}`)
	})
	mux.HandleFunc("GET /admin/realms/eventhub/clients", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		f.mu.Lock()
		f.clients++
		f.mu.Unlock()
		if r.URL.Query().Get("clientId") != "backend" {
			http.Error(w, "wrong client", http.StatusBadRequest)
			return
		}
		if f.client != nil {
			f.client(w, r)
			return
		}
		writeUserPageResponse(t, w, `[{"id":"client-backend","clientId":"backend"}]`)
	})
	mux.HandleFunc("GET /admin/realms/eventhub/users/{id}/role-mappings/clients/client-backend", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		f.mu.Lock()
		f.requests = append(f.requests, r.PathValue("id"))
		f.mu.Unlock()
		if f.roles != nil {
			f.roles(w, r)
			return
		}
		writeUserPageResponse(t, w, `[{"name":"visitor"}]`)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return NewKeycloakService(KeycloakClientConfig{
		Host: srv.URL, AdminRealm: "eventhub", UserRealm: "eventhub",
		ClientID: "backend", ClientSecret: "test-secret",
	})
}

func TestGetUsersGlobalRolesBoundedAndOrdered(t *testing.T) {
	ids := make([]string, 9)
	for i := range ids {
		ids[i] = fmt.Sprintf("page-%d", i)
	}
	started := make(chan struct{}, len(ids)+1)
	release := make(chan struct{})
	secondDone := make(chan struct{})
	var secondOnce sync.Once
	var active, peak atomic.Int32
	fake := &userPageKeycloak{roles: func(w http.ResponseWriter, r *http.Request) {
		n := active.Add(1)
		defer active.Add(-1)
		for old := peak.Load(); n > old; old = peak.Load() {
			if peak.CompareAndSwap(old, n) {
				break
			}
		}
		started <- struct{}{}
		select {
		case <-release:
		case <-r.Context().Done():
			return
		}
		switch r.PathValue("id") {
		case ids[0]:
			select {
			case <-secondDone:
			case <-r.Context().Done():
				return
			}
			writeUserPageResponse(t, w, `[{"name":"visitor"},{"name":"ignored"},{"name":"admin"}]`)
		case ids[1]:
			secondOnce.Do(func() { close(secondDone) })
			writeUserPageResponse(t, w, `[{"name":"moderator"}]`)
		default:
			writeUserPageResponse(t, w, `[]`)
		}
	}}
	kc := fake.service(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	type response struct {
		roles [][]string
		err   error
	}
	done := make(chan response, 1)
	go func() {
		roles, err := kc.GetUsersGlobalRoles(ctx, ids)
		done <- response{roles, err}
	}()
	for range 4 {
		select {
		case <-started:
		case <-time.After(3 * time.Second):
			t.Fatal("four concurrent lookups did not start")
		}
	}
	if peak.Load() != 4 {
		t.Fatalf("concurrency=%d, want 4", peak.Load())
	}
	close(release)
	var got response
	select {
	case got = <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("batch did not finish")
	}
	if got.err != nil || len(got.roles) != len(ids) || peak.Load() > 4 {
		t.Fatalf("roles=%v, err=%v, peak=%d", got.roles, got.err, peak.Load())
	}
	for i, roles := range got.roles {
		want := []string{}
		if i == 0 {
			want = []string{"admin", "visitor"}
		} else if i == 1 {
			want = []string{"moderator"}
		}
		if !reflect.DeepEqual(roles, want) {
			t.Fatalf("roles[%d]=%v, want %v", i, roles, want)
		}
	}
	fake.mu.Lock()
	if fake.logins != 1 || fake.clients != 1 || len(fake.requests) != len(ids) {
		t.Errorf("requests: login=%d clients=%d users=%v", fake.logins, fake.clients, fake.requests)
	}
	seen := make(map[string]int)
	for _, id := range fake.requests {
		seen[id]++
	}
	fake.mu.Unlock()
	for _, id := range ids {
		if seen[id] != 1 {
			t.Fatalf("%s looked up %d times", id, seen[id])
		}
	}
	if _, err := kc.GetUsersGlobalRoles(context.Background(), ids[:1]); err != nil {
		t.Fatal(err)
	}
	fake.mu.Lock()
	defer fake.mu.Unlock()
	if fake.logins != 1 || fake.clients != 2 || len(fake.requests) != 10 {
		t.Fatalf("cache/page calls: %d/%d/%d", fake.logins, fake.clients, len(fake.requests))
	}
}

func TestGetUsersGlobalRolesEmpty(t *testing.T) {
	fake := &userPageKeycloak{}
	roles, err := fake.service(t).GetUsersGlobalRoles(context.Background(), nil)
	if err != nil || roles == nil || len(roles) != 0 {
		t.Fatalf("roles=%v err=%v", roles, err)
	}
	fake.mu.Lock()
	defer fake.mu.Unlock()
	if fake.logins != 0 || fake.clients != 0 || len(fake.requests) != 0 {
		t.Fatal("empty page made external calls")
	}
}

func TestGetUsersGlobalRolesRejectsOnPageFailureAndCancelsWorkers(t *testing.T) {
	started := make(chan struct{}, 4)
	failed := make(chan struct{})
	fake := &userPageKeycloak{roles: func(w http.ResponseWriter, r *http.Request) {
		started <- struct{}{}
		if r.PathValue("id") == "bad" {
			for range 4 {
				select {
				case <-started:
				case <-r.Context().Done():
					return
				}
			}
			http.Error(w, `{"error":"missing user"}`, http.StatusNotFound)
			close(failed)
			return
		}
		<-r.Context().Done()
	}}
	kc := fake.service(t)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	roles, err := kc.GetUsersGlobalRoles(ctx, []string{"bad", "wait-1", "wait-2", "wait-3", "not-started"})
	if roles != nil || !isKeycloakStatus(err, http.StatusNotFound) {
		t.Fatalf("roles=%v err=%v", roles, err)
	}
	select {
	case <-failed:
	default:
		t.Fatal("failure endpoint did not run")
	}
	fake.mu.Lock()
	defer fake.mu.Unlock()
	if len(fake.requests) != 4 {
		t.Fatalf("started extra requests after failure: %v", fake.requests)
	}
}

func TestGetUsersGlobalRolesInvalidatesRejectedToken(t *testing.T) {
	for _, stage := range []string{"client", "roles"} {
		t.Run(stage, func(t *testing.T) {
			var attempts atomic.Int32
			rejectFirst := func(w http.ResponseWriter, r *http.Request) {
				if attempts.Add(1) == 1 {
					http.Error(w, `{"error":"expired token"}`, http.StatusUnauthorized)
					return
				}
				if stage == "client" {
					writeUserPageResponse(t, w, `[{"id":"client-backend","clientId":"backend"}]`)
				} else {
					writeUserPageResponse(t, w, `[{"name":"visitor"}]`)
				}
			}
			fake := &userPageKeycloak{}
			if stage == "client" {
				fake.client = rejectFirst
			} else {
				fake.roles = rejectFirst
			}
			kc := fake.service(t)
			if _, err := kc.GetUsersGlobalRoles(context.Background(), []string{"a"}); !isKeycloakStatus(err, http.StatusUnauthorized) {
				t.Fatalf("expected 401, got %v", err)
			}
			if _, err := kc.GetUsersGlobalRoles(context.Background(), []string{"a"}); err != nil {
				t.Fatal(err)
			}
			fake.mu.Lock()
			defer fake.mu.Unlock()
			if fake.logins != 2 {
				t.Fatalf("logins=%d, token was not refreshed", fake.logins)
			}
		})
	}
}

func TestGetUsersGlobalRolesHonorsParentDeadline(t *testing.T) {
	for _, stage := range []string{"login", "client", "roles"} {
		t.Run(stage, func(t *testing.T) {
			block := func(w http.ResponseWriter, r *http.Request) { <-r.Context().Done() }
			fake := &userPageKeycloak{}
			switch stage {
			case "login":
				fake.login = block
			case "client":
				fake.client = block
			case "roles":
				fake.roles = block
			}
			kc := fake.service(t)
			ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
			defer cancel()
			start := time.Now()
			roles, err := kc.GetUsersGlobalRoles(ctx, []string{"a"})
			if err == nil || roles != nil || time.Since(start) > 3*time.Second {
				t.Fatalf("roles=%v err=%v elapsed=%s", roles, err, time.Since(start))
			}
		})
	}
}

func TestGetUsersGlobalRolesTotalTimeout(t *testing.T) {
	fake := &userPageKeycloak{roles: func(w http.ResponseWriter, r *http.Request) { <-r.Context().Done() }}
	kc := fake.service(t)
	start := time.Now()
	roles, err := kc.GetUsersGlobalRoles(context.Background(), []string{"a"})
	elapsed := time.Since(start)
	if err == nil || roles != nil || elapsed < 14*time.Second || elapsed > 18*time.Second {
		t.Fatalf("roles=%v err=%v elapsed=%s, want 15-second timeout", roles, err, elapsed)
	}
}

func TestGetUsersGlobalRolesCancelsWhileWaitingForLogin(t *testing.T) {
	loginStarted := make(chan struct{})
	fake := &userPageKeycloak{login: func(w http.ResponseWriter, r *http.Request) {
		close(loginStarted)
		<-r.Context().Done()
	}}
	kc := fake.service(t)
	firstCtx, firstCancel := context.WithCancel(context.Background())
	defer firstCancel()
	firstDone := make(chan error, 1)
	go func() {
		_, err := kc.GetUsersGlobalRoles(firstCtx, []string{"a"})
		firstDone <- err
	}()
	select {
	case <-loginStarted:
	case <-time.After(3 * time.Second):
		t.Fatal("login did not start")
	}
	secondCtx, secondCancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer secondCancel()
	if _, err := kc.GetUsersGlobalRoles(secondCtx, []string{"b"}); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("waiting for login did not honor deadline: %v", err)
	}
	firstCancel()
	select {
	case err := <-firstDone:
		if err == nil {
			t.Fatal("canceled login unexpectedly succeeded")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("first request did not cancel")
	}
}

func TestGetUsersGlobalRolesDoesNotRequestOffPageUsers(t *testing.T) {
	fake := &userPageKeycloak{roles: func(w http.ResponseWriter, r *http.Request) {
		if r.PathValue("id") != "on-page" {
			http.Error(w, `{"error":"off-page user is broken"}`, http.StatusNotFound)
			return
		}
		writeUserPageResponse(t, w, `[{"name":"visitor"}]`)
	}}
	roles, err := fake.service(t).GetUsersGlobalRoles(context.Background(), []string{"on-page"})
	if err != nil || !reflect.DeepEqual(roles, [][]string{{"visitor"}}) {
		t.Fatalf("roles=%v err=%v", roles, err)
	}
}
