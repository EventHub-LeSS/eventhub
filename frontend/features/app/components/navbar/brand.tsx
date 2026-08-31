"use client"

import { CheckIcon, ChevronsUpDownIcon, TicketsIcon } from "lucide-react"
import Link from "next/link"
import { usePathname } from "next/navigation"

import type { SessionUser } from "@/features/auth"
import { Button } from "@/features/shared/components/ui/button"
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuGroup,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuTrigger,
} from "@/features/shared/components/ui/dropdown-menu"

function BrandContent({
  organization,
  switchable,
}: {
  organization?: string | null
  switchable?: boolean
}) {
  return (
    <>
      <TicketsIcon className="size-5" />
      <span className="flex flex-col items-start leading-tight">
        <span className="text-sm font-semibold">EventHub</span>
        {organization && (
          <span className="text-xs font-normal text-muted-foreground">
            {organization}
          </span>
        )}
      </span>
      {switchable && (
        <ChevronsUpDownIcon className="size-3 shrink-0 text-muted-foreground" />
      )}
    </>
  )
}

/** With several organizations the whole brand becomes the switcher, so it cannot also link home. */
export function Brand({ user }: { user: SessionUser | null }) {
  const pathname = usePathname()

  if (!user || user.organizations.length < 2) {
    return (
      <Link
        href="/"
        className="flex shrink-0 items-center gap-2 rounded-lg outline-none focus-visible:ring-3 focus-visible:ring-ring/50"
      >
        <BrandContent organization={user?.activeOrganization} />
      </Link>
    )
  }

  return (
    <DropdownMenu>
      <DropdownMenuTrigger
        render={
          <Button variant="ghost" className="h-auto shrink-0 gap-2 px-2 py-1" />
        }
        aria-label="Switch organization"
      >
        <BrandContent organization={user.activeOrganization} switchable />
      </DropdownMenuTrigger>
      <DropdownMenuContent align="start" className="w-56">
        <DropdownMenuGroup>
          <DropdownMenuLabel>Switch organization</DropdownMenuLabel>
          <form method="post" action="/api/auth/organization">
            <input type="hidden" name="returnTo" value={pathname} />
            {user.organizations.map((organization) => (
              <DropdownMenuItem
                key={organization}
                nativeButton
                render={
                  <button
                    type="submit"
                    name="organization"
                    value={organization}
                    aria-label={`Switch to ${organization}`}
                  />
                }
                className="w-full justify-between"
              >
                {organization}
                {organization === user.activeOrganization && <CheckIcon />}
              </DropdownMenuItem>
            ))}
          </form>
        </DropdownMenuGroup>
      </DropdownMenuContent>
    </DropdownMenu>
  )
}
