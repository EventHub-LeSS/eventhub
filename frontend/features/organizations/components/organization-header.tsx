import {
  CheckIcon,
  GlobeIcon,
  PencilIcon,
  ShieldCheckIcon,
  ShieldOffIcon,
  UserPlusIcon,
} from "lucide-react"

import type { Organization } from "@/features/organizations/lib/types"
import { Avatar, AvatarFallback } from "@/features/shared/components/ui/avatar"
import { Badge } from "@/features/shared/components/ui/badge"
import { Button } from "@/features/shared/components/ui/button"
import { Card, CardContent } from "@/features/shared/components/ui/card"
import { Separator } from "@/features/shared/components/ui/separator"
import { cn } from "@/features/shared/lib/utils"

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
  isEditing = false,
  onEditClick,
  onSaveClick,
}: {
  organization: Organization
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
          <Button variant="outline" className="w-full">
            <UserPlusIcon />
            Invite member
          </Button>
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
