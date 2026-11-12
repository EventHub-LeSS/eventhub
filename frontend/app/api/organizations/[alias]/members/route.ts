import { NextResponse } from "next/server";

import { getSession, requireSession } from "@/features/auth";
import {
  KeycloakAdminError,
  resolveOrganizationId,
  inviteOrganizationMember,
} from "@/features/organizations";
import { envConfig } from "@/features/shared/lib/env";

export async function GET(
  _request: Request,
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

  // The API reads the members with its service account; Keycloak's admin API does not accept
  // the token of an organization admin who is no global admin.
  const response = await fetch(
    `${envConfig.apiBaseUrl}/organizations/${alias}/members`,
    {
      headers: { Authorization: `Bearer ${session.accessToken}` },
      cache: "no-store",
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

  return NextResponse.json(body, { status: response.status });
}

export async function POST(
  request: Request,
  { params }: { params: Promise<{ alias: string }> },
) {
  await requireSession();

  const session = await getSession();
  if (!session) {
    return NextResponse.json(
      { error: { code: "UNAUTHENTICATED", message: "Authentication is required" } },
      { status: 401 },
    );
  }

  const { alias } = await params;

  if (!session.organizations.includes(alias)) {
    return NextResponse.json(
      { error: { code: "FORBIDDEN", message: "You are not a member of this organization" } },
      { status: 403 },
    );
  }

  const payload = (await request.json()) as { email?: string; name?: string };
  const email = payload.email?.trim();

  if (!email) {
    return NextResponse.json(
      { error: { code: "VALIDATION_ERROR", message: "email is required" } },
      { status: 400 },
    );
  }

  try {
    const organizationId = await resolveOrganizationId(session.accessToken, alias);
    await inviteOrganizationMember(
      session.accessToken,
      organizationId,
      email,
      payload.name,
    );
    return NextResponse.json({ message: "invitation sent" }, { status: 202 });
  } catch (error) {
    if (error instanceof KeycloakAdminError) {
      return NextResponse.json(
        { error: { code: "KEYCLOAK_ERROR", message: error.message } },
        { status: error.status === 404 ? 404 : error.status },
      );
    }
    throw error;
  }
}
