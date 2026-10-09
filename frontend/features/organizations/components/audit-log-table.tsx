import type {
  AuditCategory,
  AuditLogEntry,
} from "@/features/organizations/lib/mock-data"
import { Badge } from "@/features/shared/components/ui/badge"
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/features/shared/components/ui/table"

const categoryVariant: Record<
  AuditCategory,
  "default" | "secondary" | "outline"
> = {
  member: "default",
  event: "secondary",
  settings: "outline",
  finance: "secondary",
}

const categoryLabel: Record<AuditCategory, string> = {
  member: "Member",
  event: "Event",
  settings: "Settings",
  finance: "Finance",
}

function formatTimestamp(value: string) {
  return new Date(value).toLocaleString(undefined, {
    year: "numeric",
    month: "short",
    day: "numeric",
    hour: "2-digit",
    minute: "2-digit",
  })
}

export function AuditLogTable({ entries }: { entries: AuditLogEntry[] }) {
  if (entries.length === 0) {
    return (
      <p className="py-6 text-center text-sm text-muted-foreground">
        No activity has been recorded for this organization yet.
      </p>
    )
  }

  return (
    <Table>
      <TableHeader>
        <TableRow>
          <TableHead>When</TableHead>
          <TableHead>Actor</TableHead>
          <TableHead>Action</TableHead>
          <TableHead>Category</TableHead>
        </TableRow>
      </TableHeader>
      <TableBody>
        {entries.map((entry) => (
          <TableRow key={entry.id}>
            <TableCell className="whitespace-nowrap text-muted-foreground tabular-nums">
              {formatTimestamp(entry.timestamp)}
            </TableCell>
            <TableCell className="text-muted-foreground">
              {entry.actor}
            </TableCell>
            <TableCell>
              <div className="flex flex-col">
                <span className="font-medium text-foreground">
                  {entry.action}
                </span>
                <span className="text-xs text-muted-foreground">
                  {entry.target}
                </span>
              </div>
            </TableCell>
            <TableCell>
              <Badge variant={categoryVariant[entry.category]}>
                {categoryLabel[entry.category]}
              </Badge>
            </TableCell>
          </TableRow>
        ))}
      </TableBody>
    </Table>
  )
}
