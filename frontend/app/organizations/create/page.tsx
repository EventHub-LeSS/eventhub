import { QuoteIcon, SparklesIcon } from "lucide-react";

import { requireAdmin } from "@/features/auth";
import { CreateOrganizationForm } from "@/features/organizations";

/**
 * Split layout: a banner explaining what an organization is for on the left,
 * the form on the right. Below lg the banner shrinks to its headline above
 * the form, so the form stays within reach on small screens.
 */
export default async function CreateOrganizationPage() {
  const user = await requireAdmin();

  return (
    <div className="grid flex-1 grid-cols-1 gap-6 p-6 lg:grid-cols-[minmax(0,1fr)_minmax(0,2fr)]">
      <aside className="relative flex flex-col justify-between gap-6 overflow-hidden rounded-2xl bg-muted/40 p-5 ring-1 ring-border lg:p-8">
        {/* A faint glow and fading dot grid, purely visual. */}
        <div
          aria-hidden="true"
          className="pointer-events-none absolute -top-32 -right-32 size-80 rounded-full bg-purple-500/10 blur-3xl"
        />
        <div
          aria-hidden="true"
          className="pointer-events-none absolute inset-0 text-foreground/15 [mask-image:linear-gradient(to_bottom,black,transparent_70%)]"
          style={{
            backgroundImage:
              "radial-gradient(circle, currentColor 1px, transparent 1px)",
            backgroundSize: "20px 20px",
          }}
        />

        <div className="relative flex flex-col gap-4">
          <span className="inline-flex w-fit items-center gap-1.5 text-xs font-medium tracking-wider text-muted-foreground uppercase">
            <SparklesIcon
              className="size-3.5 text-purple-500"
              aria-hidden="true"
            />
            EventHub für Veranstalter
          </span>
          <div className="flex flex-col gap-2">
            <p className="text-2xl font-bold tracking-tight text-balance lg:text-3xl">
              Deine Organisation. Deine Events.
            </p>
            <p className="max-w-sm text-sm text-muted-foreground">
              Eine Organisation bündelt dein Team, eure Veranstaltungen und alle
              Verkäufe an einem Ort.
            </p>
          </div>
        </div>

        <figure className="relative hidden flex-col gap-3 lg:flex">
          <QuoteIcon
            className="size-8 fill-purple-500/15 text-purple-500"
            aria-hidden="true"
          />
          <blockquote className="text-2xl font-semibold tracking-tight text-balance xl:text-3xl">
            Teile deine Veranstaltungen mit der Welt.
          </blockquote>
        </figure>
      </aside>

      <div className="flex justify-center lg:items-center">
        <div className="flex w-full max-w-4xl flex-col gap-6">
          <div>
            <h1 className="text-2xl font-bold tracking-tight">
              Organisation erstellen
            </h1>
            <p className="text-sm text-muted-foreground">
              Leg los und schaffe Großartiges.
            </p>
          </div>
          <CreateOrganizationForm defaultOrgAdmin={user.email} />
        </div>
      </div>
    </div>
  );
}
