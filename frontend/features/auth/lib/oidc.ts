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

    discovered = client
      .discovery(
        issuer,
        authConfig.clientId,
        authConfig.clientSecret,
        undefined,
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

  return tokens.claims()
}

/**
 * No id_token_hint: Keycloak then asks the user to confirm the logout, but it also cannot
 * dead-end on "Invalid parameter: id_token_hint" after the realm signing keys change.
 */
export async function buildLogoutUrl(postLogoutRedirectUri: string) {
  const config = await getOidcConfig()

  const url = client.buildEndSessionUrl(config, {
    post_logout_redirect_uri: postLogoutRedirectUri,
  })

  return url.href
}
