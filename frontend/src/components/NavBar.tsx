import { Link } from "@tanstack/react-router";
import { CalendarDays, LogIn } from "lucide-react";

import { Button } from "@/components/ui/button";

export function NavBar() {
  return (
    <header className="sticky top-0 z-50 w-full border-b border-border bg-background/80 backdrop-blur-md">
      <div className="mx-auto flex h-16 max-w-6xl items-center justify-between px-4 sm:px-6">
        {/* Brand — top left */}
        <Link
          to="/"
          className="flex items-center gap-2 text-lg font-semibold tracking-tight text-foreground transition-opacity hover:opacity-80"
        >
          <CalendarDays className="size-5 text-primary" />
          <span>
            event<span className="text-primary">hub</span>
          </span>
        </Link>

        {/* Auth actions — top right */}
        <nav className="flex items-center gap-2 sm:gap-4">
          <Link
            to="/"
            className="hidden text-sm font-medium text-muted-foreground transition-colors hover:text-foreground sm:inline-block"
          >
            Registrieren
          </Link>
          <Button size="sm">
            <LogIn className="size-4" />
            Anmelden
          </Button>
        </nav>
      </div>
    </header>
  );
}
