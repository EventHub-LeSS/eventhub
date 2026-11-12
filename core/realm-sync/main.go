// realm-sync applies the EventHub realm file (eventhub-realm.json) to a RUNNING
// Keycloak server non-destructively.
//
// Why it exists: Keycloak's two built-in import paths are unsuitable for
// continuous configuration management.
//
//   - `start --import-realm` skips realms that already exist, so changes to the
//     realm file never reach an initialized database.
//   - `kc.sh import --override true` replaces the whole realm: everything that
//     is not in the file (organizations created at runtime, self-registered
//     users, changed passwords, ...) is deleted.
//
// This tool instead applies exactly the resources that are configured in the
// file and leaves everything else alone:
//
//   - realm settings: only the keys present in the file are overlaid
//     (read-modify-write, so server-managed fields survive).
//   - clients, client scopes, client roles: upsert in place, preserving their
//     existing IDs (and therefore service accounts, secrets, grants).
//   - users: created if missing (with the file's credentials); existing users
//     get their profile fields updated, but passwords are never touched.
//   - organizations: upserted by alias; file members are added if missing.
//     Organizations and members NOT in the file are never deleted.
//
// The sync runs on every Keycloak container start (see entrypoint script) and
// is idempotent: applying the same file twice changes nothing the second time.
package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strings"
	"time"
)

const (
	defaultRealmFile   = "/opt/keycloak/data/import/eventhub-realm.json"
	defaultSyncTimeout = 10 * time.Minute
	pollInterval       = 3 * time.Second
	pageSize           = 100
)

type config struct {
	host        string // e.g. http://localhost:8080
	realm       string // target realm, e.g. eventhub
	adminUser   string // bootstrap admin in the master realm
	adminPass   string
	realmFile   string
	syncTimeout time.Duration
}

// adminClient is a tiny Keycloak Admin API client with password-grant login
// against the master realm and a single re-login retry on 401.
type adminClient struct {
	cfg   config
	http  *http.Client
	token string
}

func main() {
	log.SetPrefix("[realm-sync] ")
	log.SetFlags(0)

	cfg := config{
		host:        strings.TrimRight(envOr("KEYCLOAK_HOST", "http://localhost:8080"), "/"),
		realm:       envOr("KEYCLOAK_REALM", "eventhub"),
		adminUser:   envOr("KEYCLOAK_ADMIN_USERNAME", "admin"),
		adminPass:   envOr("KEYCLOAK_ADMIN_PASSWORD", "admin"),
		realmFile:   envOr("REALM_IMPORT_FILE", defaultRealmFile),
		syncTimeout: defaultSyncTimeout,
	}

	raw, err := os.ReadFile(cfg.realmFile)
	if err != nil {
		log.Fatalf("FATAL: cannot read realm file %s: %v", cfg.realmFile, err)
	}
	file, err := parseRealmFile(raw)
	if err != nil {
		log.Fatalf("FATAL: cannot parse realm file %s: %v", cfg.realmFile, err)
	}
	if file.Realm != "" && file.Realm != cfg.realm {
		log.Printf("note: realm file targets realm %q, syncing realm %q as configured", file.Realm, cfg.realm)
	}

	admin := &adminClient{cfg: cfg, http: &http.Client{Timeout: 30 * time.Second}}

	if err := waitForServer(admin); err != nil {
		log.Fatalf("FATAL: Keycloak did not become ready: %v", err)
	}
	log.Printf("Keycloak is up, applying realm file %s to realm %q", cfg.realmFile, cfg.realm)

	// On the very first start the bootstrap admin is created while the server
	// is already answering; retry the login until the deadline.
	if err := retry(admin.cfg.syncTimeout, func() error { return admin.login() }); err != nil {
		log.Fatalf("FATAL: admin login failed: %v", err)
	}

	// Apply every section independently: a failure in one section must not
	// prevent the others from being applied. Any failure is reported at the end.
	var errs []error

	if err := applyRealmSettings(admin, file); err != nil {
		errs = append(errs, fmt.Errorf("realm settings: %w", err))
	}

	scopeIDs, err := ensureClientScopes(admin, file)
	if err != nil {
		errs = append(errs, fmt.Errorf("client scopes: %w", err))
		scopeIDs = map[string]string{}
	}

	clientIDs, err := ensureClients(admin, file, scopeIDs)
	if err != nil {
		errs = append(errs, fmt.Errorf("clients: %w", err))
		clientIDs = map[string]string{}
	}

	roles := &roleResolver{admin: admin, clientUUIDs: clientIDs, roleIDs: map[string]string{}}
	if err := ensureClientRoles(admin, file, roles); err != nil {
		errs = append(errs, fmt.Errorf("client roles: %w", err))
	}

	if err := ensureScopeMappings(admin, file, roles); err != nil {
		errs = append(errs, fmt.Errorf("scope mappings: %w", err))
	}

	userIDs, err := ensureUsers(admin, file, roles)
	if err != nil {
		errs = append(errs, fmt.Errorf("users: %w", err))
		userIDs = map[string]string{}
	}

	if err := ensureOrganizations(admin, file, userIDs); err != nil {
		errs = append(errs, fmt.Errorf("organizations: %w", err))
	}

	if len(errs) > 0 {
		for _, err := range errs {
			log.Printf("ERROR: %v", err)
		}
		log.Fatalf("FATAL: realm sync finished with %d error(s)", len(errs))
	}
	log.Println("realm sync finished successfully (non-destructive; resources not in the file were preserved)")
}

// ---------------------------------------------------------------------------
// realm file parsing
// ---------------------------------------------------------------------------

