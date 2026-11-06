import { envConfig } from "@/features/shared/lib/env"

export async function GET(
  _request: Request,
  { params }: { params: Promise<{ id: string }> }
) {
  const { id } = await params

  try {
    const upstream = await fetch(
      `${envConfig.apiBaseUrl}/events/${encodeURIComponent(id)}/image`,
      { cache: "no-store" }
    )

    if (!upstream.ok || !upstream.body) {
      return new Response(null, {
        status: upstream.status >= 500 ? 502 : upstream.status,
      })
    }

    return new Response(upstream.body, {
      headers: {
        "Content-Type":
          upstream.headers.get("Content-Type") ?? "application/octet-stream",
        "Cache-Control": "no-cache",
      },
    })
  } catch {
    return new Response(null, { status: 502 })
  }
}
