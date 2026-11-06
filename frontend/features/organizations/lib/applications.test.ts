import assert from "node:assert/strict"
import { test } from "node:test"

import {
  applicationQuery,
  emptyApplicationDraft,
  isApplicationUUID,
  isOrganizerApplication,
  isOrganizerApplicationList,
  isOrganizerApplicationPage,
  parseApplicationFilters,
  parseApplicationSubmissionBody,
  toApplicationPayload,
  validateApplicationDraft,
  validateDecisionPayload,
} from "@/features/organizations/lib/applications"

test("validateApplicationDraft and toApplicationPayload enforce backend constraints and normalize fields", () => {
  const validDraft = {
    ...emptyApplicationDraft,
    organizationName: "  Stadtfest Bremen e.V.  ",
    organizationAlias: "  Stadtfest-Bremen  ",
    contactEmail: "  info@stadtfest.de  ",
    city: "  Bremen ",
    countryCode: " de ",
    street: "   ",
    message: "  Wir veranstalten jedes Jahr das Stadtfest in Bremen.  ",
  }

  assert.deepEqual(validateApplicationDraft(validDraft), {})
  assert.deepEqual(toApplicationPayload(validDraft), {
    organizationName: "Stadtfest Bremen e.V.",
    organizationAlias: "stadtfest-bremen",
    contactEmail: "info@stadtfest.de",
    city: "Bremen",
    countryCode: "DE",
    message: "Wir veranstalten jedes Jahr das Stadtfest in Bremen.",
  })

  const parsed = parseApplicationSubmissionBody(validDraft)
  assert.equal(parsed.error, undefined)
  assert.equal(parsed.payload?.organizationAlias, "stadtfest-bremen")

  const invalidErrors = validateApplicationDraft({
    ...emptyApplicationDraft,
    organizationName: "A",
    organizationAlias: "-bad-",
    contactEmail: "not-an-email",
    countryCode: "DEU",
    message: "zu kurz",
  })
  assert.ok(invalidErrors.organizationName)
  assert.ok(invalidErrors.organizationAlias)
  assert.ok(invalidErrors.contactEmail)
  assert.ok(invalidErrors.countryCode)
  assert.ok(invalidErrors.message)
})

test("parseApplicationFilters and applicationQuery round-trip and reject invalid values", () => {
  assert.deepEqual(parseApplicationFilters(new URLSearchParams()), {
    filters: { page: 1, limit: 50 },
  })

  const filters = {
    page: 2,
    limit: 25,
    status: "pending" as const,
  }
  assert.deepEqual(
    parseApplicationFilters(new URLSearchParams(applicationQuery(filters))),
    { filters }
  )

  for (const query of [
    "status=unknown",
    "page=0",
    "page=-1",
    "limit=0",
    "limit=101",
    "page=9007199254740991&limit=100",
  ]) {
    assert.ok(parseApplicationFilters(new URLSearchParams(query)).error, query)
  }
})

test("validateDecisionPayload handles optional approval notes and required rejection reasons", () => {
  assert.deepEqual(validateDecisionPayload({}, "approve"), { payload: {} })
  assert.deepEqual(validateDecisionPayload({ reason: "   " }, "approve"), {
    payload: {},
  })
  assert.deepEqual(
    validateDecisionPayload({ reason: "  Willkommen! " }, "approve"),
    { payload: { reason: "Willkommen!" } }
  )
  assert.ok(
    validateDecisionPayload({ reason: "a".repeat(1001) }, "approve").error
  )

  assert.ok(validateDecisionPayload({}, "reject").error)
  assert.ok(validateDecisionPayload({ reason: "   " }, "reject").error)
  assert.deepEqual(
    validateDecisionPayload({ reason: "  Impressum fehlt " }, "reject"),
    { payload: { reason: "Impressum fehlt" } }
  )
  const emoji = String.fromCodePoint(0x1f600)
  assert.equal(
    validateDecisionPayload({ reason: emoji.repeat(1000) }, "reject").error,
    undefined
  )
  assert.ok(
    validateDecisionPayload({ reason: emoji.repeat(1001) }, "reject").error
  )
})

test("organizer application response guards validate items and pages", () => {
  const sample = {
    applicationId: "12345678-1234-1234-1234-123456789abc",
    userId: "87654321-4321-4321-4321-cba987654321",
    organizationName: "Stadtfest Bremen",
    organizationAlias: "stadtfest-bremen",
    contactEmail: "info@stadtfest.de",
    contactPhoneNumber: null,
    street: null,
    houseNumber: null,
    postalCode: null,
    city: "Bremen",
    countryCode: "DE",
    message: "Wir veranstalten das Stadtfest.",
    status: "pending",
    decisionReason: null,
    decidedBy: null,
    decidedAt: null,
    organizationId: null,
    createdAt: "2026-11-06T20:00:00Z",
    updatedAt: "2026-11-06T20:00:00Z",
  }

  assert.ok(isApplicationUUID(sample.applicationId))
  assert.equal(isApplicationUUID("not-a-uuid"), false)
  assert.ok(isOrganizerApplication(sample))
  assert.ok(isOrganizerApplicationList([sample]))
  assert.ok(
    isOrganizerApplicationPage({
      items: [sample],
      page: 1,
      limit: 50,
      total: 1,
    })
  )
  assert.equal(
    isOrganizerApplicationPage({
      items: [{}],
      page: 1,
      limit: 50,
      total: 1,
    }),
    false
  )
})
