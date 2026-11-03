import { NextResponse, type NextRequest } from "next/server";

import { getSession, requireSession } from "@/features/auth";
import { envConfig } from "@/features/shared/lib/env";

export async function GET(
  request: NextRequest,
  { params }: { params: Promise<{ alias: string }> },
) {
  await requireSession();

  const session = await getSession();
  if (!session) {
    return NextResponse.json(
      { error: { code: "UNAUTHENTICATED", message: "Anmeldung erforderlich" } },
      { status: 401 },
    );
  }

  const { alias } = await params;

  if (!session.organizations.includes(alias)) {
    return NextResponse.json(
      { error: { code: "FORBIDDEN", message: "Du bist kein Mitglied dieser Organisation" } },
      { status: 403 },
    );
  }

  const query = new URLSearchParams();
  for (const key of ["limit", "cursor"]) {
    const value = request.nextUrl.searchParams.get(key);
    if (value) {
      query.set(key, value);
    }
  }
  const queryString = query.toString();

  const response = await fetch(
    `${envConfig.apiBaseUrl}/organizations/${encodeURIComponent(alias)}/audit-logs${queryString ? `?${queryString}` : ""}`,
    {
      headers: { Authorization: `Bearer ${session.accessToken}` },
      cache: "no-store",
    },
  );

  const body = await response.json().catch(() => null);

  return NextResponse.json(body, { status: response.status });
}
