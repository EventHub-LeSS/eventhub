"use client"

import { XIcon } from "lucide-react"
import { usePathname, useRouter } from "next/navigation"
import { useRef, useState } from "react"

import type { OrganizerDashboardFilters } from "@/features/events/lib/dashboard"
import { Button } from "@/features/shared/components/ui/button"
import { Input } from "@/features/shared/components/ui/input"
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/features/shared/components/ui/select"

const ALL = "all"

/** Mirrors the backend's event_status enum. A Record, not a list, so a status
 *  the enum does not know can never reach the select. */
const STATUS_ITEMS: Record<string, string> = {
  [ALL]: "Alle Status",
  draft: "Entwurf",
  published: "Veröffentlicht",
  completed: "Abgeschlossen",
  cancelled: "Abgebrochen",
}

interface DashboardFiltersProps {
  categories: { categoryId: string; category: string }[]
  /** The values the page already parsed and filtered by, so the controls show
   *  exactly what the server used and an unknown URL value never leaks in. */
  filters: OrganizerDashboardFilters
}

/**
 * Filters the Occupancy and Sales dashboards through the URL, so the page stays
 * a Server Component and a filtered view can be linked to. Status and category
 * apply on change; the title is submitted with the form.
 */
export function DashboardFilters({
  categories,
  filters,
}: DashboardFiltersProps) {
  const router = useRouter()
  const pathname = usePathname()

  const [title, setTitle] = useState(filters.title)
  const [status, setStatus] = useState<string>(filters.status)
  const [categoryId, setCategoryId] = useState(filters.categoryId)

  const hasActiveFilter = title !== "" || status !== ALL || categoryId !== ALL

  const categoryItems: Record<string, string> = { [ALL]: "Alle Kategorien" }
  for (const category of categories) {
    categoryItems[category.categoryId] = category.category
  }

  function applyFilters(next: {
    title: string
    status: string
    categoryId: string
  }) {
    // Rebuilt from scratch: the dashboard has no other query param to keep.
    const params = new URLSearchParams()
    const query = next.title.trim()

    if (query) {
      params.set("q", query)
    }

    if (next.status !== ALL) {
      params.set("status", next.status)
    }

    if (next.categoryId !== ALL) {
      params.set("category", next.categoryId)
    }

    const search = params.toString()
    router.push(search ? `${pathname}?${search}` : pathname)
  }
  
  const debounceRef = useRef<ReturnType<typeof setTimeout>>(undefined)

  function scheduleTitleFilter(nextTitle: string) {
    clearTimeout(debounceRef.current)
    debounceRef.current = setTimeout(
      () => applyFilters({ title: nextTitle, status, categoryId }),
      300
    )
  }

  function handleTitleChange(value: string) {
    setTitle(value)
    scheduleTitleFilter(value)
  }

  function handleSubmit(event: React.FormEvent<HTMLFormElement>) {
    event.preventDefault()
    clearTimeout(debounceRef.current)
    applyFilters({ title, status, categoryId })
  }

  function handleStatusChange(value: string | null) {
    const next = value ?? ALL
    setStatus(next)
    applyFilters({ title, status: next, categoryId })
  }

  function handleCategoryChange(value: string | null) {
    const next = value ?? ALL
    setCategoryId(next)
    applyFilters({ title, status, categoryId: next })
  }

  function handleReset() {
    clearTimeout(debounceRef.current)
    setTitle("")
    setStatus(ALL)
    setCategoryId(ALL)
    applyFilters({ title: "", status: ALL, categoryId: ALL })
  }

  return (
    <form
      onSubmit={handleSubmit}
      className="flex flex-col gap-3 rounded-xl border border-border bg-card p-4 sm:flex-row sm:flex-wrap sm:items-end"
    >
      <div className="flex min-w-[180px] flex-[2] flex-col gap-1.5">
        <label
          htmlFor="dashboard-filter-title"
          className="text-xs font-medium text-muted-foreground"
        >
          Titel
        </label>
        <Input
          id="dashboard-filter-title"
          placeholder="Nach Titel suchen…"
          value={title}
          onChange={(event) => handleTitleChange(event.target.value)}
        />
      </div>
      <div className="flex min-w-[160px] flex-1 flex-col gap-1.5">
        <label
          htmlFor="dashboard-filter-status"
          className="text-xs font-medium text-muted-foreground"
        >
          Status
        </label>
        <Select
          items={STATUS_ITEMS}
          value={status}
          onValueChange={handleStatusChange}
        >
          <SelectTrigger id="dashboard-filter-status" className="w-full">
            <SelectValue placeholder="Alle Status" />
          </SelectTrigger>
          <SelectContent alignItemWithTrigger={false}>
            {Object.entries(STATUS_ITEMS).map(([value, label]) => (
              <SelectItem key={value} value={value}>
                {label}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
      </div>
      <div className="flex min-w-[160px] flex-1 flex-col gap-1.5">
        <label
          htmlFor="dashboard-filter-category"
          className="text-xs font-medium text-muted-foreground"
        >
          Kategorie
        </label>
        <Select
          items={categoryItems}
          value={categoryId}
          onValueChange={handleCategoryChange}
        >
          <SelectTrigger id="dashboard-filter-category" className="w-full">
            <SelectValue placeholder="Alle Kategorien" />
          </SelectTrigger>
          <SelectContent alignItemWithTrigger={false}>
            <SelectItem value={ALL}>Alle Kategorien</SelectItem>
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
