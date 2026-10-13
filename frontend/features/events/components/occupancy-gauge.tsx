"use client"

import { PolarAngleAxis, RadialBar, RadialBarChart } from "recharts"

import { EMPTY_VALUE } from "@/features/events/lib/format"

const WIDTH = 280
const HEIGHT = 146

/**
 * EVENTHUB-81. A half-donut gauge of how much of an event's capacity is
 * booked, with the percentage centered under the arc. `null` renders an
 * empty placeholder instead of a misleading 0%. Fixed pixel dimensions
 * (rather than a responsive container) keep the half-circle's center, and
 * so the overlaid percentage label, pinned exactly under the arc.
 */
export function OccupancyGauge({ percent }: { percent: number | null }) {
  if (percent === null) {
    return (
      <div
        style={{ width: WIDTH, height: HEIGHT }}
        className="flex items-end justify-center pb-1 text-sm text-muted-foreground"
      >
        {EMPTY_VALUE}
      </div>
    )
  }

  const data = [{ value: percent, fill: "var(--color-primary)" }]

  return (
    <div className="relative" style={{ width: WIDTH, height: HEIGHT }}>
      <RadialBarChart
        width={WIDTH}
        height={HEIGHT}
        data={data}
        startAngle={180}
        endAngle={0}
        innerRadius={WIDTH * 0.35}
        outerRadius={WIDTH * 0.5}
        cx={WIDTH / 2}
        cy={HEIGHT}
        barSize={24}
      >
        {/* Pins the bar's scale to 0–100; without it a single radial bar
            scales its domain to its own value and always renders full. */}
        <PolarAngleAxis type="number" domain={[0, 100]} tick={false} />
        <RadialBar
          dataKey="value"
          background={{ fill: "var(--color-muted)" }}
          cornerRadius={12}
          isAnimationActive={false}
        />
      </RadialBarChart>
      <div className="absolute inset-x-0 bottom-0 text-center text-3xl font-semibold tabular-nums">
        {percent}%
      </div>
    </div>
  )
}
