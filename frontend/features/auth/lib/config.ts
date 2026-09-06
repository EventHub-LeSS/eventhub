import "server-only"

import { envConfig } from "@/features/shared/lib/env"

export const authConfig = {
  get issuer() {
    return envConfig.keycloakIssuer
  },
  get clientId() {
    return envConfig.keycloakClientId
  },
  get clientSecret() {
    return envConfig.keycloakClientSecret
  },
  get appBaseUrl() {
    return envConfig.appBaseUrl
  },
  get sessionSecret() {
    return envConfig.sessionSecret
  },
}

export function getCallbackUrl(): string {
  return `${authConfig.appBaseUrl}/api/auth/callback`
}

/**
 * "organization:*" returns every organization the user belongs to. The plain
 * "organization" scope would make Keycloak show an org picker for multi-org users.
 */
export const authScope = "openid profile email organization:*"
