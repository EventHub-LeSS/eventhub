"use client"

import { XIcon } from "lucide-react"
import { usePathname, useRouter, useSearchParams } from "next/navigation"
import { useRef, useState } from "react"

import type { EventCategory } from "@/features/catalog/lib/get-published-events"
import { Button } from "@/features/shared/components/ui/button"
import { Input } from "@/features/shared/components/ui/input"
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/features/shared/components/ui/select"

const ALL_CATEGORIES = "all"

interface EventFiltersProps {
  categories: EventCategory[]
}

export function EventFilters({ categories }: EventFiltersProps) {
  const router = useRouter()
  const pathname = usePathname()
  const searchParams = useSearchParams()

  const [title, setTitle] = useState(searchParams.get("title") ?? "")
  const [location, setLocation] = useState(searchParams.get("location") ?? "")
  const [date, setDate] = useState(searchParams.get("date") ?? "")
  const [categoryId, setCategoryId] = useState(
    searchParams.get("categoryId") ?? ALL_CATEGORIES
  )

  const hasActiveFilter =
    searchParams.has("title") ||
    searchParams.has("location") ||
    searchParams.has("date") ||
    searchParams.has("categoryId")

  const categoryItems: Record<string, string> = {
    [ALL_CATEGORIES]: "Alle Kategorien",
  }
  for (const category of categories) {
    categoryItems[category.categoryId] = category.category
  }

  function applyFilters(next: {
    title: string
    location: string
    date: string
    categoryId: string
  }) {
    const params = new URLSearchParams(searchParams)

    if (next.title) {
      params.set("title", next.title)
    } else {
      params.delete("title")
    }

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

    if (next.categoryId && next.categoryId !== ALL_CATEGORIES) {
      params.set("categoryId", next.categoryId)
    } else {
      params.delete("categoryId")
    }

    const query = params.toString()
    router.replace(query ? `${pathname}?${query}` : pathname)
  }

  // Shared across fields, so typing in one resets the pending apply from another.
  const debounceRef = useRef<ReturnType<typeof setTimeout>>(undefined)

  function scheduleFilters(next: {
    title: string
    location: string
    date: string
    categoryId: string
  }) {
    clearTimeout(debounceRef.current)
    debounceRef.current = setTimeout(() => applyFilters(next), 300)
  }

  function handleSubmit(event: React.FormEvent<HTMLFormElement>) {
    event.preventDefault()
    clearTimeout(debounceRef.current)
    applyFilters({ title, location, date, categoryId })
  }

  function handleReset() {
    clearTimeout(debounceRef.current)
    setTitle("")
    setLocation("")
    setDate("")
    setCategoryId(ALL_CATEGORIES)
    applyFilters({ title: "", location: "", date: "", categoryId: "" })
  }

  return (
    <form
      onSubmit={handleSubmit}
      className="flex flex-col gap-3 rounded-xl border border-border bg-card p-4 sm:flex-row sm:flex-wrap sm:items-end"
    >
      <div className="flex min-w-[180px] flex-[2] flex-col gap-1.5">
        <label
          htmlFor="event-filter-title"
          className="text-xs font-medium text-muted-foreground"
        >
          Titel
        </label>
        <Input
          id="event-filter-title"
          placeholder="Nach Titel suchen…"
          value={title}
          onChange={(event) => {
            const value = event.target.value
            setTitle(value)
            scheduleFilters({ title: value, location, date, categoryId })
          }}
        />
      </div>
      <div className="flex min-w-[160px] flex-1 flex-col gap-1.5">
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
          onChange={(event) => {
            const value = event.target.value
            setLocation(value)
            scheduleFilters({ title, location: value, date, categoryId })
          }}
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
          onChange={(event) => {
            const value = event.target.value
            setDate(value)
            scheduleFilters({ title, location, date: value, categoryId })
          }}
        />
      </div>
      <div className="flex flex-col gap-1.5">
        <label
          htmlFor="event-filter-category"
          className="text-xs font-medium text-muted-foreground"
        >
          Kategorie
        </label>
        <Select
          items={categoryItems}
          value={categoryId}
          onValueChange={(value) => {
            const next = value ?? ALL_CATEGORIES
            setCategoryId(next)
            // A picked option is a discrete action, not continuous typing, so
            // it applies right away instead of waiting for the debounce.
            clearTimeout(debounceRef.current)
            applyFilters({ title, location, date, categoryId: next })
          }}
        >
          <SelectTrigger id="event-filter-category" className="w-full">
            <SelectValue placeholder="Alle Kategorien" />
          </SelectTrigger>
          <SelectContent alignItemWithTrigger={false}>
            <SelectItem value={ALL_CATEGORIES}>Alle Kategorien</SelectItem>
            {categories.map((category) => (
              <SelectItem key={category.categoryId} value={category.categoryId}>
                {category.category}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
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
