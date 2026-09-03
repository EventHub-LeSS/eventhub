import type { NextRequest } from "next/server"

import { handleOrganizationSwitch } from "@/features/auth"

export async function POST(request: NextRequest) {
  return handleOrganizationSwitch(request)
}