type realmFile struct {
	Realm         string                         `json:"realm"`
	settings      map[string]any                 // top-level keys of the raw file
	scopes        []map[string]any               // raw client scope representations
	clients       []map[string]any               // raw client representations
	users         []map[string]any               // raw user representations
	orgs          []map[string]any               // raw organization representations
	clientRoles   map[string][]map[string]any    // clientId -> role representations
	scopeMappings map[string][]scopeMappingEntry // container (client/scope) -> entries
}

type scopeMappingEntry struct {
	Client string   `json:"client"` // client receiving the mapped roles
	Roles  []string `json:"roles"`  // client roles of the mapping container (the map key)
}

func parseRealmFile(raw []byte) (*realmFile, error) {
	file := &realmFile{settings: map[string]any{}}
	if err := json.Unmarshal(raw, &file.settings); err != nil {
		return nil, err
	}
	if err := json.Unmarshal(raw, file); err != nil {
		return nil, err
	}
	// Sections are kept as raw maps so that any representation field Keycloak
	// understands passes through unchanged on create, while updates overlay
	// only the keys the file actually configures.
	for _, key := range []string{"clientScopes", "clients", "users", "organizations"} {
		list, _ := file.settings[key].([]any)
		for _, item := range list {
			m, ok := item.(map[string]any)
			if !ok {
				continue
			}
			switch key {
			case "clientScopes":
				file.scopes = append(file.scopes, m)
			case "clients":
				file.clients = append(file.clients, m)
			case "users":
				file.users = append(file.users, m)
			case "organizations":
				file.orgs = append(file.orgs, m)
			}
		}
	}
	if roles, ok := file.settings["roles"].(map[string]any); ok {
		file.clientRoles = map[string][]map[string]any{}
		if perClient, ok := roles["client"].(map[string]any); ok {
			for clientID, rawRoles := range perClient {
				roleList, _ := rawRoles.([]any)
				for _, rawRole := range roleList {
					if role, ok := rawRole.(map[string]any); ok {
						file.clientRoles[clientID] = append(file.clientRoles[clientID], role)
					}
				}
			}
		}
	}
	if mappings, ok := file.settings["clientScopeMappings"].(map[string]any); ok {
		file.scopeMappings = map[string][]scopeMappingEntry{}
		for container, rawEntries := range mappings {
			entryList, _ := rawEntries.([]any)
			for _, rawEntry := range entryList {
				entryJSON, err := json.Marshal(rawEntry)
				if err != nil {
					continue
				}
				var entry scopeMappingEntry
				if err := json.Unmarshal(entryJSON, &entry); err != nil {
					continue
				}
				file.scopeMappings[container] = append(file.scopeMappings[container], entry)
			}
		}
	}
	return file, nil
}

// ---------------------------------------------------------------------------
// HTTP helpers
// ---------------------------------------------------------------------------

func (a *adminClient) login() error {
	form := url.Values{
		"grant_type": {"password"},
		"client_id":  {"admin-cli"},
		"username":   {a.cfg.adminUser},
		"password":   {a.cfg.adminPass},
	}
	resp, err := a.http.PostForm(a.cfg.host+"/realms/master/protocol/openid-connect/token", form)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("login answered HTTP %d: %.200s", resp.StatusCode, body)
	}
	var token struct {
		AccessToken string `json:"access_token"`
	}
	if err := json.Unmarshal(body, &token); err != nil || token.AccessToken == "" {
		return fmt.Errorf("login returned no access token: %.200s", body)
	}
	a.token = token.AccessToken
	return nil
}

// result carries what the callers need from an admin request.
type result struct {
	status  int
	headers http.Header
	body    []byte
}

// do performs an authenticated admin request. On 401 it re-logs-in once (the
// token may have expired) and retries the request once.
func (a *adminClient) do(method, path string, body any) (result, error) {
	res, err := a.attempt(method, path, body)
	if err == nil && res.status == http.StatusUnauthorized {
		if loginErr := a.login(); loginErr == nil {
			res, err = a.attempt(method, path, body)
		}
	}
	return res, err
}

func (a *adminClient) attempt(method, path string, body any) (result, error) {
	var reader io.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			return result{}, err
		}
		reader = strings.NewReader(string(encoded))
	}
	req, err := http.NewRequest(method, a.cfg.host+path, reader)
	if err != nil {
		return result{}, err
	}
	req.Header.Set("Authorization", "Bearer "+a.token)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := a.http.Do(req)
	if err != nil {
		return result{}, err
	}
	defer resp.Body.Close()
	resBody, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return result{status: resp.StatusCode, headers: resp.Header}, err
	}
	return result{status: resp.StatusCode, headers: resp.Header, body: resBody}, nil
}

// getJSON fetches a resource; a missing resource (404) returns ok=false
// without error, so callers can distinguish "absent" from "broken".
func (a *adminClient) getJSON(path string, target any) (ok bool, err error) {
	res, err := a.do(http.MethodGet, path, nil)
	if err != nil {
		return false, err
	}
	if res.status == http.StatusNotFound {
		return false, nil
	}
	if res.status >= 300 {
		return false, fmt.Errorf("GET %s answered HTTP %d: %.200s", path, res.status, res.body)
	}
	if target == nil {
		return true, nil
	}
	if err := json.Unmarshal(res.body, target); err != nil {
		return false, fmt.Errorf("GET %s returned unexpected body: %w", path, err)
	}
	return true, nil
}

// create POSTs a resource and returns the UUID Keycloak appends to the
// Location header of successful creates.
func (a *adminClient) create(path string, body any) (string, error) {
	res, err := a.do(http.MethodPost, path, body)
	if err != nil {
		return "", err
	}
	if res.status != http.StatusCreated && res.status != http.StatusOK {
		return "", fmt.Errorf("POST %s answered HTTP %d: %.200s", path, res.status, res.body)
	}
	location := res.headers.Get("Location")
	if location == "" {
		return "", nil
	}
	if idx := strings.LastIndex(location, "/"); idx >= 0 {
		return location[idx+1:], nil
	}
	return location, nil
}

