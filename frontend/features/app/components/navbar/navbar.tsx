"use client"

import {
  CalendarDaysIcon,
  CompassIcon,
  HeartIcon,
  LayoutDashboardIcon,
  MenuIcon,
  TagsIcon,
  TicketIcon,
  type LucideIcon,
} from "lucide-react"
import Link from "next/link"
import { usePathname } from "next/navigation"

import { Brand } from "@/features/app/components/navbar/brand"
import { UserMenu } from "@/features/app/components/navbar/user-menu"
import type { SessionUser, UserRole } from "@/features/auth"
import { Button } from "@/features/shared/components/ui/button"
import { Separator } from "@/features/shared/components/ui/separator"
import {
  Sheet,
  SheetContent,
  SheetTrigger,
} from "@/features/shared/components/ui/sheet"
import { cn } from "@/features/shared/lib/utils"

export type { UserRole }

export interface NavLink {
  label: string
  href: string
  icon: LucideIcon
}

interface NavbarProps {
  user: SessionUser | null
}

const visitorLinks: NavLink[] = [
  { label: "Discover", href: "/", icon: CompassIcon },
  { label: "My Tickets", href: "/tickets", icon: TicketIcon },
  { label: "Favorites", href: "/favorites", icon: HeartIcon },
  { label: "Calendar", href: "/calendar", icon: CalendarDaysIcon },
]

const navigationByRole: Record<UserRole, NavLink[]> = {
  guest: [
    { label: "Discover", href: "/", icon: CompassIcon },
    { label: "Categories", href: "/categories", icon: TagsIcon },
  ],
  visitor: visitorLinks,
  // Organizing is an extra ability, so organizers keep everything a visitor can do.
  organizer: [
    ...visitorLinks,
    { label: "Organizer", href: "/organizer", icon: LayoutDashboardIcon },
  ],
}

/** Returns the most specific matching href, so /organizer/events/new does not also light up /organizer/events. */
function activeHref(pathname: string, links: NavLink[]) {
  let active: string | undefined

  for (const link of links) {
    const matches =
      pathname === link.href || pathname.startsWith(`${link.href}/`)

    if (matches && (!active || link.href.length > active.length)) {
      active = link.href
    }
  }

  return active
}

export function Navbar({ user }: NavbarProps) {
  const pathname = usePathname()
  const role = user?.role ?? "guest"
  const links = navigationByRole[role]
  const active = activeHref(pathname, links)

  return (
    <header className="sticky top-0 z-50 w-full border-b border-border bg-background">
      <div className="mx-auto flex h-14 max-w-screen-xl items-center gap-4 px-4">
        <Brand user={user} />
        <nav className="hidden flex-1 items-center justify-center gap-2 md:flex lg:gap-4">
          {links.map((link) => {
            const Icon = link.icon

            return (
              <Button
                key={link.href}
                render={<Link href={link.href} />}
                nativeButton={false}
                variant="ghost"
                className={
                  link.href === active
                    ? "font-semibold text-foreground"
                    : "text-muted-foreground"
                }
              >
                <Icon />
                {link.label}
              </Button>
            )
          })}
        </nav>
        <div className="ml-auto flex items-center gap-2">
          {user ? (
            <UserMenu user={user} />
          ) : (
            <div className="hidden items-center gap-1 md:flex">
              <form method="get" action="/api/auth/login">
                <Button type="submit" variant="outline">
                  Log in
                </Button>
              </form>
              <form method="get" action="/api/auth/register">
                <Button type="submit">Register</Button>
              </form>
            </div>
          )}
          <Sheet>
            <SheetTrigger
              render={
                <Button variant="ghost" size="icon-lg" className="md:hidden" />
              }
              aria-label="Open menu"
            >
              <MenuIcon className="size-5" />
            </SheetTrigger>
            <SheetContent side="right" className="px-6 pt-10">
              <nav className="flex flex-col gap-1">
                {links.map((link) => (
                  <MobileLink
                    key={link.href}
                    link={link}
                    isActive={link.href === active}
                  />
                ))}
                {!user && (
                  <div className="mt-3 flex flex-col gap-2">
                    <Separator />
                    <form method="get" action="/api/auth/login">
                      <Button
                        type="submit"
                        variant="outline"
                        className="w-full"
                      >
                        Log in
                      </Button>
                    </form>
                    <form method="get" action="/api/auth/register">
                      <Button type="submit" className="w-full">
                        Register
                      </Button>
                    </form>
                  </div>
                )}
              </nav>
            </SheetContent>
          </Sheet>
        </div>
      </div>
    </header>
  )
}

function MobileLink({ link, isActive }: { link: NavLink; isActive: boolean }) {
  const Icon = link.icon

  return (
    <Button
      render={<Link href={link.href} />}
      nativeButton={false}
      variant="ghost"
      className={cn(
        "w-full justify-start",
        isActive ? "font-semibold text-foreground" : "text-muted-foreground"
      )}
    >
      <Icon />
      {link.label}
    </Button>
  )
}
