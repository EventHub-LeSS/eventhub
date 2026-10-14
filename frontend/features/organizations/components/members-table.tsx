"use client"

import { PencilIcon } from "lucide-react"
import { useState } from "react"

import { useUpdateMemberRoles } from "@/features/organizations/lib/api"
import type {
  OrganizationMember,
  OrganizationRight,
} from "@/features/organizations/lib/types"
import { organizationRights } from "@/features/organizations/lib/types"
import { Avatar, AvatarFallback } from "@/features/shared/components/ui/avatar"
import { Badge } from "@/features/shared/components/ui/badge"
import { Button } from "@/features/shared/components/ui/button"
import { Checkbox } from "@/features/shared/components/ui/checkbox"
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/features/shared/components/ui/dialog"
import {
  Field,
  FieldContent,
  FieldDescription,
  FieldGroup,
  FieldLabel,
  FieldTitle,
} from "@/features/shared/components/ui/field"
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/features/shared/components/ui/table"

function initials(name: string) {
  return (
    name
      .split(/\s+/)
      .filter(Boolean)
      .slice(0, 2)
      .map((part) => part[0])
      .join("")
      .toUpperCase() || "?"
  )
}

const rightLabel: Record<OrganizationRight, string> = Object.fromEntries(
  organizationRights.map((right) => [right.value, right.label])
) as Record<OrganizationRight, string>

function formatDate(value: string | null) {
  if (!value) {
    return "—"
  }
  return new Date(value).toLocaleDateString(undefined, {
    year: "numeric",
    month: "short",
    day: "numeric",
  })
}

export function MembersTable({
  members,
  alias,
}: {
  members: OrganizationMember[]
  alias: string
}) {
  const [editingMember, setEditingMember] = useState<OrganizationMember | null>(
    null
  )
  const [draftRights, setDraftRights] = useState<OrganizationRight[]>([])
  const [error, setError] = useState<string | null>(null)

  const updateMemberRoles = useUpdateMemberRoles(alias)

  function openEditDialog(member: OrganizationMember) {
    setError(null)
    setEditingMember(member)
    setDraftRights(member.rights)
  }

  function closeEditDialog(open: boolean) {
    if (!open) {
      setEditingMember(null)
      setError(null)
    }
  }

  function toggleDraftRight(right: OrganizationRight, checked: boolean) {
    setDraftRights((prev) =>
      checked ? [...prev, right] : prev.filter((value) => value !== right)
    )
  }

  function handleSaveRights() {
    if (!editingMember) return
    setError(null)
    updateMemberRoles.mutate(
      { username: editingMember.username, roles: draftRights },
      {
        onSuccess: () => setEditingMember(null),
        onError: (mutationError) => setError(mutationError.message),
      }
    )
  }

  return (
    <>
      <Table>
        <TableHeader>
          <TableRow>
            <TableHead>Member</TableHead>
            <TableHead>Rights</TableHead>
            <TableHead className="text-right">Joined</TableHead>
            <TableHead className="w-0">
              <span className="sr-only">Actions</span>
            </TableHead>
          </TableRow>
        </TableHeader>
        <TableBody>
          {members.map((member) => (
            <TableRow key={member.id}>
              <TableCell>
                <div className="flex items-center gap-3">
                  <Avatar>
                    <AvatarFallback>{initials(member.name)}</AvatarFallback>
                  </Avatar>
                  <div className="flex flex-col">
                    <span className="font-medium text-foreground">
                      {member.name}
                    </span>
                    <span className="text-xs text-muted-foreground">
                      {member.email}
                    </span>
                  </div>
                </div>
              </TableCell>
              <TableCell>
                {member.rights.length > 0 ? (
                  <div className="flex flex-wrap gap-1">
                    {member.rights.map((right) => (
                      <Badge key={right} variant="outline">
                        {rightLabel[right]}
                      </Badge>
                    ))}
                  </div>
                ) : (
                  <span className="text-sm text-muted-foreground">—</span>
                )}
              </TableCell>
              <TableCell className="text-right text-muted-foreground">
                {formatDate(member.joinedAt)}
              </TableCell>
              <TableCell>
                <Button
                  variant="ghost"
                  size="icon-sm"
                  onClick={() => openEditDialog(member)}
                >
                  <PencilIcon />
                  <span className="sr-only">Edit rights for {member.name}</span>
                </Button>
              </TableCell>
            </TableRow>
          ))}
        </TableBody>
      </Table>

      <Dialog open={editingMember !== null} onOpenChange={closeEditDialog}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Edit organizational rights</DialogTitle>
            <DialogDescription>
              Choose the rights {editingMember?.name} has within this
              organization.
            </DialogDescription>
          </DialogHeader>

          <FieldGroup>
            {organizationRights.map((right) => (
              <FieldLabel key={right.value} htmlFor={`right-${right.value}`}>
                <Field orientation="horizontal">
                  <FieldContent>
                    <FieldTitle>{right.label}</FieldTitle>
                    <FieldDescription>{right.description}</FieldDescription>
                  </FieldContent>
                  <Checkbox
                    id={`right-${right.value}`}
                    checked={draftRights.includes(right.value)}
                    onCheckedChange={(checked) =>
                      toggleDraftRight(right.value, checked === true)
                    }
                  />
                </Field>
              </FieldLabel>
            ))}
          </FieldGroup>

          {error ? (
            <p className="text-sm text-destructive">{error}</p>
          ) : null}

          <DialogFooter>
            <Button
              variant="outline"
              onClick={() => setEditingMember(null)}
              disabled={updateMemberRoles.isPending}
            >
              Cancel
            </Button>
            <Button
              onClick={handleSaveRights}
              disabled={updateMemberRoles.isPending}
            >
              {updateMemberRoles.isPending ? "Saving…" : "Save changes"}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </>
  )
}
