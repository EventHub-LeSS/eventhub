import { NextResponse } from "next/server";

import { getSession, requireSession } from "@/features/auth";
import {
  KeycloakAdminError,
  getOrganizationByAlias,
} from "@/features/organizations";

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

  try {
    const organization = await getOrganizationByAlias(session.accessToken, alias);
    return NextResponse.json(organization);
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
