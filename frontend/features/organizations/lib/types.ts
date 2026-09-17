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
    label: "Organization admin",
    description: "Manage organization settings and members.",
  },
  {
    value: "event_manager",
    label: "Event manager",
    description: "Create, edit and publish events.",
  },
  {
    value: "finance_viewer",
    label: "Finance viewer",
    description: "View ticket sales and financial reports.",
  },
];

export interface OrganizationMember {
  id: string;
  name: string;
  email: string;
  joinedAt: string | null;
  rights: OrganizationRight[];
}
