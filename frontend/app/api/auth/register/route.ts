import type { NextRequest } from "next/server"

import { startAuthorization } from "@/features/auth"

export async function GET(request: NextRequest) {
  // "create" is the standard OIDC prompt for the identity provider's registration screen.
  return startAuthorization(request, "create")
}
