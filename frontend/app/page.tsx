import { Suspense } from "react"

import { EventFilters, EventList } from "@/features/catalog"

interface PageProps {
  searchParams: Promise<{ location?: string; date?: string }>
}

export default async function Page({ searchParams }: PageProps) {
  const { location, date } = await searchParams

  return (
    <div className="mx-auto flex w-full max-w-screen-xl flex-1 flex-col gap-6 p-6">
      <div>
        <h1 className="text-2xl font-semibold tracking-tight">
          Entdecke Veranstaltungen in deiner Region
        </h1>
      </div>
      <Suspense fallback={null}>
        <EventFilters />
      </Suspense>
      <EventList filter={{ location, date }} />
    </div>
  )
}
