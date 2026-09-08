export interface MockOrganization {
  name: string
  alias: string
  description: string
  contactEmail: string
  enabled: boolean
  domains: { name: string; verified: boolean }[]
  createdAt: string
}

export type MemberRole = "owner" | "admin" | "organizer" | "member"
export type MemberStatus = "active" | "invited"

export type OrganizationRight = "org_admin" | "event_manager" | "finance_viewer"

export interface OrganizationRightOption {
  value: OrganizationRight
  label: string
  description: string
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
]

export interface OrganizationMember {
  id: string
  name: string
  email: string
  role: MemberRole
  status: MemberStatus
  joinedAt: string
  rights: OrganizationRight[]
}

export type EventStatus = "upcoming" | "past" | "draft"

export interface OrganizationEvent {
  id: string
  name: string
  date: string
  location: string
  status: EventStatus
  ticketsSold: number
  capacity: number
}

export interface ActivityPoint {
  month: string
  created: number
  held: number
}

export const mockOrganization: MockOrganization = {
  name: "Fachschaft Informatik",
  alias: "fachschaft-informatik",
  description:
    "Student council for Computer Science. We organize socials, workshops and the yearly welcome event for first semesters.",
  contactEmail: "kontakt@fs-inf.uni-example.de",
  enabled: true,
  domains: [
    { name: "fs-inf.uni-example.de", verified: true },
    { name: "informatik.uni-example.de", verified: false },
  ],
  createdAt: "2023-09-12",
}

export const mockMembers: OrganizationMember[] = [
  {
    id: "1",
    name: "Finn Betz",
    email: "finn.betz@grossmeister.de",
    role: "owner",
    status: "active",
    joinedAt: "2023-09-12",
    rights: ["org_admin", "event_manager", "finance_viewer"],
  },
  {
    id: "2",
    name: "Mara Lindqvist",
    email: "mara.lindqvist@fs-inf.uni-example.de",
    role: "admin",
    status: "active",
    joinedAt: "2023-10-02",
    rights: ["org_admin", "event_manager"],
  },
  {
    id: "3",
    name: "Jonah Weber",
    email: "jonah.weber@fs-inf.uni-example.de",
    role: "organizer",
    status: "active",
    joinedAt: "2024-01-18",
    rights: ["event_manager"],
  },
  {
    id: "4",
    name: "Priya Nair",
    email: "priya.nair@fs-inf.uni-example.de",
    role: "organizer",
    status: "active",
    joinedAt: "2024-03-05",
    rights: ["event_manager", "finance_viewer"],
  },
  {
    id: "5",
    name: "Tom Achterberg",
    email: "tom.achterberg@fs-inf.uni-example.de",
    role: "member",
    status: "active",
    joinedAt: "2024-05-21",
    rights: [],
  },
  {
    id: "6",
    name: "Lea Vogel",
    email: "lea.vogel@informatik.uni-example.de",
    role: "member",
    status: "invited",
    joinedAt: "2025-08-30",
    rights: [],
  },
]

export const mockEvents: OrganizationEvent[] = [
  {
    id: "1",
    name: "Welcome Week Kickoff",
    date: "2026-10-05",
    location: "Audimax",
    status: "upcoming",
    ticketsSold: 182,
    capacity: 250,
  },
  {
    id: "2",
    name: "Winter Hackathon",
    date: "2026-11-14",
    location: "Building E, Room 12",
    status: "upcoming",
    ticketsSold: 64,
    capacity: 120,
  },
  {
    id: "3",
    name: "Career Night with Industry Partners",
    date: "2026-09-24",
    location: "Mensa Hall",
    status: "draft",
    ticketsSold: 0,
    capacity: 200,
  },
  {
    id: "4",
    name: "Summer BBQ",
    date: "2026-07-11",
    location: "Campus Garden",
    status: "past",
    ticketsSold: 140,
    capacity: 150,
  },
  {
    id: "5",
    name: "Intro to Rust Workshop",
    date: "2026-05-02",
    location: "Building E, Room 4",
    status: "past",
    ticketsSold: 38,
    capacity: 40,
  },
]

export const mockActivity: ActivityPoint[] = [
  { month: "Apr", created: 2, held: 1 },
  { month: "May", created: 3, held: 2 },
  { month: "Jun", created: 2, held: 1 },
  { month: "Jul", created: 4, held: 2 },
  { month: "Aug", created: 3, held: 1 },
  { month: "Sep", created: 5, held: 3 },
]
