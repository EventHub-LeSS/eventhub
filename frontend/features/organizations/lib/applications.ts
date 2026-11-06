export const organizerApplicationStatuses = [
  "pending",
  "approved",
  "rejected",
] as const

export type OrganizerApplicationStatus =
  (typeof organizerApplicationStatuses)[number]

export const organizerApplicationStatusLabels: Record<
  OrganizerApplicationStatus,
  string
> = {
  pending: "Ausstehend",
  approved: "Angenommen",
  rejected: "Abgelehnt",
}

export interface OrganizerApplication {
  applicationId: string
  userId: string
  organizationName: string
  organizationAlias: string
  contactEmail: string
  contactPhoneNumber: string | null
  street: string | null
  houseNumber: string | null
  postalCode: string | null
  city: string | null
  countryCode: string | null
  message: string
  status: OrganizerApplicationStatus
  decisionReason: string | null
  decidedBy: string | null
  decidedAt: string | null
  organizationId: string | null
  createdAt: string
  updatedAt: string
}

export interface OrganizerApplicationPage {
  items: OrganizerApplication[]
  page: number
  limit: number
  total: number
}

export interface OrganizerApplicationFilters {
  page: number
  limit: number
  status?: OrganizerApplicationStatus
}

export interface OrganizerApplicationDraft {
  organizationName: string
  organizationAlias: string
  contactEmail: string
  contactPhoneNumber: string
  street: string
  houseNumber: string
  postalCode: string
  city: string
  countryCode: string
  message: string
}

export interface SubmitOrganizerApplicationPayload {
  organizationName: string
  organizationAlias: string
  contactEmail: string
  contactPhoneNumber?: string
  street?: string
  houseNumber?: string
  postalCode?: string
  city?: string
  countryCode?: string
  message: string
}

export type ApplicationDraftErrors = Partial<
  Record<keyof OrganizerApplicationDraft, string>
>

export const emptyApplicationDraft: OrganizerApplicationDraft = {
  organizationName: "",
  organizationAlias: "",
  contactEmail: "",
  contactPhoneNumber: "",
  street: "",
  houseNumber: "",
  postalCode: "",
  city: "",
  countryCode: "",
  message: "",
}

export const MAX_DECISION_REASON_LENGTH = 1000
export const MAX_APPLICATION_MESSAGE_LENGTH = 5000

const APPLICATION_ALIAS_PATTERN = /^[a-z0-9][a-z0-9-]{1,48}[a-z0-9]$/
const EMAIL_PATTERN = /^[^\s@]+@[^\s@]+\.[^\s@]+$/
const COUNTRY_CODE_PATTERN = /^[A-Z]{2}$/
const UUID_PATTERN =
  /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i

function codePointLength(value: string): number {
  return Array.from(value).length
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return value !== null && typeof value === "object" && !Array.isArray(value)
}

function isNullableString(value: unknown): value is string | null {
  return value === null || typeof value === "string"
}

export function isApplicationUUID(value: string): boolean {
  return UUID_PATTERN.test(value)
}

export function isOrganizerApplicationStatus(
  value: unknown
): value is OrganizerApplicationStatus {
  return (
    typeof value === "string" &&
    organizerApplicationStatuses.some((status) => status === value)
  )
}

export function isOrganizerApplication(
  value: unknown
): value is OrganizerApplication {
  return (
    isRecord(value) &&
    typeof value.applicationId === "string" &&
    typeof value.userId === "string" &&
    typeof value.organizationName === "string" &&
    typeof value.organizationAlias === "string" &&
    typeof value.contactEmail === "string" &&
    isNullableString(value.contactPhoneNumber) &&
    isNullableString(value.street) &&
    isNullableString(value.houseNumber) &&
    isNullableString(value.postalCode) &&
    isNullableString(value.city) &&
    isNullableString(value.countryCode) &&
    typeof value.message === "string" &&
    isOrganizerApplicationStatus(value.status) &&
    isNullableString(value.decisionReason) &&
    (value.decidedBy === undefined || isNullableString(value.decidedBy)) &&
    isNullableString(value.decidedAt) &&
    isNullableString(value.organizationId) &&
    typeof value.createdAt === "string" &&
    typeof value.updatedAt === "string"
  )
}

