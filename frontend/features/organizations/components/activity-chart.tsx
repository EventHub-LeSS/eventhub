"use client";

import { Bar, BarChart, CartesianGrid, XAxis } from "recharts";

import type { ActivityPoint } from "@/features/organizations/lib/activity";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/features/shared/components/ui/card";
import {
  ChartContainer,
  ChartLegend,
  ChartLegendContent,
  ChartTooltip,
  ChartTooltipContent,
  type ChartConfig,
} from "@/features/shared/components/ui/chart";

const chartConfig = {
  created: {
    label: "Erstellt",
    color: "var(--color-chart-2)",
  },
  held: {
    label: "Durchgeführt",
    color: "var(--color-chart-4)",
  },
  joined: {
    label: "Neue Mitglieder",
    color: "var(--color-chart-1)",
  },
} satisfies ChartConfig;

export function ActivityChart({
  data,
  showJoined = false,
}: {
  data: ActivityPoint[];
  showJoined?: boolean;
}) {
  return (
    <Card>
      <CardHeader>
        <CardTitle>Aktivität</CardTitle>
        <CardDescription>
          {showJoined
            ? "Erstellte und durchgeführte Veranstaltungen sowie neue Mitglieder pro Monat"
            : "Erstellte vs. durchgeführte Veranstaltungen pro Monat"}
        </CardDescription>
      </CardHeader>
      <CardContent>
        <ChartContainer
          config={chartConfig}
          className="aspect-auto h-72 w-full"
        >
          <BarChart data={data} barGap={4}>
            <CartesianGrid vertical={false} />
            <XAxis
              dataKey="month"
              tickLine={false}
              axisLine={false}
              tickMargin={8}
            />
            <ChartTooltip content={<ChartTooltipContent />} />
            <ChartLegend content={<ChartLegendContent />} />
            <Bar dataKey="created" fill="var(--color-created)" radius={4} />
            <Bar dataKey="held" fill="var(--color-held)" radius={4} />
            {showJoined && (
              <Bar dataKey="joined" fill="var(--color-joined)" radius={4} />
            )}
          </BarChart>
        </ChartContainer>
      </CardContent>
    </Card>
  );
}
