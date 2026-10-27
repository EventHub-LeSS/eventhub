import "server-only"

import { getSession } from "@/features/auth"
import { getSoldTickets } from "@/features/events/lib/api"
import {
  getOrganizerEvents,
  type OrganizerEvent,
} from "@/features/events/lib/get-organizer-events"
import type { EventStatus } from "@/features/events/lib/types"

/** Statistics shown on the organizer dashboard, derived from the backend
 *  `/events/{eventId}/sold-tickets` endpoint (confirmed bookings only). */
export interface EventStatisticsData {
  /** Confirmed tickets — cancellations are excluded by the backend. */
  soldTickets: number
  /** capacity - soldTickets, floored at 0 (matches the backend's rest capacity). */
  availableSeats: number
  /** soldTickets / capacity * 100, capped at 100 and rounded to two decimals. */
  occupancyPercent: number
}

export interface OrganizerDashboardEvent
  extends OrganizerEvent,
    EventStatisticsData {
  /** True when the per-event ticket figures could not be loaded, e.g. because
   *  the caller lacks the event_manager role for this event's organization.
   *  The event is still listed with its capacity. */
  statsUnavailable: boolean
  /** price × confirmed tickets, in EUR. 0 while `statsUnavailable`. */
  revenue: number
  /** Always a number after enrichment (from computeEventStatistics or fallback to capacity). */
  availableSeats: number
}

/** Filters of the organizer dashboard, taken from the page's search params. */
export interface OrganizerDashboardFilters {
  /** "all" keeps every status. */
  status: EventStatus | "all"
  /** Category id, or "all". */
  categoryId: string | "all"
  /** Case-insensitive substring of the title; "" keeps every title. */
  title: string
}

/** A category option of the dashboard's filter select. */
export interface OrganizerDashboardCategory {
  categoryId: string
  category: string
}

export type OrganizerDashboardResult =
  | {
      status: "ok"
      events: OrganizerDashboardEvent[]
      /** Categories present among the org's events, for the filter select. */
      categories: OrganizerDashboardCategory[]
      /** The filters that were actually applied, so the controls can show the
       *  same values the server filtered by. */
      filters: OrganizerDashboardFilters
      /** Number of the org's events before filtering, so the page can tell
       *  "no events yet" from "no events match the filters". */
      totalEvents: number
    }
  | { status: "unavailable" }

const EVENT_STATUSES: Record<EventStatus, true> = {
  draft: true,
  published: true,
  cancelled: true,
  completed: true,
}

/** Every filter value arrives from the URL, so unknown ones fall back to "all". */
export function parseDashboardFilters(params: {
  status?: string
  category?: string
  q?: string
}): OrganizerDashboardFilters {
  const { status } = params

  return {
    status:
      status && Object.hasOwn(EVENT_STATUSES, status)
        ? (status as EventStatus)
        : "all",
    categoryId: params.category || "all",
    title: params.q ?? "",
  }
}

/**
 * Mirrors `backend/internal/service/event_service.go` `GetEventStatistics`:
 * soldTickets = confirmed bookings, availableSeats = capacity - soldTickets
 * (floored at 0), occupancyPercent = soldTickets / capacity, capped at 100 and
 * rounded to two decimals. Kept in sync with the backend so the displayed rest
 * capacity and cancellation handling match the API exactly.
 */
export function computeEventStatistics(
  capacity: number,
  soldTickets: number
): EventStatisticsData {
  const availableSeats = Math.max(0, capacity - soldTickets)
  const ratio =
    capacity > 0 ? Math.min(1, Math.max(0, soldTickets / capacity)) : 0
  const occupancyPercent = Math.round(ratio * 10000) / 100

  return { soldTickets, availableSeats, occupancyPercent }
}

function matchesFilters(
  event: OrganizerEvent,
  filters: OrganizerDashboardFilters
): boolean {
  if (filters.status !== "all" && event.status !== filters.status) {
    return false
  }

  if (filters.categoryId !== "all" && event.categoryId !== filters.categoryId) {
    return false
  }

  const query = filters.title.trim().toLowerCase()

  return query === "" || event.title.toLowerCase().includes(query)
}

/** The organizer's events of the active organization, filtered and enriched
 *  with their confirmed ticket counts, occupancy figures and revenue. One
 *  sold-tickets request is issued per matching event (in parallel); a single
 *  failure degrades that event only — via `statsUnavailable` — and never the
 *  whole dashboard. */
export async function getOrganizerDashboardData(
  filters: OrganizerDashboardFilters
): Promise<OrganizerDashboardResult> {
  const events = await getOrganizerEvents()

  if (events.status !== "ok") {
    return { status: "unavailable" }
  }

  const session = await getSession()
  const accessToken = session?.accessToken ?? ""

  // The options come from every event rather than the filtered ones, so
  // narrowing a filter never removes the values another filter can offer.
  const categories = Array.from(
    new Map(
      events.events.flatMap((event) =>
        event.category
          ? [[event.category.categoryId, event.category] as const]
          : []
      )
    ).values()
  ).toSorted((a, b) => a.category.localeCompare(b.category))

  // A category id that is not among the org's events (a stale link, a deleted
  // category) falls back to "all": it neither hides every event nor reaches the
  // select, which only knows the ids the org actually uses.
  const appliedFilters: OrganizerDashboardFilters = {
    ...filters,
    categoryId:
      filters.categoryId === "all" ||
      categories.some((category) => category.categoryId === filters.categoryId)
        ? filters.categoryId
        : "all",
  }

  const matching = events.events.filter((event) =>
    matchesFilters(event, appliedFilters)
  )

  const enriched = await Promise.all(
    matching.map(async (event) => {
      try {
        const { soldTickets } = await getSoldTickets(
          accessToken,
          event.eventId
        )
        const price = Number(event.price)

        return {
          ...event,
          ...computeEventStatistics(event.capacity, soldTickets),
          revenue: Number.isFinite(price) ? price * soldTickets : 0,
          statsUnavailable: false,
        }
      } catch {
        return {
          ...event,
          soldTickets: 0,
          availableSeats: event.capacity,
          occupancyPercent: 0,
          revenue: 0,
          statsUnavailable: true,
        }
      }
    })
  )

  return {
    status: "ok",
    events: enriched,
    categories,
    filters: appliedFilters,
    totalEvents: events.events.length,
  }
}
