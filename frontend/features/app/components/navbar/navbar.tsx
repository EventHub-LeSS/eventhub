"use client"

import {
  CalendarDaysIcon,
  CirclePlusIcon,
  CompassIcon,
  HeartIcon,
  LayoutDashboardIcon,
  MenuIcon,
  TagsIcon,
  TicketIcon,
  TicketsIcon,
  type LucideIcon,
} from "lucide-react"
import Link from "next/link"
import { usePathname } from "next/navigation"

import { Avatar, AvatarFallback } from "@/features/shared/components/ui/avatar"
import { Button } from "@/features/shared/components/ui/button"
import { Separator } from "@/features/shared/components/ui/separator"
import {
  Sheet,
  SheetContent,
  SheetTrigger,
} from "@/features/shared/components/ui/sheet"
import { cn } from "@/features/shared/lib/utils"

export type UserRole = "guest" | "visitor" | "organizer"

export interface NavLink {
  label: string
  href: string
  icon: LucideIcon
}

interface NavbarProps {
  role?: UserRole
}

const navigationByRole: Record<UserRole, NavLink[]> = {
  guest: [
    { label: "Discover", href: "/", icon: CompassIcon },
    { label: "Categories", href: "/categories", icon: TagsIcon },
  ],
  visitor: [
    { label: "Discover", href: "/", icon: CompassIcon },
    { label: "My Tickets", href: "/tickets", icon: TicketIcon },
    { label: "Favorites", href: "/favorites", icon: HeartIcon },
    { label: "Calendar", href: "/calendar", icon: CalendarDaysIcon },
  ],
  organizer: [
    { label: "Dashboard", href: "/organizer", icon: LayoutDashboardIcon },
    { label: "My Events", href: "/organizer/events", icon: TicketsIcon },
    {
      label: "Create Event",
      href: "/organizer/events/new",
      icon: CirclePlusIcon,
    },
  ],
}

function isActiveLink(pathname: string, href: string) {
  if (href === "/" || href === "/organizer") {
    return pathname === href
  }

  return pathname === href || pathname.startsWith(`${href}/`)
}

export function Navbar({ role = "visitor" }: NavbarProps) {
  const pathname = usePathname()
  const links = navigationByRole[role]
  const isAuthenticated = role !== "guest"

  return (
    <header className="sticky top-0 z-50 w-full border-b border-border bg-background">
      <div className="mx-auto flex h-14 max-w-screen-xl items-center gap-4 px-4">
        <Link
          href="/"
          className="flex shrink-0 items-center gap-2 font-semibold"
        >
          <TicketsIcon className="size-5" />
          <span>EventHub</span>
        </Link>
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
                  isActiveLink(pathname, link.href)
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
        <div className="ml-auto flex items-center gap-1">
          {isAuthenticated ? (
            <Avatar>
              <AvatarFallback>U</AvatarFallback>
            </Avatar>
          ) : (
            <div className="hidden items-center gap-1 md:flex">
              <Button
                render={<Link href="/login" />}
                nativeButton={false}
                variant="outline"
              >
                Log in
              </Button>
              <Button render={<Link href="/register" />} nativeButton={false}>
                Register
              </Button>
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
                  <MobileLink key={link.href} link={link} pathname={pathname} />
                ))}
                {!isAuthenticated && (
                  <div className="mt-3 flex flex-col gap-2">
                    <Separator />
                    <Button
                      render={<Link href="/login" />}
                      nativeButton={false}
                      variant="outline"
                    >
                      Log in
                    </Button>
                    <Button
                      render={<Link href="/register" />}
                      nativeButton={false}
                    >
                      Register
                    </Button>
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

function MobileLink({ link, pathname }: { link: NavLink; pathname: string }) {
  const Icon = link.icon

  return (
    <Button
      render={<Link href={link.href} />}
      nativeButton={false}
      variant="ghost"
      className={cn(
        "w-full justify-start",
        isActiveLink(pathname, link.href)
          ? "font-semibold text-foreground"
          : "text-muted-foreground"
      )}
    >
      <Icon />
      {link.label}
    </Button>
  )
}
