import assert from "node:assert/strict"
import { test } from "node:test"

import {
  isRatingPage,
  isVisibilityResponse,
} from "@/features/moderation/lib/types"
import {
  isUUID,
  moderationQuery,
  parseModerationFilters,
  reasonError,
} from "@/features/moderation/lib/validation"

test("moderation reasons reject whitespace and count trimmed Unicode code points", () => {
  assert.ok(reasonError(" \n\t "))
  assert.equal(reasonError("  geprüft  "), undefined)
  assert.equal(reasonError("a".repeat(1000)), undefined)
  assert.ok(reasonError("a".repeat(1001)))
  const supplementaryCharacter = String.fromCodePoint(0x1f600)
  assert.equal(
    reasonError(` ${supplementaryCharacter.repeat(1000)} `),
    undefined
  )
  assert.ok(reasonError(supplementaryCharacter.repeat(1001)))
})

test("moderation filters use supported backend defaults and round-trip", () => {
  assert.deepEqual(parseModerationFilters(new URLSearchParams()), {
    filters: { page: 1, limit: 50 },
  })
  const filters = {
    page: 2,
    limit: 100,
    hidden: false,
    eventId: "12345678-1234-1234-1234-123456789abc",
  }
  assert.deepEqual(
    parseModerationFilters(new URLSearchParams(moderationQuery(filters))),
    { filters }
  )
  assert.ok(isUUID(filters.eventId))
  assert.equal(isUUID("../events"), false)
  assert.deepEqual(
    parseModerationFilters(new URLSearchParams("organization=ignored")),
    { filters: { page: 1, limit: 50 } }
  )
})

test("moderation filters reject invalid boolean, UUID, page sizes and overflow", () => {
  for (const query of [
    "hidden=1",
    "hidden=",
    "eventId=invalid",
    "eventId=",
    "page=0",
    "page=-1",
    "page=1.5",
    "page=Infinity",
    "page=9007199254740992",
    "page=9007199254740991&limit=100",
    "limit=0",
    "limit=101",
    "limit=1.5",
    "limit=",
  ]) {
    assert.ok(parseModerationFilters(new URLSearchParams(query)).error, query)
  }
})

test("moderation response guards accept empty pages but reject incomplete data", () => {
  assert.equal(isRatingPage({ items: [], page: 1, limit: 50, total: 0 }), true)
  assert.equal(
    isRatingPage({ items: null, page: 1, limit: 50, total: 0 }),
    false
  )
  assert.equal(
    isRatingPage({ items: [{}], page: 1, limit: 50, total: 1 }),
    false
  )
  assert.equal(
    isRatingPage({ items: [], page: 0, limit: 101, total: -1 }),
    false
  )
  assert.equal(
    isVisibilityResponse({ ratingId: "id", hidden: true, changed: false }),
    true
  )
  assert.equal(isVisibilityResponse({ ratingId: "id", hidden: true }), false)
})
