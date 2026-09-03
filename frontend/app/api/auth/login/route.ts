import type { NextRequest } from "next/server"

import { startAuthorization } from "@/features/auth"

export async function GET(request: NextRequest) {
  return startAuthorization(request)
}
