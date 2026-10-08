import "server-only"

const devDefaults: Record<string, string> = {
  KEYCLOAK_ISSUER: "http://localhost:8081/realms/eventhub",
  KEYCLOAK_CLIENT_ID: "frontend",
  APP_BASE_URL: "http://localhost:3000",
  SESSION_SECRET: "replace-me-with-32-random-bytes-base64",
}

const isProduction = process.env.NODE_ENV === "production"

function get(name: string): string {
  const value = process.env[name]
  if (value) {
    return value
  }

  if (isProduction) {
    throw new Error(`Missing environment variable ${name}. See .env.example.`)
  }

  const fallback = devDefaults[name]
  if (!fallback) {
    throw new Error(`Missing environment variable ${name}. See .env.example.`)
  }

  return fallback
}

// Getters defer env var access to request time so importing this module
// during the build (e.g. static error page generation) doesn't throw.
export const envConfig = {
  get keycloakIssuer() {
    return get("KEYCLOAK_ISSUER")
  },
  get keycloakClientId() {
    return get("KEYCLOAK_CLIENT_ID")
  },
  get keycloakClientSecret() {
    return process.env.KEYCLOAK_CLIENT_SECRET ?? ""
  },
  get appBaseUrl() {
    return get("APP_BASE_URL")
  },
  get sessionSecret() {
    return get("SESSION_SECRET")
  },
}
