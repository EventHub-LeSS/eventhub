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
