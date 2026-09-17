"use client";

import { useMutation, useQuery } from "@tanstack/react-query";

import type { OrganizationPayload } from "@/features/organizations/lib/organization";
import type {
  Organization,
  OrganizationMember,
} from "@/features/organizations/lib/types";

export interface CreateOrganizationResponse {
  id: string;
  name: string;
  alias: string;
}

export function useCreateOrganization() {
  return useMutation<CreateOrganizationResponse, Error, OrganizationPayload>({
    mutationFn: createOrganization,
  });
}

async function createOrganization(
  payload: OrganizationPayload,
): Promise<CreateOrganizationResponse> {
  const response = await fetch("/api/organizations", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(payload),
  });

  const body = await response.json().catch(() => null);

  if (!response.ok) {
    throw new Error(errorMessage(body, response.status));
  }

  return body as CreateOrganizationResponse;
}

function errorMessage(body: unknown, status: number): string {
  if (body && typeof body === "object") {
    const { error, message } = body as { error?: unknown; message?: unknown };

    if (typeof error === "string" && error) {
      return error;
    }

    if (typeof message === "string" && message) {
      return message;
    }

    if (error && typeof error === "object" && "message" in error) {
      const nested = (error as { message?: unknown }).message;
      if (typeof nested === "string" && nested) {
        return nested;
      }
    }
  }

  return `Request failed (${status}).`;
}

export function useOrganization(alias: string) {
  return useQuery<Organization, Error>({
    queryKey: ["organization", alias],
    queryFn: () => fetchOrganization(alias),
  });
}

export function useOrganizationMembers(alias: string) {
  return useQuery<OrganizationMember[], Error>({
    queryKey: ["organization", alias, "members"],
    queryFn: () => fetchOrganizationMembers(alias),
  });
}

async function fetchOrganization(alias: string): Promise<Organization> {
  const response = await fetch(`/api/organizations/${alias}`);
  const body = await response.json().catch(() => null);

  if (!response.ok) {
    throw new Error(errorMessage(body, response.status));
  }

  return body as Organization;
}

async function fetchOrganizationMembers(
  alias: string,
): Promise<OrganizationMember[]> {
  const response = await fetch(`/api/organizations/${alias}/members`);
  const body = await response.json().catch(() => null);

  if (!response.ok) {
    throw new Error(errorMessage(body, response.status));
  }

  return body as OrganizationMember[];
}
