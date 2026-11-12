import assert from "node:assert/strict"
import { test } from "node:test"

import {
  emptyDraft,
  toDraft,
  toDraftPayload,
} from "@/features/events/lib/draft"
import {
  formatEventDateTime,
  formatEventTimeRange,
} from "@/features/events/lib/format"
import type { Event } from "@/features/events/lib/types"

for (const [date, expectedStart, expectedEnd] of [
  ["2027-01-15", "2027-01-15T11:00:00.000Z", "2027-01-15T12:00:00.000Z"],
  ["2027-07-15", "2027-07-15T10:00:00.000Z", "2027-07-15T11:00:00.000Z"],
  ["2027-03-28", "2027-03-28T10:00:00.000Z", "2027-03-28T11:00:00.000Z"],
  ["2027-10-31", "2027-10-31T11:00:00.000Z", "2027-10-31T12:00:00.000Z"],
]) {
  test(`event times stay in Berlin time when saving and editing ${date}`, () => {
    const draft = {
      ...emptyDraft,
      startTime: `${date}T12:00`,
      endTime: `${date}T13:00`,
    }
    const payload = toDraftPayload(draft)

    assert.equal(payload.startTime, expectedStart)
    assert.equal(payload.endTime, expectedEnd)
    assert.match(formatEventDateTime(payload.startTime), /12:00/)
    assert.match(formatEventDateTime(payload.endTime), /13:00/)
    assert.match(
      formatEventTimeRange(payload.startTime, payload.endTime),
      /12:00.*13:00/
    )
    const endingAtNoon = toDraftPayload({
      ...draft,
      startTime: `${date}T11:00`,
      endTime: `${date}T12:00`,
    })
    assert.match(
      formatEventTimeRange(endingAtNoon.startTime, endingAtNoon.endTime),
      /11:00.*12:00/
    )

    const event: Event = {
      ...payload,
      eventId: "event-id",
      description: payload.description ?? null,
      status: "draft",
      withdrawnByModerator: false,
      organizerId: null,
      locationId: null,
      createdAt: expectedStart,
      updatedAt: expectedStart,
    }
    const restored = toDraft(event, [])
    assert.equal(restored.startTime, draft.startTime)
    assert.equal(restored.endTime, draft.endTime)
  })
}
