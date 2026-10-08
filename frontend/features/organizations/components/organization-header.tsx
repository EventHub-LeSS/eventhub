"use client"

import {
  CheckIcon,
  GlobeIcon,
  PencilIcon,
  ShieldCheckIcon,
  ShieldOffIcon,
  UserPlusIcon,
} from "lucide-react"
import { useState } from "react"

import { useInviteMember } from "@/features/organizations/lib/api"
import type { Organization } from "@/features/organizations/lib/types"
import { Avatar, AvatarFallback } from "@/features/shared/components/ui/avatar"
import { Badge } from "@/features/shared/components/ui/badge"
import { Button } from "@/features/shared/components/ui/button"
import { Card, CardContent } from "@/features/shared/components/ui/card"
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/features/shared/components/ui/dialog"
import { Field, FieldError, FieldLabel } from "@/features/shared/components/ui/field"
import { Input } from "@/features/shared/components/ui/input"
import { Separator } from "@/features/shared/components/ui/separator"
import { toast } from "@/features/shared/components/ui/toast"
import { cn } from "@/features/shared/lib/utils"

const EMAIL_PATTERN = /^[^\s@]+@[^\s@]+\.[^\s@]+$/

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

export function OrganizationHeader({
  organization,
  alias,
  isEditing = false,
  onEditClick,
  onSaveClick,
}: {
  organization: Organization
  alias: string
  isEditing?: boolean
  onEditClick?: () => void
  onSaveClick?: () => void
}) {
  return (
    <Card className="h-fit">
      <CardContent className="flex flex-col gap-4">
        <Avatar size="lg" className="size-28">
          <AvatarFallback className="text-2xl">
            {initials(organization.name)}
          </AvatarFallback>
        </Avatar>

        <div className="flex flex-col gap-1">
          <h1 className="text-xl font-semibold">{organization.name}</h1>
          <p className="text-sm text-muted-foreground">@{organization.alias}</p>
        </div>

        {organization.description && (
          <p className="text-sm text-muted-foreground">
            {organization.description}
          </p>
        )}

        <div className="flex flex-col gap-2">
          <InviteMemberDialog alias={alias} />
          {isEditing ? (
            <Button
              onClick={onSaveClick}
              className={cn(
                "w-full border-emerald-600 bg-emerald-600 text-white",
                "hover:bg-emerald-500",
                "focus-visible:border-emerald-600 focus-visible:ring-emerald-600/50",
                "dark:bg-emerald-500 dark:hover:bg-emerald-400"
              )}
            >
              <CheckIcon />
              Save changes
            </Button>
          ) : (
            <Button variant="outline" className="w-full" onClick={onEditClick}>
              <PencilIcon />
              Edit organization
            </Button>
          )}
        </div>

        <Separator />

        <ul className="flex flex-col gap-2.5 text-sm text-muted-foreground">
          <li className="flex items-center gap-2">
            {organization.enabled ? (
              <ShieldCheckIcon className="size-4 shrink-0" />
            ) : (
              <ShieldOffIcon className="size-4 shrink-0" />
            )}
            {organization.enabled ? "Enabled" : "Disabled"}
          </li>
          {organization.domains.map((domain) => (
            <li key={domain.name} className="flex items-center gap-2">
              <GlobeIcon className="size-4 shrink-0" />
              <span className="truncate">{domain.name}</span>
              {!domain.verified && (
                <Badge
                  variant="outline"
                  className="h-4 shrink-0 px-1.5 text-[10px]"
                >
                  Unverified
                </Badge>
              )}
            </li>
          ))}
        </ul>
      </CardContent>
    </Card>
  )
}

function InviteMemberDialog({ alias }: { alias: string }) {
  const [open, setOpen] = useState(false)
  const [email, setEmail] = useState("")
  const [name, setName] = useState("")
  const [error, setError] = useState<string | undefined>()

  const inviteMember = useInviteMember(alias)

  function handleOpenChange(nextOpen: boolean) {
    setOpen(nextOpen)

    if (!nextOpen) {
      setEmail("")
      setName("")
      setError(undefined)
    }
  }

  function handleSubmit(event: React.FormEvent<HTMLFormElement>) {
    event.preventDefault()

    const trimmedEmail = email.trim()

    if (!EMAIL_PATTERN.test(trimmedEmail)) {
      setError("Enter a valid email address.")
      return
    }

    setError(undefined)
    inviteMember.mutate(
      { email: trimmedEmail, name: name.trim() || undefined },
      {
        onSuccess: () => {
          toast.add({
            type: "success",
            title: "Invitation sent",
            description: `${trimmedEmail} will see this organization once they accept.`,
          })
          handleOpenChange(false)
        },
        onError: (mutationError) => setError(mutationError.message),
      }
    )
  }

  return (
    <Dialog open={open} onOpenChange={handleOpenChange}>
      <Button variant="outline" className="w-full" onClick={() => setOpen(true)}>
        <UserPlusIcon />
        Invite member
      </Button>
      <DialogContent>
        <form onSubmit={handleSubmit} className="flex flex-col gap-5">
          <DialogHeader>
            <DialogTitle>Invite member</DialogTitle>
            <DialogDescription>
              Keycloak sends them an email invitation. Membership stays
              pending until they accept — events, sales and billing already
              belong to the organization either way.
            </DialogDescription>
          </DialogHeader>

          <Field data-invalid={Boolean(error)}>
            <FieldLabel htmlFor="invite-email">Email</FieldLabel>
            <Input
              id="invite-email"
              type="email"
              autoComplete="off"
              value={email}
              placeholder="name@example.com"
              aria-invalid={Boolean(error)}
              onChange={(event) => {
                setEmail(event.target.value)
                setError(undefined)
              }}
            />
            <FieldError>{error}</FieldError>
          </Field>

          <Field>
            <FieldLabel htmlFor="invite-name">Name</FieldLabel>
            <Input
              id="invite-name"
              value={name}
              placeholder="Optional"
              autoComplete="off"
              onChange={(event) => setName(event.target.value)}
            />
          </Field>

          <DialogFooter>
            <Button
              type="button"
              variant="outline"
              onClick={() => handleOpenChange(false)}
              disabled={inviteMember.isPending}
            >
              Cancel
            </Button>
            <Button type="submit" disabled={inviteMember.isPending}>
              {inviteMember.isPending ? "Sending…" : "Send invite"}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}
