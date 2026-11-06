export const globalRoles = ["admin", "moderator", "visitor"] as const

export type GlobalRole = (typeof globalRoles)[number]

export const roleLabels: Record<GlobalRole, string> = {
  admin: "Admin",
  moderator: "Moderator",
  visitor: "Visitor",
}

export function isGlobalRole(value: unknown): value is GlobalRole {
  return globalRoles.some((role) => role === value)
}

export interface AdminUser {
  userId: string
  firstName: string
  lastName: string
  email: string
  roles: GlobalRole[]
}

export interface AdminUsersPage {
  items: AdminUser[]
  page: number
  limit: number
  total: number
}
