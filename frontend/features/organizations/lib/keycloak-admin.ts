import "server-only";

import { envConfig } from "@/features/shared/lib/env";
import type {
  Organization,
  OrganizationMember,
  OrganizationRight,
} from "@/features/organizations/lib/types";

/**
 * Talks to Keycloak's Admin REST API directly, using the signed-in user's own
 * access token. Keycloak's Organizations feature grants members of an org's
 * "org_admin" group scoped admin permissions over just that org, so this works
 * without a separate service account.
 */
export class KeycloakAdminError extends Error {
  constructor(
    message: string,
    public readonly status: number,
  ) {
    super(message);
    this.name = "KeycloakAdminError";
  }
}

interface KeycloakOrganizationDomain {
  name: string;
  verified?: boolean;
}

interface KeycloakOrganization {
  id: string;
  name: string;
  alias: string;
  enabled?: boolean;
  description?: string;
  domains?: KeycloakOrganizationDomain[];
}

interface KeycloakGroup {
  id: string;
  name: string;
}

interface KeycloakUser {
  id: string;
  username?: string;
  email?: string;
  firstName?: string;
  lastName?: string;
  createdTimestamp?: number;
}

function adminBaseUrl(): string {
  return envConfig.keycloakIssuer.replace("/realms/", "/admin/realms/");
}

async function keycloakRequest(
  accessToken: string,
  path: string,
  init: RequestInit = {},
): Promise<Response> {
  const response = await fetch(`${adminBaseUrl()}${path}`, {
    ...init,
    headers: { Authorization: `Bearer ${accessToken}`, ...init.headers },
    cache: "no-store",
  });

  if (!response.ok) {
    const detail = await response.text().catch(() => "");
    throw new KeycloakAdminError(
      detail || `Keycloak admin request to ${path} failed`,
      response.status,
    );
  }

  return response;
}

async function keycloakFetch<T>(
  accessToken: string,
  path: string,
): Promise<T> {
  const response = await keycloakRequest(accessToken, path);

  return (await response.json()) as T;
}

function memberName(user: KeycloakUser): string {
  const name = [user.firstName, user.lastName].filter(Boolean).join(" ").trim();
  return name || user.username || user.email || user.id;
}

export async function getOrganizationByAlias(
  accessToken: string,
  alias: string,
): Promise<Organization> {
  const orgs = await keycloakFetch<KeycloakOrganization[]>(
    accessToken,
    "/organizations?max=200",
  );
  const match = orgs.find((org) => org.alias === alias);

  if (!match) {
    throw new KeycloakAdminError(`Organization "${alias}" not found`, 404);
  }

  return {
    id: match.id,
    name: match.name,
    alias: match.alias,
    description: match.description ?? "",
    enabled: match.enabled ?? true,
    domains: (match.domains ?? []).map((domain) => ({
      name: domain.name,
      verified: domain.verified ?? false,
    })),
  };
}

async function getOrganizationGroups(
  accessToken: string,
  orgId: string,
): Promise<KeycloakGroup[]> {
  return keycloakFetch<KeycloakGroup[]>(
    accessToken,
    `/organizations/${orgId}/groups`,
  );
}

async function getOrganizationGroupMembers(
  accessToken: string,
  orgId: string,
  groupId: string,
): Promise<KeycloakUser[]> {
  return keycloakFetch<KeycloakUser[]>(
    accessToken,
    `/organizations/${orgId}/groups/${groupId}/members`,
  );
}

const KNOWN_RIGHTS: OrganizationRight[] = [
  "org_admin",
  "event_manager",
  "finance_viewer",
];

export async function getOrganizationMembersWithRights(
  accessToken: string,
  orgId: string,
): Promise<OrganizationMember[]> {
  const [members, groups] = await Promise.all([
    keycloakFetch<KeycloakUser[]>(accessToken, `/organizations/${orgId}/members`),
    getOrganizationGroups(accessToken, orgId),
  ]);

  const rightsByUserId = new Map<string, OrganizationRight[]>();
  await Promise.all(
    groups
      .filter((group) => KNOWN_RIGHTS.includes(group.name as OrganizationRight))
      .map(async (group) => {
        const right = group.name as OrganizationRight;
        const groupMembers = await getOrganizationGroupMembers(
          accessToken,
          orgId,
          group.id,
        );
        for (const user of groupMembers) {
          const existing = rightsByUserId.get(user.id) ?? [];
          existing.push(right);
          rightsByUserId.set(user.id, existing);
        }
      }),
  );

  return members.map((member) => ({
    id: member.id,
    username: member.username ?? "",
    name: memberName(member),
    email: member.email ?? "",
    joinedAt: member.createdTimestamp
      ? new Date(member.createdTimestamp).toISOString()
      : null,
    rights: rightsByUserId.get(member.id) ?? [],
  }));
}

/**
 * Invites someone to the organization by email via Keycloak's own Organizations
 * invitation flow (POST .../members/invite-user). New emails get an account-setup
 * invite, existing users get a join invite; either way membership is pending until
 * they accept, matching "Scheidet ein Mitarbeitender aus, bleiben Daten bei der
 * Organisation" — nothing is granted to this app until Keycloak confirms it.
 */
export async function inviteOrganizationMember(
  accessToken: string,
  orgId: string,
  email: string,
  name?: string,
): Promise<void> {
  const form = new FormData();
  form.set("email", email);

  const trimmedName = name?.trim();
  if (trimmedName) {
    const [firstName, ...rest] = trimmedName.split(/\s+/);
    form.set("firstName", firstName);
    if (rest.length > 0) {
      form.set("lastName", rest.join(" "));
    }
  }

  await keycloakRequest(accessToken, `/organizations/${orgId}/members/invite-user`, {
    method: "POST",
    body: form,
  });
}

/**
 * Removes someone's membership only — their events, sales figures and billing
 * records live on the organization itself, not on the Keycloak user, so this
 * cannot and does not touch them.
 */
export async function removeOrganizationMember(
  accessToken: string,
  orgId: string,
  userId: string,
): Promise<void> {
  await keycloakRequest(accessToken, `/organizations/${orgId}/members/${userId}`, {
    method: "DELETE",
  });
}

/** The members API is keyed by username, but Keycloak's endpoint takes the user's ID. */
export async function removeOrganizationMemberByUsername(
  accessToken: string,
  orgId: string,
  username: string,
): Promise<void> {
  const members = await getOrganizationMembersWithRights(accessToken, orgId);
  const member = members.find((candidate) => candidate.username === username);

  if (!member) {
    throw new KeycloakAdminError(
      `"${username}" is not a member of this organization`,
      404,
    );
  }

  await removeOrganizationMember(accessToken, orgId, member.id);
}
