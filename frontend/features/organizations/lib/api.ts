"use client"

import {
  useInfiniteQuery,
  useMutation,
  useQuery,
  useQueryClient,
} from "@tanstack/react-query"

import {
  applicationQuery,
  isOrganizerApplication,
  isOrganizerApplicationList,
  isOrganizerApplicationPage,
  type OrganizerApplication,
  type OrganizerApplicationFilters,
  type OrganizerApplicationPage,
  type SubmitOrganizerApplicationPayload,
} from "@/features/organizations/lib/applications"
import type { OrganizationPayload } from "@/features/organizations/lib/organization"
import type {
  AuditLogPage,
  OrganizationMember,
  OrganizationRight,
} from "@/features/organizations/lib/types"

export interface CreateOrganizationResponse {
  id: string
  name: string
  alias: string
  /** Whether the creator made themselves the organization's admin */
  selfAdmin: boolean
}

export interface OrganizationDetails {
  id: string
  name: string
  alias: string
  contactEmail?: string
  contactPhoneNumber?: string
  street?: string
  houseNumber?: string
  postalCode?: string
  city?: string
  countryCode?: string
  createdAt?: string
  updatedAt?: string
}

export function useCreateOrganization() {
  return useMutation<CreateOrganizationResponse, Error, OrganizationPayload>({
    mutationFn: createOrganization,
  })
}

async function createOrganization(
  payload: OrganizationPayload
): Promise<CreateOrganizationResponse> {
  const response = await fetch("/api/organizations", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(payload),
  })

  const body = await response.json().catch(() => null)

  if (!response.ok) {
    throw new Error(errorMessage(body, response.status))
  }

  return body as CreateOrganizationResponse
}

function errorMessage(body: unknown, status: number): string {
  if (body && typeof body === "object") {
    const { error, message, detail } = body as {
      error?: unknown
      message?: unknown
      detail?: unknown
    }

    if (typeof detail === "string" && detail) {
      return detail
    }

    if (typeof error === "string" && error) {
      return error
    }

    if (typeof message === "string" && message) {
      return message
    }

    if (error && typeof error === "object" && "message" in error) {
      const nested = (error as { message?: unknown }).message
      if (typeof nested === "string" && nested) {
        return nested
      }
    }
  }

  return `Anfrage fehlgeschlagen (${status}).`
}

export function useOrganization(alias: string, enabled = true) {
  return useQuery<OrganizationDetails, Error>({
    queryKey: ["organization", alias, "details"],
    queryFn: () => fetchOrganization(alias),
    enabled,
  })
}

async function fetchOrganization(alias: string): Promise<OrganizationDetails> {
  const response = await fetch(`/api/organizations/${alias}`)
  const body = await response.json().catch(() => null)

  if (!response.ok) {
    throw new Error(errorMessage(body, response.status))
  }

  return body as OrganizationDetails
}

export interface UpdateOrganizationResponse {
  id: string
  name: string
  alias: string
  message: string
}

export function useUpdateOrganization(alias: string) {
  const queryClient = useQueryClient()

  return useMutation<UpdateOrganizationResponse, Error, string>({
    mutationFn: (name) => updateOrganization(alias, name),
    onSuccess: () => {
      queryClient.invalidateQueries({
        queryKey: ["organization", alias, "details"],
      })
      queryClient.invalidateQueries({
        queryKey: ["organization", alias, "audit-log"],
      })
    },
  })
}

async function updateOrganization(
  alias: string,
  name: string
): Promise<UpdateOrganizationResponse> {
  const response = await fetch(`/api/organizations/${alias}`, {
    method: "PUT",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ name }),
  })

  const body = await response.json().catch(() => null)

  if (!response.ok) {
    throw new Error(errorMessage(body, response.status))
  }

  return body as UpdateOrganizationResponse
}

export function useOrganizationMembers(alias: string, enabled = true) {
  return useQuery<OrganizationMember[], Error>({
    queryKey: ["organization", alias, "members"],
    queryFn: () => fetchOrganizationMembers(alias),
    enabled,
  })
}