export function isOrganizerApplicationList(
  value: unknown
): value is OrganizerApplication[] {
  return Array.isArray(value) && value.every(isOrganizerApplication)
}

export function isOrganizerApplicationPage(
  value: unknown
): value is OrganizerApplicationPage {
  return (
    isRecord(value) &&
    Array.isArray(value.items) &&
    value.items.every(isOrganizerApplication) &&
    typeof value.page === "number" &&
    Number.isSafeInteger(value.page) &&
    value.page >= 1 &&
    typeof value.limit === "number" &&
    Number.isSafeInteger(value.limit) &&
    value.limit >= 1 &&
    value.limit <= 100 &&
    typeof value.total === "number" &&
    Number.isSafeInteger(value.total) &&
    value.total >= 0
  )
}

export function validateApplicationDraft(
  draft: OrganizerApplicationDraft
): ApplicationDraftErrors {
  const errors: ApplicationDraftErrors = {}

  const name = draft.organizationName.trim()
  const nameLen = codePointLength(name)
  if (nameLen === 0) {
    errors.organizationName = "Bitte gib den Namen der Organisation ein."
  } else if (nameLen < 2 || nameLen > 200) {
    errors.organizationName =
      "Der Name der Organisation muss zwischen 2 und 200 Zeichen lang sein."
  }

  const alias = draft.organizationAlias.trim().toLowerCase()
  if (!alias) {
    errors.organizationAlias = "Bitte gib einen Alias ein."
  } else if (!APPLICATION_ALIAS_PATTERN.test(alias)) {
    errors.organizationAlias =
      "Der Alias muss 3–50 Zeichen lang sein (Kleinbuchstaben, Ziffern oder Bindestriche) und mit Buchstabe oder Ziffer beginnen und enden."
  }

  const email = draft.contactEmail.trim()
  if (!email) {
    errors.contactEmail = "Bitte gib eine Kontakt-E-Mail-Adresse ein."
  } else if (!EMAIL_PATTERN.test(email)) {
    errors.contactEmail = "Bitte gib eine gültige E-Mail-Adresse ein."
  }

  const message = draft.message.trim()
  const messageLen = codePointLength(message)
  if (messageLen === 0) {
    errors.message =
      "Bitte beschreibe kurz eure Organisation und die geplanten Veranstaltungen."
  } else if (messageLen < 10 || messageLen > MAX_APPLICATION_MESSAGE_LENGTH) {
    errors.message =
      "Die Nachricht muss zwischen 10 und 5.000 Zeichen lang sein."
  }

  const optionalLimits: Array<{
    key: keyof OrganizerApplicationDraft
    max: number
    label: string
  }> = [
    { key: "contactPhoneNumber", max: 50, label: "Die Telefonnummer" },
    { key: "street", max: 200, label: "Die Straße" },
    { key: "houseNumber", max: 20, label: "Die Hausnummer" },
    { key: "postalCode", max: 20, label: "Die Postleitzahl" },
    { key: "city", max: 100, label: "Die Stadt" },
  ]

  for (const field of optionalLimits) {
    const trimmed = draft[field.key].trim()
    if (trimmed && codePointLength(trimmed) > field.max) {
      errors[field.key] =
        `${field.label} darf höchstens ${field.max} Zeichen lang sein.`
    }
  }

  const country = draft.countryCode.trim().toUpperCase()
  if (country && !COUNTRY_CODE_PATTERN.test(country)) {
    errors.countryCode =
      "Bitte gib einen zweistelligen Ländercode nach ISO 3166-1 an (z. B. DE)."
  }

  return errors
}

export function toApplicationPayload(
  draft: OrganizerApplicationDraft
): SubmitOrganizerApplicationPayload {
  const contactPhoneNumber = draft.contactPhoneNumber.trim()
  const street = draft.street.trim()
  const houseNumber = draft.houseNumber.trim()
  const postalCode = draft.postalCode.trim()
  const city = draft.city.trim()
  const countryCode = draft.countryCode.trim().toUpperCase()

  return {
    organizationName: draft.organizationName.trim(),
    organizationAlias: draft.organizationAlias.trim().toLowerCase(),
    contactEmail: draft.contactEmail.trim(),
    message: draft.message.trim(),
    ...(contactPhoneNumber ? { contactPhoneNumber } : {}),
    ...(street ? { street } : {}),
    ...(houseNumber ? { houseNumber } : {}),
    ...(postalCode ? { postalCode } : {}),
    ...(city ? { city } : {}),
    ...(countryCode ? { countryCode } : {}),
  }
}

