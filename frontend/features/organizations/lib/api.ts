"use client";

import { useMutation } from "@tanstack/react-query";

import type { OrganizationPayload } from "@/features/organizations/lib/organization";

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
  }

  return `Could not create the organization (${status}).`;
}
