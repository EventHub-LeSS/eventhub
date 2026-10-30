export type OrganizationRight = "org_admin" | "event_manager" | "finance_viewer";

export interface OrganizationRightOption {
  value: OrganizationRight;
  label: string;
  description: string;
}

export const organizationRights: OrganizationRightOption[] = [
  {
    value: "org_admin",
    label: "Organisations-Admin",
    description: "Einstellungen und Mitglieder der Organisation verwalten.",
  },
  {
    value: "event_manager",
    label: "Event-Manager",
    description: "Veranstaltungen erstellen, bearbeiten und veröffentlichen.",
  },
  {
    value: "finance_viewer",
    label: "Finanzeinsicht",
    description: "Ticketverkäufe und Finanzberichte einsehen.",
  },
];

export interface OrganizationMember {
  id: string;
  username: string;
  name: string;
  email: string;
  joinedAt: string | null;
  rights: OrganizationRight[];
}

export type AuditAction =
  | "event.created"
  | "event.updated"
  | "event.published"
  | "event.cancelled"
  | "event.deleted"
  | "organization.member_roles_changed";

export type AuditPhase = "completed" | "started" | "succeeded" | "incomplete";

export interface AuditLogEntry {
  id: string;
  operationId: string;
  occurredAt: string;
  actorSubject: string;
  actorUsername: string;
  organizationId: string;
  action: AuditAction;
  resourceType: "event" | "organization_member";
  resourceId: string;
  phase: AuditPhase;
  changes: Record<string, unknown>;
}

export interface AuditLogPage {
  items: AuditLogEntry[];
  nextCursor: string | null;
}
