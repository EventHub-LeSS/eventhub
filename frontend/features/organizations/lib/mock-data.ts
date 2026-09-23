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

export interface SalesPoint {
  month: string
  revenue: number
}

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

export const mockSales: SalesPoint[] = [
  { month: "Apr", revenue: 480 },
  { month: "May", revenue: 920 },
  { month: "Jun", revenue: 610 },
  { month: "Jul", revenue: 1540 },
  { month: "Aug", revenue: 1180 },
  { month: "Sep", revenue: 2340 },
]

export type AuditCategory = "member" | "event" | "settings" | "finance"

export interface AuditLogEntry {
  id: string
  timestamp: string
  actor: string
  action: string
  target: string
  category: AuditCategory
}

export const mockAuditLog: AuditLogEntry[] = [
  {
    id: "1",
    timestamp: "2026-09-23T09:14:00Z",
    actor: "anna.weber@uni.example",
    action: "Granted right",
    target: "event_manager to lars.hoff@uni.example",
    category: "member",
  },
  {
    id: "2",
    timestamp: "2026-09-22T16:48:00Z",
    actor: "lars.hoff@uni.example",
    action: "Published event",
    target: "Welcome Week Kickoff",
    category: "event",
  },
  {
    id: "3",
    timestamp: "2026-09-22T11:05:00Z",
    actor: "anna.weber@uni.example",
    action: "Updated settings",
    target: "Organization name",
    category: "settings",
  },
  {
    id: "4",
    timestamp: "2026-09-21T14:30:00Z",
    actor: "maria.koch@uni.example",
    action: "Exported report",
    target: "Sales report Q3",
    category: "finance",
  },
  {
    id: "5",
    timestamp: "2026-09-20T08:22:00Z",
    actor: "anna.weber@uni.example",
    action: "Invited member",
    target: "maria.koch@uni.example",
    category: "member",
  },
  {
    id: "6",
    timestamp: "2026-09-18T19:02:00Z",
    actor: "lars.hoff@uni.example",
    action: "Created event",
    target: "Winter Hackathon",
    category: "event",
  },
  {
    id: "7",
    timestamp: "2026-09-17T10:41:00Z",
    actor: "anna.weber@uni.example",
    action: "Revoked right",
    target: "finance_viewer from tom.berg@uni.example",
    category: "member",
  },
  {
    id: "8",
    timestamp: "2026-09-15T13:57:00Z",
    actor: "anna.weber@uni.example",
    action: "Removed member",
    target: "tom.berg@uni.example",
    category: "member",
  },
  {
    id: "9",
    timestamp: "2026-09-14T07:35:00Z",
    actor: "lars.hoff@uni.example",
    action: "Cancelled event",
    target: "Alumni Brunch",
    category: "event",
  },
  {
    id: "10",
    timestamp: "2026-09-12T15:19:00Z",
    actor: "anna.weber@uni.example",
    action: "Enabled organization",
    target: "Organization status",
    category: "settings",
  },
]
