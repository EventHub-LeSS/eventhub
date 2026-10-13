"use client"

import { XIcon } from "lucide-react"
import { usePathname, useRouter } from "next/navigation"
import { useState } from "react"

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
  [ALL]: "All statuses",
  draft: "Draft",
  published: "Published",
  completed: "Completed",
  cancelled: "Cancelled",
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

  const categoryItems: Record<string, string> = { [ALL]: "All categories" }
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

  function handleSubmit(event: React.FormEvent<HTMLFormElement>) {
    event.preventDefault()
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
          Title
        </label>
        <Input
          id="dashboard-filter-title"
          placeholder="Search by title…"
          value={title}
          onChange={(event) => setTitle(event.target.value)}
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
            <SelectValue placeholder="All statuses" />
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
          Category
        </label>
        <Select
          items={categoryItems}
          value={categoryId}
          onValueChange={handleCategoryChange}
        >
          <SelectTrigger id="dashboard-filter-category" className="w-full">
            <SelectValue placeholder="All categories" />
          </SelectTrigger>
          <SelectContent alignItemWithTrigger={false}>
            <SelectItem value={ALL}>All categories</SelectItem>
            {categories.map((category) => (
              <SelectItem key={category.categoryId} value={category.categoryId}>
                {category.category}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
      </div>
      <div className="flex gap-2">
        <Button type="submit">Filter</Button>
        {hasActiveFilter && (
          <Button type="button" variant="outline" onClick={handleReset}>
            <XIcon />
            Reset
          </Button>
        )}
      </div>
    </form>
  )
}
