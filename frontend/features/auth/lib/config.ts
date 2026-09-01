import "server-only"

function required(name: string): string {
  const value = process.env[name]

  if (!value) {
    throw new Error(`Missing environment variable ${name}. See .env.example.`)
  }

  return value
}

export const authConfig = {
  issuer: required("KEYCLOAK_ISSUER"),
  clientId: required("KEYCLOAK_CLIENT_ID"),
  clientSecret: required("KEYCLOAK_CLIENT_SECRET"),
  appBaseUrl: required("APP_BASE_URL"),
  sessionSecret: required("SESSION_SECRET"),
}

export const callbackUrl = `${authConfig.appBaseUrl}/api/auth/callback`

/**
 * "organization:*" returns every organization the user belongs to. The plain
 * "organization" scope would make Keycloak show an org picker for multi-org users.
 */
export const authScope = "openid profile email organization:*"
