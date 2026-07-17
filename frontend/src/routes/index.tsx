import { createFileRoute } from "@tanstack/react-router";
import { ArrowRight, CalendarDays, Ticket, Users } from "lucide-react";

import { Button } from "@/components/ui/button";

export const Route = createFileRoute("/")({
  component: Landing,
});

const features = [
  {
    icon: CalendarDays,
    title: "Einfach planen",
    description:
      "Erstelle und plane Events in wenigen Minuten mit einem klaren, fokussierten Ablauf.",
  },
  {
    icon: Ticket,
    title: "Tickets verkaufen",
    description:
      "Verwalte Anmeldungen und Ticketing, ohne die Plattform zu verlassen.",
  },
  {
    icon: Users,
    title: "Publikum gewinnen",
    description:
      "Teile deine Events und bring die Menschen rund um das zusammen, was zählt.",
  },
];

function Landing() {
  return (
    <main className="mx-auto max-w-6xl px-4 sm:px-6">
      {/* Hero */}
      <section className="flex flex-col items-center py-20 text-center sm:py-28">
        <h1 className="max-w-3xl text-4xl font-semibold tracking-tight text-foreground sm:text-6xl">
          Wo großartige Events{" "}
          <span className="bg-gradient-to-r from-[#F5A623] to-[#7B2FB5] bg-clip-text text-transparent">
            zusammenkommen
          </span>
        </h1>

        <p className="mt-6 max-w-xl text-lg text-muted-foreground">
          EventHub ist der einfache Weg, deine Events zu erstellen, zu verwalten
          und zu teilen. Von der ersten Idee bis zum letzten Gast.
        </p>

        <div className="mt-10 flex flex-col gap-3 sm:flex-row">
          <Button size="lg">
            Loslegen
            <ArrowRight className="size-4" />
          </Button>
          <Button size="lg" variant="outline">
            Events entdecken
          </Button>
        </div>
      </section>

      {/* Features */}
      <section className="grid gap-6 pb-24 sm:grid-cols-3">
        {features.map((feature) => (
          <div
            key={feature.title}
            className="rounded-xl border border-border bg-card p-6 text-left shadow-xs transition-shadow hover:shadow-md"
          >
            <div className="mb-4 inline-flex size-11 items-center justify-center rounded-lg bg-accent text-primary">
              <feature.icon className="size-5" />
            </div>
            <h3 className="text-lg font-semibold text-card-foreground">
              {feature.title}
            </h3>
            <p className="mt-2 text-sm text-muted-foreground">
              {feature.description}
            </p>
          </div>
        ))}
      </section>
    </main>
  );
}