func (a *adminClient) update(method, path string, body any) error {
	res, err := a.do(method, path, body)
	if err != nil {
		return err
	}
	if res.status >= 300 {
		return fmt.Errorf("%s %s answered HTTP %d: %.200s", method, path, res.status, res.body)
	}
	return nil
}

// waitForServer polls until the server answers for the target realm, whether
// the realm already exists (200) or not (404). Anything else means "not ready".
func waitForServer(a *adminClient) error {
	return retry(a.cfg.syncTimeout, func() error {
		res, err := a.attempt(http.MethodGet, "/realms/"+url.PathEscape(a.cfg.realm), nil)
		if err != nil {
			return err
		}
		if res.status == http.StatusOK || res.status == http.StatusNotFound {
			return nil
		}
		return fmt.Errorf("HTTP %d: %.200s", res.status, res.body)
	})
}

// retry runs attempt until it succeeds or the deadline passes, sleeping
// between tries.
func retry(timeout time.Duration, attempt func() error) error {
	deadline := time.Now().Add(timeout)
	for {
		err := attempt()
		if err == nil {
			return nil
		}
		if time.Now().After(deadline) {
			return err
		}
		time.Sleep(pollInterval)
	}
}

// ---------------------------------------------------------------------------
// 1. realm settings: overlay the file's settings onto the existing realm
// ---------------------------------------------------------------------------

// settingKeys are the realm settings this project manages in the realm file.
// Only these keys are copied onto the existing realm; anything else the server
// or an operator manages at runtime is preserved.
var settingKeys = []string{
	"enabled", "displayName", "loginTheme", "organizationsEnabled",
	"registrationAllowed", "registrationEmailAsUsername", "loginWithEmailAllowed",
	"duplicateEmailsAllowed", "verifyEmail", "resetPasswordAllowed", "rememberMe",
	"sslRequired", "accessTokenLifespan", "ssoSessionIdleTimeout", "ssoSessionMaxLifespan",
	"smtpServer",
}

func applyRealmSettings(a *adminClient, file *realmFile) error {
	path := "/admin/realms/" + url.PathEscape(a.cfg.realm)
	var current map[string]any
	ok, err := a.getJSON(path, &current)
	if err != nil {
		return err
	}
	if !ok {
		// The realm does not exist yet (fresh database without --import-realm):
		// create it from the complete file, which gives Keycloak's own import
		// semantics. The upsert passes below fill in anything it skipped.
		log.Printf("realm %q does not exist, importing the complete file", a.cfg.realm)
		return a.update(http.MethodPost, "/admin/realms", file.settings)
	}

	applied := []string{}
	for _, key := range settingKeys {
		value, present := file.settings[key]
		if !present || equalJSON(current[key], value) {
			continue
		}
		current[key] = value
		applied = append(applied, key)
	}
	if len(applied) == 0 {
		log.Println("settings: up to date")
		return nil
	}
	if err := a.update(http.MethodPut, path, current); err != nil {
		return err
	}
	log.Printf("settings: applied %d change(s): %s", len(applied), strings.Join(applied, ", "))
	return nil
}

// ---------------------------------------------------------------------------
// 2. client scopes: upsert by name
// ---------------------------------------------------------------------------

func (a *adminClient) listClientScopes() ([]map[string]any, error) {
	var scopes []map[string]any
	_, err := a.getJSON("/admin/realms/"+url.PathEscape(a.cfg.realm)+"/client-scopes", &scopes)
	return scopes, err
}

func ensureClientScopes(a *adminClient, file *realmFile) (map[string]string, error) {
	ids := map[string]string{}
	for _, scope := range file.scopes {
		name, _ := scope["name"].(string)
		if name == "" {
			continue
		}
		existingID, err := a.findScopeID(name)
		if err != nil {
			return nil, err
		}
		path := "/admin/realms/" + url.PathEscape(a.cfg.realm) + "/client-scopes"
		if existingID == "" {
			id, err := a.create(path, scope)
			if err != nil {
				return nil, fmt.Errorf("create client scope %q: %w", name, err)
			}
			log.Printf("client scope %q: created", name)
			if id == "" {
				if id, err = a.findScopeID(name); err != nil {
					return nil, err
				}
			}
			ids[name] = id
			continue
		}
		// Read-modify-write so the overlay only touches file-managed keys;
		// server-added fields (and attribute subkeys) survive.
		path = path + "/" + url.PathEscape(existingID)
		var current map[string]any
		if _, err := a.getJSON(path, &current); err != nil {
			return nil, fmt.Errorf("read client scope %q: %w", name, err)
		}
		changed := overlayObject(current, scope)
		// protocolMappers is a list of server-enriched objects (Keycloak adds
		// an "id" per mapper), so it is compared semantically: every mapper
		// from the file must exist by name with the same configuration.
		if fileMappers, hasMappers := scope["protocolMappers"].([]any); hasMappers {
			if !mappersUpToDate(current, fileMappers) {
				// Handing the file's mappers to the update creates or updates
				// them; mappers added at runtime are not removed.
				current["protocolMappers"] = fileMappers
				changed = true
			}
		}
		if changed {
			if err := a.update(http.MethodPut, path, current); err != nil {
				return nil, fmt.Errorf("update client scope %q: %w", name, err)
			}
			log.Printf("client scope %q: updated", name)
		} else {
			log.Printf("client scope %q: up to date", name)
		}
		ids[name] = existingID
	}
	return ids, nil
}

