import "server-only"
import * as client from "openid-client"

import { authConfig, getCallbackUrl, authScope } from "@/features/auth/lib/config"

let discovered: Promise<client.Configuration> | undefined

/** Discovery is cached in a module-level promise so it runs once per server process. */
function getOidcConfig() {
  if (!discovered) {
    const issuer = new URL(authConfig.issuer)
    const options =
      issuer.protocol === "http:"
        ? { execute: [client.allowInsecureRequests] }
        : undefined

    const clientSecret = authConfig.clientSecret || undefined

    discovered = client
      .discovery(
        issuer,
        authConfig.clientId,
        clientSecret,
        clientSecret ? undefined : client.None(),
        options
      )
      .catch((error) => {
        discovered = undefined
        throw error
      })
  }

  return discovered
}

export interface AuthorizationRequest {
  url: string
  state: string
  nonce: string
  codeVerifier: string
}

export async function createAuthorizationRequest(
  options: { prompt?: string } = {}
): Promise<AuthorizationRequest> {
  const config = await getOidcConfig()

  const codeVerifier = client.randomPKCECodeVerifier()
  const codeChallenge = await client.calculatePKCECodeChallenge(codeVerifier)
  const state = client.randomState()
  const nonce = client.randomNonce()

  const url = client.buildAuthorizationUrl(config, {
    redirect_uri: getCallbackUrl(),
    scope: authScope,
    code_challenge: codeChallenge,
    code_challenge_method: "S256",
    state,
    nonce,
    ...(options.prompt ? { prompt: options.prompt } : {}),
  })

  return { url: url.href, state, nonce, codeVerifier }
}

/** Exchanges the authorization code and validates the ID token signature, issuer, audience and nonce. */
export async function exchangeCode(
  currentUrl: URL,
  checks: { state: string; nonce: string; codeVerifier: string }
) {
  const config = await getOidcConfig()

  const tokens = await client.authorizationCodeGrant(config, currentUrl, {
    pkceCodeVerifier: checks.codeVerifier,
    expectedState: checks.state,
    expectedNonce: checks.nonce,
  })

  return {
    claims: tokens.claims(),
    accessToken: tokens.access_token,
    refreshToken: tokens.refresh_token,
    expiresIn: tokens.expiresIn(),
  }
}

/** Exchanges the refresh token for a new access token once the short-lived access token expires. */
export async function refreshAccessToken(refreshToken: string) {
  const config = await getOidcConfig()

  const tokens = await client.refreshTokenGrant(config, refreshToken)

  return {
    claims: tokens.claims(),
    accessToken: tokens.access_token,
    // Keycloak rotates refresh tokens on use; fall back to the old one if none comes back.
    refreshToken: tokens.refresh_token ?? refreshToken,
    expiresIn: tokens.expiresIn(),
  }
}

/**
 * Ends the Keycloak SSO session server-to-server (Keycloak's non-browser logout:
 * a POST to the end-session endpoint with the refresh token), so the user never
 * sees a Keycloak page. Returns false if the session may still be active, in
 * which case the caller should fall back to the browser redirect.
 */
export async function endSessionBackchannel(refreshToken: string) {
  try {
    const config = await getOidcConfig()
    const endpoint = config.serverMetadata().end_session_endpoint

    if (!endpoint) {
      return false
    }

    const body = new URLSearchParams({
      client_id: authConfig.clientId,
      refresh_token: refreshToken,
    })
    if (authConfig.clientSecret) {
      body.set("client_secret", authConfig.clientSecret)
    }

    const response = await fetch(endpoint, {
      method: "POST",
      headers: { "Content-Type": "application/x-www-form-urlencoded" },
      body,
      cache: "no-store",
      signal: AbortSignal.timeout(5000),
    })

    if (response.ok) {
      return true
    }

    // invalid_grant means the refresh token's session is already gone, which is
    // the outcome we wanted anyway.
    const error = await response
      .json()
      .then((data: { error?: unknown }) => data.error)
      .catch(() => undefined)

    return error === "invalid_grant"
  } catch {
    return false
  }
}

/**
 * Fallback for when the back-channel logout fails. No id_token_hint: Keycloak then asks
 * the user to confirm the logout, but it also cannot dead-end on
 * "Invalid parameter: id_token_hint" after the realm signing keys change.
 */
export async function buildLogoutUrl(postLogoutRedirectUri: string) {
  const config = await getOidcConfig()

  const url = client.buildEndSessionUrl(config, {
    post_logout_redirect_uri: postLogoutRedirectUri,
  })

  return url.href
}
