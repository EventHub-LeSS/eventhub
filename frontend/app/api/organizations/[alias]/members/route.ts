import { NextResponse } from "next/server";

import { getSession, requireSession } from "@/features/auth";
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

  let response: Response;
  try {
    response = await fetch(
      `${envConfig.apiBaseUrl}/organizations/${encodeURIComponent(alias)}/members`,
      {
        headers: { Authorization: `Bearer ${session.accessToken}` },
        cache: "no-store",
      },
    );
  } catch (error) {
    console.error("Failed to fetch organization members", error);
    return NextResponse.json(
      { error: { code: "BAD_GATEWAY", message: "Mitglieder konnten nicht geladen werden" } },
      { status: 502 },
    );
  }

  let body: unknown;
  try {
    body = await response.json();
  } catch {
    return NextResponse.json(
      { error: { code: "BAD_GATEWAY", message: "Die Mitgliederantwort war ungültig" } },
      { status: 502 },
    );
  }

  return NextResponse.json(body, {
    status: response.status,
    headers: { "Cache-Control": "no-store" },
  });
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

  let payload: { email?: unknown; name?: unknown };
  try {
    payload = (await request.json()) as { email?: unknown; name?: unknown };
  } catch {
    return NextResponse.json(
      { error: { code: "VALIDATION_ERROR", message: "Die Anfrage muss gültiges JSON enthalten" } },
      { status: 400 },
    );
  }

  const email = typeof payload.email === "string" ? payload.email.trim() : "";

  if (!email) {
    return NextResponse.json(
      { error: { code: "VALIDATION_ERROR", message: "email is required" } },
      { status: 400 },
    );
  }

  const name = typeof payload.name === "string" ? payload.name.trim() : "";
  let response: Response;
  try {
    response = await fetch(
      `${envConfig.apiBaseUrl}/organizations/${encodeURIComponent(alias)}/members/invitations`,
      {
        method: "POST",
        headers: {
          Authorization: `Bearer ${session.accessToken}`,
          "Content-Type": "application/json",
        },
        body: JSON.stringify({ email, ...(name ? { name } : {}) }),
        cache: "no-store",
      },
    );
  } catch (error) {
    console.error("Failed to send organization invitation", error);
    return NextResponse.json(
      { error: { code: "BAD_GATEWAY", message: "Die Einladung konnte nicht gesendet werden" } },
      { status: 502 },
    );
  }

  let body: unknown;
  try {
    body = await response.json();
  } catch {
    return NextResponse.json(
      { error: { code: "BAD_GATEWAY", message: "Die Einladungsantwort war ungültig" } },
      { status: 502 },
    );
  }

  return NextResponse.json(body, {
    status: response.status,
    headers: { "Cache-Control": "no-store" },
  });
}
