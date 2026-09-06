# Local Keycloak

The local realm import configures the security contract used by the backend:

- `frontend`: public OpenID Connect client using Authorization Code with PKCE
- `backend`: confidential resource-server and token-introspection client
- global backend client roles: `admin`, `moderator`, `visitor`
- optional `organization` scope for one active organization per token
- `backend-audience` mapper adding `backend` to access-token audiences

Start Keycloak through WSL from the repository root:

```sh
wsl docker compose -f core/docker-compose.yml up -d
```

The development admin console is available at `http://localhost:5433` with the credentials configured in `docker-compose.yml`.

The imported backend client secret is development-only. Configure the backend with:

```text
KEYCLOAK_HOST=http://localhost:5433
KEYCLOAK_REALM=eventhub
KEYCLOAK_CLIENT_ID=backend
KEYCLOAK_CLIENT_SECRET=eventhub-backend-dev-secret
KEYCLOAK_AUDIENCE=backend
KEYCLOAK_ALLOWED_AZP=frontend
```

## Organization Roles

For each organization, create these organization-local groups and assign members as needed:

- `/roles/org_admin`
- `/roles/event_manager`
- `/roles/finance_viewer`

The imported `organization` client scope contains the Organization Membership and Organization Group Membership mappers. It includes the stable organization ID and group paths in access-token and introspection claims.

Organization-scoped requests must use exactly one organization context through `scope=organization:<alias>`. Tokens containing multiple organizations are rejected by the backend. Tokens without organization context remain valid for global and visitor-only endpoints.

Keycloak 26.6 does not support declaratively importing organizations and organization-local groups in a realm export. They must be provisioned through the Admin API or Admin Console after realm import. The backend uses the stable organization ID from the token for authorization.
