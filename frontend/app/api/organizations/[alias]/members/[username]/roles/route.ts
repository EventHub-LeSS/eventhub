import { NextResponse, type NextRequest } from "next/server";

import { getSession, requireSession } from "@/features/auth";
import type { OrganizationRight } from "@/features/organizations";

export async function PUT(
  request: NextRequest,
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

  const payload = (await request.json()) as { roles: OrganizationRight[] };

  const response = await fetch(
    `${process.env.BACKEND_API_URL}/organizations/${alias}/members/${username}/roles`,
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

  return NextResponse.json(body, { status: response.status });
}