async function fetchOrganizationMembers(
  alias: string
): Promise<OrganizationMember[]> {
  const response = await fetch(`/api/organizations/${alias}/members`)
  const body = await response.json().catch(() => null)

  if (!response.ok) {
    throw new Error(errorMessage(body, response.status))
  }

  return body as OrganizationMember[]
}

export function useOrganizationAuditLog(alias: string, enabled = true) {
  return useInfiniteQuery<
    AuditLogPage,
    Error,
    { pages: AuditLogPage[] },
    string[],
    string | null
  >({
    queryKey: ["organization", alias, "audit-log"],
    queryFn: ({ pageParam }) => fetchOrganizationAuditLog(alias, pageParam),
    initialPageParam: null,
    getNextPageParam: (lastPage) => lastPage.nextCursor,
    enabled,
  })
}

async function fetchOrganizationAuditLog(
  alias: string,
  cursor: string | null
): Promise<AuditLogPage> {
  const query = cursor ? `?cursor=${encodeURIComponent(cursor)}` : ""
  const response = await fetch(`/api/organizations/${alias}/audit-logs${query}`)
  const body = await response.json().catch(() => null)

  if (!response.ok) {
    throw new Error(errorMessage(body, response.status))
  }

  return body as AuditLogPage
}

export interface UpdateMemberRolesVariables {
  username: string
  roles: OrganizationRight[]
}

export interface UpdateMemberRolesResponse {
  username: string
  organizationId: string
  roles: OrganizationRight[]
  message: string
}

export function useUpdateMemberRoles(alias: string) {
  const queryClient = useQueryClient()

  return useMutation<
    UpdateMemberRolesResponse,
    Error,
    UpdateMemberRolesVariables
  >({
    mutationFn: ({ username, roles }) =>
      updateMemberRoles(alias, username, roles),
    onSettled: () => {
      queryClient.invalidateQueries({
        queryKey: ["organization", alias, "members"],
      })
      queryClient.invalidateQueries({
        queryKey: ["organization", alias, "audit-log"],
      })
    },
  })
}

async function updateMemberRoles(
  alias: string,
  username: string,
  roles: OrganizationRight[]
): Promise<UpdateMemberRolesResponse> {
  const response = await fetch(
    `/api/organizations/${alias}/members/${username}/roles`,
    {
      method: "PUT",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ roles }),
    }
  )

  const body = await response.json().catch(() => null)

  if (!response.ok) {
    throw new Error(errorMessage(body, response.status))
  }

  return body as UpdateMemberRolesResponse
}

export interface InviteMemberVariables {
  email: string
  name?: string
}

export interface InviteMemberResponse {
  message: string
}

export function useInviteMember(alias: string) {
  const queryClient = useQueryClient()

  return useMutation<InviteMemberResponse, Error, InviteMemberVariables>({
    mutationFn: (variables) => inviteMember(alias, variables),
    onSuccess: () => {
      queryClient.invalidateQueries({
        queryKey: ["organization", alias, "members"],
      })
    },
  })
}

async function inviteMember(
  alias: string,
  variables: InviteMemberVariables
): Promise<InviteMemberResponse> {
  const response = await fetch(`/api/organizations/${alias}/members`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(variables),
  })

  const body = await response.json().catch(() => null)

  if (!response.ok) {
    throw new Error(errorMessage(body, response.status))
  }

  return body as InviteMemberResponse
}

export function useRemoveMember(alias: string) {
  const queryClient = useQueryClient()

  return useMutation<void, Error, string>({
    mutationFn: (username) => removeMember(alias, username),
    onSuccess: () => {
      queryClient.invalidateQueries({
        queryKey: ["organization", alias, "members"],
      })
    },
  })
}

