"use client"

import { XIcon } from "lucide-react"
import { usePathname, useRouter, useSearchParams } from "next/navigation"
import { useState } from "react"

import { Button } from "@/features/shared/components/ui/button"
import { Input } from "@/features/shared/components/ui/input"

export function EventFilters() {
  const router = useRouter()
  const pathname = usePathname()
  const searchParams = useSearchParams()

  const [location, setLocation] = useState(searchParams.get("location") ?? "")
  const [date, setDate] = useState(searchParams.get("date") ?? "")

  const hasActiveFilter =
    searchParams.has("location") || searchParams.has("date")

  function applyFilters(next: { location: string; date: string }) {
    const params = new URLSearchParams(searchParams)

    if (next.location) {
      params.set("location", next.location)
    } else {
      params.delete("location")
    }

    if (next.date) {
      params.set("date", next.date)
    } else {
      params.delete("date")
    }

    const query = params.toString()
    router.push(query ? `${pathname}?${query}` : pathname)
  }

  function handleSubmit(event: React.FormEvent<HTMLFormElement>) {
    event.preventDefault()
    applyFilters({ location, date })
  }

  function handleReset() {
    setLocation("")
    setDate("")
    applyFilters({ location: "", date: "" })
  }

  return (
    <form
      onSubmit={handleSubmit}
      className="flex flex-col gap-3 rounded-xl border border-border bg-card p-4 sm:flex-row sm:items-end"
    >
      <div className="flex flex-1 flex-col gap-1.5">
        <label
          htmlFor="event-filter-location"
          className="text-xs font-medium text-muted-foreground"
        >
          Ort
        </label>
        <Input
          id="event-filter-location"
          placeholder="Stadt oder Veranstaltungsort"
          value={location}
          onChange={(event) => setLocation(event.target.value)}
        />
      </div>
      <div className="flex flex-col gap-1.5">
        <label
          htmlFor="event-filter-date"
          className="text-xs font-medium text-muted-foreground"
        >
          Datum
        </label>
        <Input
          id="event-filter-date"
          type="date"
          value={date}
          onChange={(event) => setDate(event.target.value)}
        />
      </div>
      <div className="flex gap-2">
        <Button type="submit">Filtern</Button>
        {hasActiveFilter && (
          <Button type="button" variant="outline" onClick={handleReset}>
            <XIcon />
            Zurücksetzen
          </Button>
        )}
      </div>
    </form>
  )
}
