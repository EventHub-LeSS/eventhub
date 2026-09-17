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

async function keycloakFetch<T>(
  accessToken: string,
  path: string,
): Promise<T> {
  const response = await fetch(`${adminBaseUrl()}${path}`, {
    headers: { Authorization: `Bearer ${accessToken}` },
    cache: "no-store",
  });

  if (!response.ok) {
    throw new KeycloakAdminError(
      `Keycloak admin request to ${path} failed`,
      response.status,
    );
  }

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
    name: memberName(member),
    email: member.email ?? "",
    joinedAt: member.createdTimestamp
      ? new Date(member.createdTimestamp).toISOString()
      : null,
    rights: rightsByUserId.get(member.id) ?? [],
  }));
}
