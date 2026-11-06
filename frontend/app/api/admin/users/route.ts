import { authorizePermissions, requestAdminUsers } from "@/features/permissions"

export async function GET(request: Request) {
  const authorization = await authorizePermissions()
  if (authorization instanceof Response) return authorization

  const page = new URL(request.url).searchParams.get("page") ?? "1"
  const query = new URLSearchParams({ page, limit: "50" })
  const response = await requestAdminUsers(
    `?${query}`,
    authorization.accessToken
  )
  return new Response(response.body, {
    status: response.status,
    headers: {
      "Content-Type":
        response.headers.get("Content-Type") ?? "application/json",
      "Cache-Control": "no-store",
    },
  })
}
