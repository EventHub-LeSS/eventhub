"use client"

import { Bar, BarChart, CartesianGrid, XAxis } from "recharts"

import type { ActivityPoint } from "@/features/organizations/lib/mock-data"
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/features/shared/components/ui/card"
import {
  ChartContainer,
  ChartLegend,
  ChartLegendContent,
  ChartTooltip,
  ChartTooltipContent,
  type ChartConfig,
} from "@/features/shared/components/ui/chart"

const chartConfig = {
  created: {
    label: "Events created",
    color: "var(--color-chart-2)",
  },
  held: {
    label: "Events held",
    color: "var(--color-chart-4)",
  },
} satisfies ChartConfig

export function ActivityChart({ data }: { data: ActivityPoint[] }) {
  return (
    <Card>
      <CardHeader>
        <CardTitle>Activity</CardTitle>
        <CardDescription>
          Events created vs. events held, per month
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
          </BarChart>
        </ChartContainer>
      </CardContent>
    </Card>
  )
}
