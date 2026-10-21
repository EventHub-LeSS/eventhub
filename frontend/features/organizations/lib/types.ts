export interface OrganizationDomain {
  name: string;
  verified: boolean;
}

export interface Organization {
  id: string;
  name: string;
  alias: string;
  description: string;
  enabled: boolean;
  domains: OrganizationDomain[];
}

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
