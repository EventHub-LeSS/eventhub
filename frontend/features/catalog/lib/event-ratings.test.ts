import assert from "node:assert/strict"
import { test } from "node:test"

import {
  isEventRatingsPage,
  parseReviewPage,
  reviewPageHref,
} from "@/features/catalog/lib/event-ratings"

const rating = {
  ratingId: "12345678-1234-1234-1234-123456789abc",
  score: 4,
  text: "",
  authorName: null,
  createdAt: "2026-05-01T12:00:00Z",
}

test("event ratings pages accept the backend shape with nullable author", () => {
  const page = { items: [rating], page: 1, limit: 10, total: 1 }
  assert.ok(isEventRatingsPage(page))
  assert.ok(isEventRatingsPage({ ...page, items: [], total: 0 }))
  assert.ok(
    isEventRatingsPage({ ...page, items: [{ ...rating, authorName: "Max" }] })
  )
})

test("event ratings pages reject malformed ratings and paging", () => {
  const page = { items: [rating], page: 1, limit: 10, total: 1 }
  for (const invalid of [
    null,
    [],
    { ...page, items: null },
    { ...page, items: [{ ...rating, score: 0 }] },
    { ...page, items: [{ ...rating, score: 4.5 }] },
    { ...page, items: [{ ...rating, text: null }] },
    { ...page, items: [{ ...rating, authorName: undefined }] },
    { ...page, page: 0 },
    { ...page, limit: 101 },
    { ...page, total: -1 },
  ]) {
    assert.equal(isEventRatingsPage(invalid), false, JSON.stringify(invalid))
  }
})

test("review pages fall back to the first page for invalid input", () => {
  assert.equal(parseReviewPage("3"), 3)
  for (const invalid of [
    undefined,
    "",
    "0",
    "-1",
    "1.5",
    "abc",
    "01",
    ["2", "3"],
    "9".repeat(20),
  ]) {
    assert.equal(parseReviewPage(invalid), 1, String(invalid))
  }
})

test("review page links omit the parameter for the first page", () => {
  assert.equal(reviewPageHref("abc", 1), "/events/abc")
  assert.equal(reviewPageHref("abc", 2), "/events/abc?reviewPage=2")
  assert.equal(reviewPageHref("a/b", 2), "/events/a%2Fb?reviewPage=2")
})
