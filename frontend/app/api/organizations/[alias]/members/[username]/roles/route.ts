import { NextResponse, type NextRequest } from "next/server";

import {
  createSessionCookie,
  getSession,
  organizationsFromClaims,
  refreshAccessToken,
  requireSession,
} from "@/features/auth";
import type { OrganizationRight } from "@/features/organizations";
import { envConfig } from "@/features/shared/lib/env";

export async function PUT(
  request: NextRequest,
  { params }: { params: Promise<{ alias: string; username: string }> },
) {
  await requireSession();

  const session = await getSession();
  if (!session) {
    return NextResponse.json(
      { error: { code: "UNAUTHENTICATED", message: "Anmeldung erforderlich" } },
      { status: 401 },
    );
  }

  const { alias, username } = await params;

  if (!session.organizations.includes(alias)) {
    return NextResponse.json(
      { error: { code: "FORBIDDEN", message: "Du bist kein Mitglied dieser Organisation" } },
      { status: 403 },
    );
  }

  const payload = (await request.json()) as { roles: OrganizationRight[] };

  const response = await fetch(
    `${envConfig.apiBaseUrl}/organizations/${alias}/members/${username}/roles`,
    {
      method: "PUT",
      headers: {
        "Content-Type": "application/json",
        Authorization: `Bearer ${session.accessToken}`,
      },
      body: JSON.stringify(payload),
    },
  );

  const text = await response.text();
  let body: unknown = null;

  if (text) {
    try {
      body = JSON.parse(text);
    } catch {
      body = { error: { code: "BAD_GATEWAY", message: text } };
    }
  }

  if (
    response.ok &&
    username.toLowerCase() === session.email.toLowerCase()
  ) {
    try {
      const refreshed = await refreshAccessToken(session.refreshToken);

      if (!refreshed.accessToken) {
        throw new Error("refresh returned no access token");
      }

      await createSessionCookie({
        ...session,
        organizations: refreshed.claims
          ? organizationsFromClaims(refreshed.claims.organization)
          : session.organizations,
        accessToken: refreshed.accessToken,
        refreshToken: refreshed.refreshToken,
        accessTokenExpiresAt: Date.now() + (refreshed.expiresIn ?? 0) * 1000,
      });
    } catch {
      return NextResponse.json(
        {
          error: {
            code: "SESSION_REFRESH_FAILED",
            message:
              "Die Rechte wurden gespeichert, aber die Sitzung konnte nicht aktualisiert werden. Bitte lade die Seite neu.",
          },
        },
        { status: 502 },
      );
    }
  }

  return NextResponse.json(body, { status: response.status });
}