// mappersUpToDate reports whether every file mapper exists in the current
// mapper list (matched by name) with an identical configuration.
func mappersUpToDate(current map[string]any, fileMappers []any) bool {
	currentMappers, _ := current["protocolMappers"].([]any)
	byName := map[string]any{}
	for _, rawMapper := range currentMappers {
		mapper, _ := rawMapper.(map[string]any)
		if name, _ := mapper["name"].(string); name != "" {
			byName[name] = mapper
		}
	}
	for _, rawMapper := range fileMappers {
		mapper, ok := rawMapper.(map[string]any)
		if !ok {
			continue
		}
		name, _ := mapper["name"].(string)
		existing, ok := byName[name]
		if !ok {
			return false
		}
		existingMapper, _ := existing.(map[string]any)
		// Mapper scalars must match exactly; the config is compared as a
		// subset, because Keycloak stores defaults for config keys the file
		// does not mention (e.g. userinfo.token.claim of the audience mapper).
		for key, value := range mapper {
			if key == "config" {
				continue
			}
			if key == "id" {
				continue // server-assigned
			}
			if !equalJSON(existingMapper[key], value) {
				return false
			}
		}
		fileConfig, _ := mapper["config"].(map[string]any)
		currentConfig, _ := existingMapper["config"].(map[string]any)
		for configKey, configValue := range fileConfig {
			if !equalJSON(currentConfig[configKey], configValue) {
				return false
			}
		}
	}
	return true
}

// ---------------------------------------------------------------------------
// 3. clients: upsert by clientId, preserving IDs, secrets and service accounts
// ---------------------------------------------------------------------------

// overlayKeys are representation keys the sync never overlays from the file:
// identity fields ("id", "username", "clientId"), credential material and
// helper fields that are not part of Keycloak's representations. Secret is
// write-only (GET never returns it) and compared via its own endpoint.
// defaultClientScopes/optionalClientScopes are managed additively by their
// dedicated endpoints, never by the update PUT. Everything else the file
// configures is overlaid onto the current representation.
var overlaySkipKeys = map[string]bool{
	"id": true, "clientId": true, "username": true, "secret": true,
	"credentials": true, "clientRoles": true, "serviceAccountClientId": true,
	"members": true, "defaultClientScopes": true, "optionalClientScopes": true,
	// protocolMappers is a list of server-enriched objects compared by
	// mappersUpToDate, not by the generic object overlay.
	"protocolMappers": true,
}

// overlayObject copies the keys the file configures onto current and reports
// whether anything changed. A wanted key that is itself an object (e.g.
// "attributes") is merged per subkey: Keycloak's GET responses enrich objects
// with server-managed subkeys that must neither count as drift (otherwise the
// sync would rewrite the resource on every start) nor be wiped by the PUT.
func overlayObject(current, wanted map[string]any) bool {
	changed := false
	for key, value := range wanted {
		if overlaySkipKeys[key] {
			continue
		}
		currentValue, inCurrent := current[key]
		if !inCurrent && isZeroValue(value) {
			// Keycloak omits default/zero fields from its representations.
			// There is nothing to apply, and writing them back would keep
			// them absent, so they must not count as drift.
			continue
		}
		if sub, isObject := value.(map[string]any); isObject {
			currentSub, _ := currentValue.(map[string]any)
			if currentSub == nil {
				currentSub = map[string]any{}
			}
			subChanged := false
			for subKey, subValue := range sub {
				if equalJSON(currentSub[subKey], subValue) {
					continue
				}
				currentSub[subKey] = subValue
				subChanged = true
			}
			if subChanged {
				current[key] = currentSub
				changed = true
			}
			continue
		}
		if equalJSON(currentValue, value) {
			continue
		}
		current[key] = value
		changed = true
	}
	return changed
}

// isZeroValue reports whether value is the zero value of its type (false, "",
// 0, an empty array or object). Such values are Keycloak defaults that its
// representations omit, so they never need to be written.
func isZeroValue(value any) bool {
	switch typed := value.(type) {
	case nil:
		return true
	case bool:
		return !typed
	case string:
		return typed == ""
	case float64:
		return typed == 0
	case []any:
		return len(typed) == 0
	case map[string]any:
		return len(typed) == 0
	default:
		return false
	}
}

func ensureClients(a *adminClient, file *realmFile, scopeIDs map[string]string) (map[string]string, error) {
	ids := map[string]string{}
	for _, client := range file.clients {
		clientID, _ := client["clientId"].(string)
		if clientID == "" {
			continue
		}
		uuid, err := a.findClientID(clientID)
		if err != nil {
			return nil, err
		}
		base := "/admin/realms/" + url.PathEscape(a.cfg.realm) + "/clients"
		if uuid == "" {
			uuid, err = a.create(base, client)
			if err != nil {
				return nil, fmt.Errorf("create client %q: %w", clientID, err)
			}
			log.Printf("client %q: created", clientID)
		} else {
			// Read-modify-write so the overlay only touches file-managed keys.
			var current map[string]any
			if _, err := a.getJSON(base+"/"+url.PathEscape(uuid), &current); err != nil {
				return nil, fmt.Errorf("read client %q: %w", clientID, err)
			}
			changed := overlayObject(current, client)
			// The client secret is write-only: Keycloak's client GET omits it,
			// so drift is detected via the dedicated secret endpoint.
			if secret, present := client["secret"].(string); present {
				currentSecret, err := a.clientSecret(uuid)
				if err != nil {
					return nil, fmt.Errorf("read client %q secret: %w", clientID, err)
				}
				if currentSecret != secret {
					current["secret"] = secret
					changed = true
				}
			}
			if changed {
				if err := a.update(http.MethodPut, base+"/"+url.PathEscape(uuid), current); err != nil {
					return nil, fmt.Errorf("update client %q: %w", clientID, err)
				}
				log.Printf("client %q: updated", clientID)
			} else {
				log.Printf("client %q: up to date", clientID)
			}
		}
		ids[clientID] = uuid

		if err := ensureClientAssignedScopes(a, clientID, uuid, client, scopeIDs); err != nil {
			return nil, err
		}
	}
	return ids, nil
}

