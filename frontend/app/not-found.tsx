import Link from "next/link"

import { Button } from "@/features/shared/components/ui/button"

export default function NotFound() {
  return (
    <main className="flex flex-1 items-center justify-center px-6 py-12">
      <div className="flex w-full max-w-md flex-col items-start gap-6">
        <div className="flex flex-col gap-2">
          <h1 className="text-2xl font-semibold tracking-tight">
            404 – Seite nicht gefunden
          </h1>
          <p className="text-base text-muted-foreground">
            Diese Seite existiert nicht oder ist nicht mehr verfügbar.
          </p>
        </div>
        <Button render={<Link href="/" />} nativeButton={false}>
          Zur Startseite
        </Button>
      </div>
    </main>
  )
}
