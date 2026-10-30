import type {
  AuditAction,
  AuditLogEntry,
  AuditPhase,
} from "@/features/organizations/lib/types"
import { organizationRights } from "@/features/organizations/lib/types"
import { Badge } from "@/features/shared/components/ui/badge"
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/features/shared/components/ui/table"

const actionLabel: Record<AuditAction, string> = {
  "event.created": "Veranstaltung erstellt",
  "event.updated": "Veranstaltung bearbeitet",
  "event.published": "Veranstaltung veröffentlicht",
  "event.cancelled": "Veranstaltung abgesagt",
  "event.deleted": "Veranstaltung gelöscht",
  "organization.member_roles_changed": "Rechte geändert",
}

const categoryLabel: Record<AuditLogEntry["resourceType"], string> = {
  event: "Veranstaltung",
  organization_member: "Mitglied",
}

const categoryVariant: Record<
  AuditLogEntry["resourceType"],
  "default" | "secondary"
> = {
  event: "secondary",
  organization_member: "default",
}

const phaseBadge: Partial<
  Record<AuditPhase, { label: string; variant: "outline" | "destructive" }>
> = {
  started: { label: "Unbestätigt", variant: "outline" },
  incomplete: { label: "Unvollständig", variant: "destructive" },
}

const fieldLabel: Record<string, string> = {
  title: "Titel",
  description: "Beschreibung",
  startTime: "Beginn",
  endTime: "Ende",
  capacity: "Kapazität",
  status: "Status",
  price: "Preis",
  categoryId: "Kategorie",
  locationId: "Ort",
}

const rightLabel = new Map<string, string>(
  organizationRights.map((right) => [right.value, right.label])
)

function formatTimestamp(value: string) {
  return new Date(value).toLocaleString("de-DE", {
    year: "numeric",
    month: "short",
    day: "numeric",
    hour: "2-digit",
    minute: "2-digit",
  })
}

function asRecord(value: unknown): Record<string, unknown> | null {
  return value && typeof value === "object" && !Array.isArray(value)
    ? (value as Record<string, unknown>)
    : null
}

function asStrings(value: unknown): string[] {
  return Array.isArray(value)
    ? value.filter((item): item is string => typeof item === "string")
    : []
}

function rights(values: string[]) {
  return values.map((value) => rightLabel.get(value) ?? value).join(", ")
}

function describeChange(entry: AuditLogEntry): string {
  const { changes } = entry

  if (entry.action === "organization.member_roles_changed") {
    const target =
      typeof changes.targetUsername === "string" ? changes.targetUsername : ""
    const concluded = entry.phase === "succeeded" || entry.phase === "incomplete"
    const granted = asStrings(concluded ? changes.granted : changes.toGrant)
    const revoked = asStrings(concluded ? changes.revoked : changes.toRevoke)
    const parts = [
      granted.length > 0 && `+ ${rights(granted)}`,
      revoked.length > 0 && `− ${rights(revoked)}`,
    ].filter(Boolean)
    return [target, parts.join(" · ") || "keine Änderung"]
      .filter(Boolean)
      .join(": ")
  }

  const event = asRecord(changes.event)
  if (event && typeof event.title === "string") {
    return event.title
  }

  const fields = asRecord(changes.fields)
  if (fields) {
    const names = Object.keys(fields).map((field) => fieldLabel[field] ?? field)
    if (names.length > 0) {
      return `Geändert: ${names.join(", ")}`
    }
  }

  return entry.resourceId
}

/**
 * Operations in Keycloak are logged as a "started" entry plus a result entry. The log is newest
 * first, so the first entry per operation is its latest state; a lone "started" stays unconfirmed.
 */
function latestPerOperation(entries: AuditLogEntry[]) {
  const seen = new Set<string>()
  return entries.filter((entry) => {
    if (seen.has(entry.operationId)) {
      return false
    }
    seen.add(entry.operationId)
    return true
  })
}

export function AuditLogTable({ entries }: { entries: AuditLogEntry[] }) {
  if (entries.length === 0) {
    return (
      <p className="py-6 text-center text-sm text-muted-foreground">
        Für diese Organisation wurde noch keine Aktivität erfasst.
      </p>
    )
  }

  return (
    <Table>
      <TableHeader>
        <TableRow>
          <TableHead>Zeitpunkt</TableHead>
          <TableHead>Person</TableHead>
          <TableHead>Aktion</TableHead>
          <TableHead>Kategorie</TableHead>
        </TableRow>
      </TableHeader>
      <TableBody>
        {latestPerOperation(entries).map((entry) => {
          const phase = phaseBadge[entry.phase]
          return (
            <TableRow key={entry.id}>
              <TableCell className="whitespace-nowrap text-muted-foreground tabular-nums">
                {formatTimestamp(entry.occurredAt)}
              </TableCell>
              <TableCell className="text-muted-foreground">
                {entry.actorUsername || entry.actorSubject}
              </TableCell>
              <TableCell>
                <div className="flex flex-col">
                  <span className="flex items-center gap-2 font-medium text-foreground">
                    {actionLabel[entry.action] ?? entry.action}
                    {phase && (
                      <Badge variant={phase.variant}>{phase.label}</Badge>
                    )}
                  </span>
                  <span className="text-xs text-muted-foreground">
                    {describeChange(entry)}
                  </span>
                </div>
              </TableCell>
              <TableCell>
                <Badge variant={categoryVariant[entry.resourceType] ?? "outline"}>
                  {categoryLabel[entry.resourceType] ?? entry.resourceType}
                </Badge>
              </TableCell>
            </TableRow>
          )
        })}
      </TableBody>
    </Table>
  )
}