// ensureClientAssignedScopes applies a client's defaultClientScopes and
// optionalClientScopes from the file, additively.
func ensureClientAssignedScopes(a *adminClient, clientID, uuid string, client map[string]any, scopeIDs map[string]string) error {
	for _, assign := range []struct {
		fileKey  string
		endpoint string
	}{
		{"defaultClientScopes", "default-client-scopes"},
		{"optionalClientScopes", "optional-client-scopes"},
	} {
		wanted, _ := client[assign.fileKey].([]any)
		if len(wanted) == 0 {
			continue
		}
		base := "/admin/realms/" + url.PathEscape(a.cfg.realm) + "/clients/" + url.PathEscape(uuid) + "/" + assign.endpoint
		var attached []map[string]any
		if _, err := a.getJSON(base, &attached); err != nil {
			return fmt.Errorf("client %q: list %s: %w", clientID, assign.endpoint, err)
		}
		attachedNames := map[string]bool{}
		for _, scope := range attached {
			if name, _ := scope["name"].(string); name != "" {
				attachedNames[name] = true
			}
		}
		for _, rawName := range wanted {
			name, _ := rawName.(string)
			if name == "" || attachedNames[name] {
				continue
			}
			scopeID := scopeIDs[name]
			if scopeID == "" {
				// Not one of the file's own scopes: could be a built-in, resolve it.
				id, err := a.findScopeID(name)
				if err != nil {
					return err
				}
				if id == "" {
					return fmt.Errorf("client %q: scope %q is neither in the file nor in the realm", clientID, name)
				}
				scopeID = id
			}
			if err := a.update(http.MethodPut, base+"/"+url.PathEscape(scopeID), nil); err != nil {
				return fmt.Errorf("client %q: attach %s scope %q: %w", clientID, assign.endpoint, name, err)
			}
			log.Printf("client %q: attached %s scope %q", clientID, assign.endpoint, name)
		}
	}
	return nil
}

// clientSecret reads the current secret of a client via the dedicated
// endpoint, because the client representation omits it.
func (a *adminClient) clientSecret(uuid string) (string, error) {
	path := "/admin/realms/" + url.PathEscape(a.cfg.realm) + "/clients/" + url.PathEscape(uuid) + "/client-secret"
	var secret struct {
		Value string `json:"value"`
	}
	if _, err := a.getJSON(path, &secret); err != nil {
		return "", err
	}
	return secret.Value, nil
}

func (a *adminClient) findClientID(clientID string) (string, error) {
	path := "/admin/realms/" + url.PathEscape(a.cfg.realm) + "/clients?clientId=" + url.QueryEscape(clientID) + "&exact=true"
	var clients []map[string]any
	if _, err := a.getJSON(path, &clients); err != nil {
		return "", err
	}
	for _, client := range clients {
		if id, _ := client["clientId"].(string); id == clientID {
			id, _ := client["id"].(string)
			return id, nil
		}
	}
	return "", nil
}

func (a *adminClient) findScopeID(name string) (string, error) {
	scopes, err := a.listClientScopes()
	if err != nil {
		return "", err
	}
	for _, scope := range scopes {
		if scopeName, _ := scope["name"].(string); scopeName == name {
			id, _ := scope["id"].(string)
			return id, nil
		}
	}
	return "", nil
}

// ---------------------------------------------------------------------------
// 4. client roles: create or update in place, add missing composites
// ---------------------------------------------------------------------------

// roleResolver resolves clients and their client roles by name and caches the
// results; the same roles are needed for composites, scope mappings and user
// role assignments.
type roleResolver struct {
	admin       *adminClient
	clientUUIDs map[string]string
	roleIDs     map[string]string
}

func (r *roleResolver) clientUUID(clientID string) (string, error) {
	if uuid, ok := r.clientUUIDs[clientID]; ok && uuid != "" {
		return uuid, nil
	}
	uuid, err := r.admin.findClientID(clientID)
	if err != nil {
		return "", err
	}
	if uuid == "" {
		return "", fmt.Errorf("client %q not found", clientID)
	}
	r.clientUUIDs[clientID] = uuid
	return uuid, nil
}

func (r *roleResolver) roleID(clientID, roleName string) (string, error) {
	cacheKey := clientID + "/" + roleName
	if id, ok := r.roleIDs[cacheKey]; ok {
		return id, nil
	}
	uuid, err := r.clientUUID(clientID)
	if err != nil {
		return "", fmt.Errorf("role %q: %w", roleName, err)
	}
	path := "/admin/realms/" + url.PathEscape(r.admin.cfg.realm) + "/clients/" + url.PathEscape(uuid) + "/roles/" + url.PathEscape(roleName)
	var role struct {
		ID string `json:"id"`
	}
	ok, err := r.admin.getJSON(path, &role)
	if err != nil || !ok || role.ID == "" {
		return "", fmt.Errorf("role %q of client %q not found", roleName, clientID)
	}
	r.roleIDs[cacheKey] = role.ID
	return role.ID, nil
}

func ensureClientRoles(a *adminClient, file *realmFile, roles *roleResolver) error {
	for clientID, wanted := range file.clientRoles {
		for _, role := range wanted {
			if err := ensureClientRole(a, clientID, role, roles); err != nil {
				return err
			}
		}
	}
	return nil
}

