"use client"

import { CartesianGrid, Line, LineChart, XAxis, YAxis } from "recharts"

import type { SalesPoint } from "@/features/organizations/lib/mock-data"
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/features/shared/components/ui/card"
import {
  ChartContainer,
  ChartTooltip,
  ChartTooltipContent,
  type ChartConfig,
} from "@/features/shared/components/ui/chart"

const chartConfig = {
  revenue: {
    label: "Umsatz",
    color: "var(--color-chart-2)",
  },
} satisfies ChartConfig

export function SalesChart({ data }: { data: SalesPoint[] }) {
  return (
    <Card>
      <CardHeader>
        <CardTitle>Verkäufe</CardTitle>
        <CardDescription>Ticketumsatz pro Monat</CardDescription>
      </CardHeader>
      <CardContent>
        <ChartContainer
          config={chartConfig}
          className="aspect-auto h-72 w-full"
        >
          <LineChart data={data}>
            <CartesianGrid vertical={false} />
            <XAxis
              dataKey="month"
              tickLine={false}
              axisLine={false}
              tickMargin={8}
            />
            <YAxis
              tickLine={false}
              axisLine={false}
              tickMargin={8}
              width={56}
              domain={[0, "dataMax"]}
              tickCount={3}
              tickFormatter={(value: number) =>
                `${value.toLocaleString("de-DE")} €`
              }
            />
            <ChartTooltip content={<ChartTooltipContent />} />
            <Line
              dataKey="revenue"
              type="monotone"
              stroke="var(--color-revenue)"
              strokeWidth={2}
              dot={false}
            />
          </LineChart>
        </ChartContainer>
      </CardContent>
    </Card>
  )
}
