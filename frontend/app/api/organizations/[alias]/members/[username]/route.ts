import { NextResponse } from "next/server";

import { getSession, requireSession } from "@/features/auth";
import {
  KeycloakAdminError,
  getOrganizationByAlias,
  removeOrganizationMemberByUsername,
} from "@/features/organizations";

export async function DELETE(
  _request: Request,
  { params }: { params: Promise<{ alias: string; username: string }> },
) {
  await requireSession();

  const session = await getSession();
  if (!session) {
    return NextResponse.json(
      { error: { code: "UNAUTHENTICATED", message: "Authentication is required" } },
      { status: 401 },
    );
  }

  const { alias, username } = await params;

  if (!session.organizations.includes(alias)) {
    return NextResponse.json(
      { error: { code: "FORBIDDEN", message: "You are not a member of this organization" } },
      { status: 403 },
    );
  }

  try {
    const organization = await getOrganizationByAlias(session.accessToken, alias);
    await removeOrganizationMemberByUsername(
      session.accessToken,
      organization.id,
      username,
    );
    return new NextResponse(null, { status: 204 });
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