func ensureClientRole(a *adminClient, clientID string, role map[string]any, roles *roleResolver) error {
	roleName, _ := role["name"].(string)
	if roleName == "" {
		return nil
	}
	uuid, err := roles.clientUUID(clientID)
	if err != nil {
		return fmt.Errorf("client role %q: %w", roleName, err)
	}
	base := "/admin/realms/" + url.PathEscape(a.cfg.realm) + "/clients/" + url.PathEscape(uuid)
	rolePath := base + "/roles/" + url.PathEscape(roleName)

	var current map[string]any
	ok, err := a.getJSON(rolePath, &current)
	if err != nil {
		return fmt.Errorf("client role %q of %q: %w", roleName, clientID, err)
	}
	if !ok {
		if _, err := a.create(base+"/roles", role); err != nil {
			return fmt.Errorf("create client role %q of %q: %w", roleName, clientID, err)
		}
		log.Printf("client role %q of %q: created", roleName, clientID)
	} else {
		changed := false
		if description, present := role["description"].(string); present && current["description"] != description {
			current["description"] = description
			changed = true
		}
		if changed {
			if err := a.update(http.MethodPut, rolePath, current); err != nil {
				return fmt.Errorf("update client role %q of %q: %w", roleName, clientID, err)
			}
			log.Printf("client role %q of %q: updated", roleName, clientID)
		} else {
			log.Printf("client role %q of %q: up to date", roleName, clientID)
		}
	}

	// Composites are added additively: whatever was composed at runtime is
	// never removed, whatever the file configures is ensured.
	if composites, present := role["composites"].(map[string]any); present {
		if err := ensureComposites(a, clientID, roleName, composites, roles); err != nil {
			return err
		}
	}
	return nil
}

func ensureComposites(a *adminClient, clientID, roleName string, composites map[string]any, roles *roleResolver) error {
	perClient, _ := composites["client"].(map[string]any)
	uuid, err := roles.clientUUID(clientID)
	if err != nil {
		return fmt.Errorf("composites of role %q: %w", roleName, err)
	}
	path := "/admin/realms/" + url.PathEscape(a.cfg.realm) + "/clients/" + url.PathEscape(uuid) + "/roles/" + url.PathEscape(roleName) + "/composites"
	var current []map[string]any
	if _, err := a.getJSON(path, &current); err != nil {
		return fmt.Errorf("composites of role %q of %q: %w", roleName, clientID, err)
	}
	currentIDs := map[string]bool{}
	for _, composite := range current {
		if id, _ := composite["id"].(string); id != "" {
			currentIDs[id] = true
		}
	}
	var missing []map[string]any
	for compositeClient, rawRoles := range perClient {
		roleNames, _ := rawRoles.([]any)
		for _, rawName := range roleNames {
			compositeRole, _ := rawName.(string)
			id, err := roles.roleID(compositeClient, compositeRole)
			if err != nil {
				return fmt.Errorf("composite of role %q of %q: %w", roleName, clientID, err)
			}
			if currentIDs[id] {
				continue
			}
			missing = append(missing, map[string]any{"id": id, "name": compositeRole})
		}
	}
	if len(missing) == 0 {
		return nil
	}
	if err := a.update(http.MethodPost, path, missing); err != nil {
		return fmt.Errorf("add composites to role %q of %q: %w", roleName, clientID, err)
	}
	log.Printf("client role %q of %q: added %d composite(s)", roleName, clientID, len(missing))
	return nil
}

// ---------------------------------------------------------------------------
// 5. clientScopeMappings: additive scope mappings between clients/scopes
// ---------------------------------------------------------------------------

// ensureScopeMappings applies the file's clientScopeMappings. A mapping key
// names the client whose roles are mapped (the role container); each entry
// lists a target client that receives those roles in its scope mappings.
// Keycloak's API direction is: the receiver owns the endpoint, the role
// container is named last, e.g. /clients/{receiver}/scope-mappings/clients/{container}.
// Mappings are added additively and never removed.
func ensureScopeMappings(a *adminClient, file *realmFile, roles *roleResolver) error {
	realm := url.PathEscape(a.cfg.realm)
	for container, entries := range file.scopeMappings {
		containerUUID, err := roles.clientUUID(container)
		if err != nil {
			log.Printf("scope mapping container %q: not a client in the realm, skipping (%v)", container, err)
			continue
		}
		for _, entry := range entries {
			receiverUUID, err := roles.clientUUID(entry.Client)
			if err != nil {
				log.Printf("scope mapping container %q: target %v, skipping", container, err)
				continue
			}
			mappingPath := "/admin/realms/" + realm + "/clients/" + url.PathEscape(receiverUUID) + "/scope-mappings/clients/" + url.PathEscape(containerUUID)

			var current []map[string]any
			if _, err := a.getJSON(mappingPath, &current); err != nil {
				return fmt.Errorf("scope mappings of container %q for client %q: %w", container, entry.Client, err)
			}
			currentIDs := map[string]bool{}
			for _, mapping := range current {
				if id, _ := mapping["id"].(string); id != "" {
					currentIDs[id] = true
				}
			}
			var missing []map[string]any
			for _, roleName := range entry.Roles {
				id, err := roles.roleID(container, roleName)
				if err != nil {
					log.Printf("scope mapping container %q: %v, skipping role", container, err)
					continue
				}
				if currentIDs[id] {
					continue
				}
				missing = append(missing, map[string]any{"id": id, "name": roleName})
			}
			if len(missing) == 0 {
				continue
			}
			if err := a.update(http.MethodPost, mappingPath, missing); err != nil {
				return fmt.Errorf("add scope mappings of container %q for client %q: %w", container, entry.Client, err)
			}
			log.Printf("scope mappings of container %q for client %q: added %d role(s)", container, entry.Client, len(missing))
		}
	}
	return nil
}

