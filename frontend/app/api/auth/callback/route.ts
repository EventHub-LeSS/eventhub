import type { NextRequest } from "next/server"

import { handleCallback } from "@/features/auth"

export async function GET(request: NextRequest) {
  return handleCallback(request)
}