export function parseApplicationSubmissionBody(
  body: unknown
):
  | { payload: SubmitOrganizerApplicationPayload; error?: never }
  | { payload?: never; error: string } {
  if (!isRecord(body)) {
    return { error: "Ungültige Formulardaten." }
  }

  const draft: OrganizerApplicationDraft = {
    organizationName:
      typeof body.organizationName === "string" ? body.organizationName : "",
    organizationAlias:
      typeof body.organizationAlias === "string" ? body.organizationAlias : "",
    contactEmail:
      typeof body.contactEmail === "string" ? body.contactEmail : "",
    contactPhoneNumber:
      typeof body.contactPhoneNumber === "string"
        ? body.contactPhoneNumber
        : "",
    street: typeof body.street === "string" ? body.street : "",
    houseNumber: typeof body.houseNumber === "string" ? body.houseNumber : "",
    postalCode: typeof body.postalCode === "string" ? body.postalCode : "",
    city: typeof body.city === "string" ? body.city : "",
    countryCode: typeof body.countryCode === "string" ? body.countryCode : "",
    message: typeof body.message === "string" ? body.message : "",
  }

  const errors = validateApplicationDraft(draft)
  const firstError = Object.values(errors).find(Boolean)
  if (firstError) {
    return { error: firstError }
  }

  return { payload: toApplicationPayload(draft) }
}

export function parseApplicationFilters(
  query: URLSearchParams
):
  | { filters: OrganizerApplicationFilters; error?: never }
  | { filters?: never; error: string } {
  const page = query.get("page") ?? "1"
  const limit = query.get("limit") ?? "50"
  if (
    !/^[1-9]\d*$/.test(page) ||
    !Number.isSafeInteger(Number(page)) ||
    !/^[1-9]\d*$/.test(limit) ||
    Number(limit) > 100 ||
    !Number.isSafeInteger((Number(page) - 1) * Number(limit))
  ) {
    return { error: "Ungültige Seitennummer oder Seitengröße (1–100)." }
  }

  const status = query.get("status")
  if (status !== null && !isOrganizerApplicationStatus(status)) {
    return { error: "Ungültiger Antragsstatus." }
  }

  return {
    filters: {
      page: Number(page),
      limit: Number(limit),
      ...(status !== null && isOrganizerApplicationStatus(status)
        ? { status }
        : {}),
    },
  }
}

export function applicationQuery(filters: OrganizerApplicationFilters): string {
  const query = new URLSearchParams({
    page: String(filters.page),
    limit: String(filters.limit),
  })
  if (filters.status) {
    query.set("status", filters.status)
  }
  return query.toString()
}

export function validateDecisionPayload(
  value: unknown,
  action: "approve" | "reject"
):
  | { payload: { reason?: string }; error?: never }
  | { payload?: never; error: string } {
  if (action === "approve") {
    if (value === null || value === undefined) {
      return { payload: {} }
    }
    if (!isRecord(value)) {
      return { error: "Ungültige Anfrage." }
    }
    if (value.reason === undefined || value.reason === null) {
      return { payload: {} }
    }
    if (typeof value.reason !== "string") {
      return { error: "Der Hinweis muss ein Text sein." }
    }
    const trimmed = value.reason.trim()
    if (codePointLength(trimmed) > MAX_DECISION_REASON_LENGTH) {
      return {
        error: `Der Hinweis darf höchstens ${MAX_DECISION_REASON_LENGTH} Zeichen enthalten.`,
      }
    }
    return { payload: trimmed ? { reason: trimmed } : {} }
  }

  if (!isRecord(value) || typeof value.reason !== "string") {
    return { error: "Bitte gib eine Begründung für die Ablehnung an." }
  }
  const trimmed = value.reason.trim()
  const length = codePointLength(trimmed)
  if (length === 0) {
    return { error: "Bitte gib eine Begründung für die Ablehnung an." }
  }
  if (length > MAX_DECISION_REASON_LENGTH) {
    return {
      error: `Die Begründung darf höchstens ${MAX_DECISION_REASON_LENGTH} Zeichen enthalten.`,
    }
  }
  return { payload: { reason: trimmed } }
}