// ---------------------------------------------------------------------------
// 6. users: create if missing, update profile in place, never touch passwords
// ---------------------------------------------------------------------------

func ensureUsers(a *adminClient, file *realmFile, roles *roleResolver) (map[string]string, error) {
	ids := map[string]string{}
	base := "/admin/realms/" + url.PathEscape(a.cfg.realm) + "/users"
	for _, user := range file.users {
		username, _ := user["username"].(string)
		email, _ := user["email"].(string)
		if username == "" {
			continue
		}
		if _, isServiceAccount := user["serviceAccountClientId"]; isServiceAccount {
			// The shadow user of a service-account client is managed by the
			// client itself: creating it here would produce a regular user.
			// Its role grants from the file are still applied if the shadow
			// user exists (the client upsert keeps it alive).
			userID, err := a.findUserID(username, email)
			if err != nil {
				return nil, err
			}
			if userID == "" {
				log.Printf("user %q: service account shadow user missing, it is recreated via the %q client's serviceAccountsEnabled; run 'docker compose up api-seed' if its role grants need repair", username, user["serviceAccountClientId"])
				continue
			}
			if err := addUserClientRoles(a, userID, user, roles); err != nil {
				return nil, err
			}
			continue
		}
		userID, err := a.findUserID(username, email)
		if err != nil {
			return nil, err
		}
		if userID == "" {
			if _, err := a.create(base, user); err != nil {
				return nil, fmt.Errorf("create user %q: %w", username, err)
			}
			log.Printf("user %q: created", username)
			userID, err = a.findUserID(username, email)
			if err != nil {
				return nil, err
			}
		} else {
			// Existing users keep their credentials (e.g. changed passwords)
			// and their Keycloak user ID; only the profile fields from the
			// file are overlaid.
			path := base + "/" + url.PathEscape(userID)
			var current map[string]any
			if _, err := a.getJSON(path, &current); err != nil {
				return nil, fmt.Errorf("read user %q: %w", username, err)
			}
			if overlayObject(current, user) {
				if err := a.update(http.MethodPut, path, current); err != nil {
					return nil, fmt.Errorf("update user %q: %w", username, err)
				}
				log.Printf("user %q: updated", username)
			} else {
				log.Printf("user %q: up to date", username)
			}
		}
		if userID == "" {
			return nil, fmt.Errorf("user %q: could not resolve ID after ensuring", username)
		}
		ids[username] = userID

		// Client roles from the file are added additively, so role grants made
		// at runtime are preserved.
		if err := addUserClientRoles(a, userID, user, roles); err != nil {
			return nil, err
		}
	}
	return ids, nil
}

// addUserClientRoles applies a file user's clientRoles additively: roles the
// file configures are granted if missing, role grants made at runtime are
// never revoked.
func addUserClientRoles(a *adminClient, userID string, user map[string]any, roles *roleResolver) error {
	rawRoles, present := user["clientRoles"].(map[string]any)
	if !present {
		return nil
	}
	username, _ := user["username"].(string)
	base := "/admin/realms/" + url.PathEscape(a.cfg.realm) + "/users"
	for clientID, rawRoleNames := range rawRoles {
		roleNames, _ := rawRoleNames.([]any)
		clientUUID, err := roles.clientUUID(clientID)
		if err != nil {
			log.Printf("user %q: %v, skipping client roles", username, err)
			continue
		}
		mappingPath := base + "/" + url.PathEscape(userID) + "/role-mappings/clients/" + url.PathEscape(clientUUID)
		var current []map[string]any
		if _, err := a.getJSON(mappingPath, &current); err != nil {
			return fmt.Errorf("role mappings of user %q for client %q: %w", username, clientID, err)
		}
		currentNames := map[string]bool{}
		for _, mapping := range current {
			if name, _ := mapping["name"].(string); name != "" {
				currentNames[name] = true
			}
		}
		var missing []map[string]any
		for _, rawName := range roleNames {
			roleName, _ := rawName.(string)
			if roleName == "" || currentNames[roleName] {
				continue
			}
			id, err := roles.roleID(clientID, roleName)
			if err != nil {
				log.Printf("user %q: %v, skipping role", username, err)
				continue
			}
			missing = append(missing, map[string]any{"id": id, "name": roleName})
		}
		if len(missing) == 0 {
			continue
		}
		if err := a.update(http.MethodPost, mappingPath, missing); err != nil {
			return fmt.Errorf("add role mappings of user %q for client %q: %w", username, clientID, err)
		}
		log.Printf("user %q: added %d role(s) of client %q", username, len(missing), clientID)
	}
	return nil
}

func (a *adminClient) findUserID(username, email string) (string, error) {
	base := "/admin/realms/" + url.PathEscape(a.cfg.realm) + "/users"
	var users []map[string]any
	if _, err := a.getJSON(base+"?username="+url.QueryEscape(username)+"&exact=true", &users); err != nil {
		return "", err
	}
	for _, user := range users {
		if name, _ := user["username"].(string); strings.EqualFold(name, username) {
			id, _ := user["id"].(string)
			return id, nil
		}
	}
	if email != "" {
		if _, err := a.getJSON(base+"?email="+url.QueryEscape(email)+"&exact=true", &users); err != nil {
			return "", err
		}
		for _, user := range users {
			if name, _ := user["username"].(string); strings.EqualFold(name, username) {
				id, _ := user["id"].(string)
				return id, nil
			}
		}
	}
	return "", nil
}

// ---------------------------------------------------------------------------
// 7. organizations: upsert by alias, add missing members, never delete
// ---------------------------------------------------------------------------