async function removeMember(alias: string, username: string): Promise<void> {
  const response = await fetch(
    `/api/organizations/${alias}/members/${username}`,
    { method: "DELETE" }
  )

  if (!response.ok) {
    const body = await response.json().catch(() => null)
    throw new Error(errorMessage(body, response.status))
  }
}

const ownApplicationsKey = ["organizer-applications", "mine"] as const
const adminApplicationsKey = ["organizer-applications", "admin"] as const

async function applicationRequest<T>(
  url: string,
  validate: (body: unknown) => body is T,
  payload?: unknown
): Promise<T> {
  let response: Response
  try {
    response = await fetch(url, {
      method: payload !== undefined ? "POST" : "GET",
      ...(payload !== undefined
        ? {
            headers: { "Content-Type": "application/json" },
            body: JSON.stringify(payload),
          }
        : {}),
      cache: "no-store",
    })
  } catch {
    throw new Error(
      "Die Verbindung ist fehlgeschlagen. Bitte versuche es erneut."
    )
  }

  const body: unknown = await response.json().catch(() => null)
  if (!response.ok) {
    throw new Error(errorMessage(body, response.status))
  }
  if (!validate(body)) {
    throw new Error("Die Serverantwort ist unvollständig.")
  }

  return body
}

export function useMyOrganizerApplications(enabled = true) {
  return useQuery<OrganizerApplication[], Error>({
    queryKey: ownApplicationsKey,
    queryFn: () =>
      applicationRequest(
        "/api/organizations/applications",
        isOrganizerApplicationList
      ),
    enabled,
    retry: false,
  })
}

export function useSubmitOrganizerApplication() {
  const queryClient = useQueryClient()

  return useMutation<
    OrganizerApplication,
    Error,
    SubmitOrganizerApplicationPayload
  >({
    mutationFn: (payload) =>
      applicationRequest(
        "/api/organizations/applications",
        isOrganizerApplication,
        payload
      ),
    onSuccess: (created) => {
      queryClient.setQueryData<OrganizerApplication[]>(
        ownApplicationsKey,
        (current) => (current ? [created, ...current] : [created])
      )
      void queryClient.invalidateQueries({
        queryKey: ["organizer-applications"],
      })
    },
  })
}

export function useOrganizerApplications(filters: OrganizerApplicationFilters) {
  return useQuery<OrganizerApplicationPage, Error>({
    queryKey: [...adminApplicationsKey, filters],
    queryFn: () =>
      applicationRequest(
        `/api/admin/organizer-applications?${applicationQuery(filters)}`,
        isOrganizerApplicationPage
      ),
    retry: false,
  })
}

export function useDecideOrganizerApplication() {
  const queryClient = useQueryClient()

  return useMutation<
    OrganizerApplication,
    Error,
    {
      applicationId: string
      action: "approve" | "reject"
      reason?: string
    }
  >({
    mutationFn: ({ applicationId, action, reason }) =>
      applicationRequest(
        `/api/admin/organizer-applications/${applicationId}/${action}`,
        isOrganizerApplication,
        reason !== undefined ? { reason } : {}
      ),
    onSuccess: async (updated) => {
      await queryClient.cancelQueries({ queryKey: adminApplicationsKey })
      for (const [key] of queryClient.getQueriesData<OrganizerApplicationPage>({
        queryKey: adminApplicationsKey,
      })) {
        const filters = key[2] as OrganizerApplicationFilters | undefined
        const statusFilter = filters?.status
        queryClient.setQueryData<OrganizerApplicationPage>(key, (page) => {
          if (!page) return page
          const items = page.items
            .map((item) =>
              item.applicationId === updated.applicationId ? updated : item
            )
            .filter(
              (item) =>
                statusFilter === undefined || item.status === statusFilter
            )
          return {
            ...page,
            items,
            total: Math.max(0, page.total - (page.items.length - items.length)),
          }
        })
      }
      void queryClient.invalidateQueries({
        queryKey: ["organizer-applications"],
      })
      void queryClient.invalidateQueries({ queryKey: ["organization"] })
    },
  })
}
