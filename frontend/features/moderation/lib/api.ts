"use client"

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query"

import {
  isRatingPage,
  isVisibilityResponse,
  moderationErrorMessage,
  type ModeratedRatingPage,
  type ModerationFilters,
} from "@/features/moderation/lib/types"
import { moderationQuery } from "@/features/moderation/lib/validation"

const ratingsKey = ["moderation", "ratings"] as const

async function moderationRequest<T>(
  path: string,
  validate: (body: unknown) => body is T,
  payload?: { hidden: boolean; reason: string }
): Promise<T> {
  let response: Response
  try {
    response = await fetch(`/api/moderation/ratings${path}`, {
      method: payload ? "PUT" : "GET",
      ...(payload
        ? {
            headers: { "Content-Type": "application/json" },
            body: JSON.stringify(payload),
          }
        : {}),
      cache: "no-store",
    })
  } catch {
    throw new Error(
      "Die Verbindung ist fehlgeschlagen. Bitte versuche es erneut."
    )
  }
  let body: unknown
  try {
    body = await response.json()
  } catch {
    throw new Error(
      "Der Moderationsdienst hat eine ungültige Antwort geliefert."
    )
  }
  if (!response.ok)
    throw new Error(moderationErrorMessage(body, response.status))
  if (!validate(body))
    throw new Error("Die Moderationsantwort ist unvollständig.")
  return body
}

export function useModeratedRatings(filters: ModerationFilters) {
  return useQuery({
    queryKey: [...ratingsKey, filters],
    queryFn: () =>
      moderationRequest(`?${moderationQuery(filters)}`, isRatingPage),
    retry: false,
  })
}

export function useRatingVisibility() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: ({
      ratingId,
      hidden,
      reason,
    }: {
      ratingId: string
      hidden: boolean
      reason: string
    }) =>
      moderationRequest(`/${ratingId}/visibility`, isVisibilityResponse, {
        hidden,
        reason,
      }),
    onSuccess: async (result) => {
      await queryClient.cancelQueries({ queryKey: ratingsKey })
      for (const [key] of queryClient.getQueriesData<ModeratedRatingPage>({
        queryKey: ratingsKey,
      })) {
        const filters = key[2]
        const hidden =
          filters && typeof filters === "object" && "hidden" in filters
            ? filters.hidden
            : undefined
        queryClient.setQueryData<ModeratedRatingPage>(key, (page) => {
          if (!page) return page
          const items = page.items
            .map((item) =>
              item.ratingId === result.ratingId
                ? { ...item, hidden: result.hidden }
                : item
            )
            .filter((item) => hidden === undefined || item.hidden === hidden)
          return {
            ...page,
            items,
            total: page.total - (page.items.length - items.length),
          }
        })
      }
      void queryClient.invalidateQueries({ queryKey: ratingsKey })
      void queryClient.invalidateQueries({ queryKey: ["organization"] })
    },
  })
}