func ensureOrganizations(a *adminClient, file *realmFile, userIDs map[string]string) error {
	base := "/admin/realms/" + url.PathEscape(a.cfg.realm) + "/organizations"
	existing, err := a.listOrganizations()
	if err != nil {
		return err
	}

	for _, org := range file.orgs {
		name, _ := org["name"].(string)
		alias, _ := org["alias"].(string)
		if alias == "" {
			alias = name
		}
		if name == "" {
			continue
		}
		var current map[string]any
		for _, candidate := range existing {
			candidateAlias, _ := candidate["alias"].(string)
			candidateName, _ := candidate["name"].(string)
			if candidateAlias == alias || candidateName == name {
				current = candidate
				break
			}
		}

		orgID := ""
		if current == nil {
			// The members list is not part of the create payload.
			orgID, err = a.create(base, orgWithoutMembers(org))
			if err != nil {
				return fmt.Errorf("create organization %q: %w", name, err)
			}
			log.Printf("organization %q: created", name)
		} else {
			orgID, _ = current["id"].(string)
			if overlayObject(current, org) {
				if err := a.update(http.MethodPut, base+"/"+url.PathEscape(orgID), current); err != nil {
					return fmt.Errorf("update organization %q: %w", name, err)
				}
				log.Printf("organization %q: updated", name)
			} else {
				log.Printf("organization %q: up to date", name)
			}
		}
		if orgID == "" {
			// The create response carried no parsable ID: resolve via a fresh
			// listing.
			fresh, listErr := a.listOrganizations()
			if listErr != nil {
				return listErr
			}
			for _, candidate := range fresh {
				candidateAlias, _ := candidate["alias"].(string)
				if candidateAlias == alias {
					orgID, _ = candidate["id"].(string)
					break
				}
			}
		}

		// Members from the file are added if missing; members that were added
		// at runtime are never removed.
		rawMembers, _ := org["members"].([]any)
		if len(rawMembers) == 0 || orgID == "" {
			continue
		}
		currentMembers, err := a.listOrganizationMembers(orgID)
		if err != nil {
			return err
		}
		memberIDs := map[string]bool{}
		for _, member := range currentMembers {
			if id, _ := member["id"].(string); id != "" {
				memberIDs[id] = true
			}
		}
		for _, rawMember := range rawMembers {
			member, _ := rawMember.(map[string]any)
			username, _ := member["username"].(string)
			if username == "" {
				continue
			}
			userID := userIDs[username]
			if userID == "" {
				resolved, err := a.findUserID(username, username)
				if err != nil {
					return err
				}
				if resolved == "" {
					log.Printf("organization %q: member %q not found, skipping", name, username)
					continue
				}
				userID = resolved
			}
			if memberIDs[userID] {
				continue
			}
			// Keycloak 26 takes the user id as the JSON body of the member
			// POST, not as a path parameter.
			if err := a.update(http.MethodPost, base+"/"+url.PathEscape(orgID)+"/members", userID); err != nil {
				return fmt.Errorf("add member %q to organization %q: %w", username, name, err)
			}
			log.Printf("organization %q: added member %q", name, username)
		}
	}
	return nil
}

// orgWithoutMembers returns the organization representation without the
// members list, which is not part of the create payload.
func orgWithoutMembers(org map[string]any) map[string]any {
	clone := map[string]any{}
	for key, value := range org {
		if key == "members" {
			continue
		}
		clone[key] = value
	}
	return clone
}

func (a *adminClient) listOrganizations() ([]map[string]any, error) {
	base := "/admin/realms/" + url.PathEscape(a.cfg.realm) + "/organizations"
	return a.listPaged(base)
}

func (a *adminClient) listOrganizationMembers(orgID string) ([]map[string]any, error) {
	base := "/admin/realms/" + url.PathEscape(a.cfg.realm) + "/organizations/" + url.PathEscape(orgID) + "/members"
	return a.listPaged(base)
}

// listPaged walks a paginated Keycloak collection endpoint.
func (a *adminClient) listPaged(base string) ([]map[string]any, error) {
	var all []map[string]any
	first := 0
	for {
		var page []map[string]any
		path := fmt.Sprintf("%s?first=%d&max=%d", base, first, pageSize)
		if _, err := a.getJSON(path, &page); err != nil {
			return nil, err
		}
		all = append(all, page...)
		if len(page) < pageSize {
			return all, nil
		}
		first += pageSize
	}
}

// ---------------------------------------------------------------------------
// small helpers
// ---------------------------------------------------------------------------

func envOr(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

// equalJSON compares two values canonically: object key order and array
// element order are irrelevant, because Keycloak returns set-typed fields
// (redirect URIs, web origins, protocol mappers, ...) in arbitrary order.
func equalJSON(a, b any) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return canonical(a) == canonical(b)
}

func canonical(v any) string {
	switch typed := v.(type) {
	case map[string]any:
		keys := make([]string, 0, len(typed))
		for key := range typed {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		var buffer strings.Builder
		buffer.WriteByte('{')
		for index, key := range keys {
			if index > 0 {
				buffer.WriteByte(',')
			}
			keyJSON, _ := json.Marshal(key)
			buffer.Write(keyJSON)
			buffer.WriteByte(':')
			buffer.WriteString(canonical(typed[key]))
		}
		buffer.WriteByte('}')
		return buffer.String()
	case []any:
		elements := make([]string, 0, len(typed))
		for _, element := range typed {
			elements = append(elements, canonical(element))
		}
		sort.Strings(elements)
		return "[" + strings.Join(elements, ",") + "]"
	default:
		encoded, err := json.Marshal(v)
		if err != nil {
			return fmt.Sprintf("%v", v)
		}
		return string(encoded)
	}
}
