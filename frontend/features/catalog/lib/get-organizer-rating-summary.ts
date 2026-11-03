import "server-only";
import { envConfig } from "@/features/shared/lib/env";

export interface OrganizerRatingSummaryData {
  organizationId: string;
  averageRating: number | null;
  ratedEventCount: number;
}

export type OrganizerRatingSummaryResult =
  | { status: "ok"; summary: OrganizerRatingSummaryData }
  | { status: "unavailable" };

export async function getOrganizerRatingSummary(
  organizationId: string,
): Promise<OrganizerRatingSummaryResult> {
  try {
    const response = await fetch(
      `${envConfig.apiBaseUrl}/organizations/${encodeURIComponent(organizationId)}/rating-summary`,
      { cache: "no-store" },
    );

    if (!response.ok) {
      return { status: "unavailable" };
    }

    const summary =
      (await response.json()) as Partial<OrganizerRatingSummaryData>;
    const average = summary.averageRating ?? null;

    if (
      !summary ||
      typeof summary.organizationId !== "string" ||
      (average !== null && typeof average !== "number") ||
      typeof summary.ratedEventCount !== "number"
    ) {
      return { status: "unavailable" };
    }

    return {
      status: "ok",
      summary: {
        organizationId: summary.organizationId,
        averageRating: average,
        ratedEventCount: summary.ratedEventCount,
      },
    };
  } catch {
    return { status: "unavailable" };
  }
}
