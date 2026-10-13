/* eslint-disable jsx-a11y/prefer-tag-over-role — custom inner bar is not styleable with <progress> */
import { cn } from "@/features/shared/lib/utils"

export interface OccupancyBarProps {
  sold: number
  capacity: number
  percent: number
  compact?: boolean
  className?: string
}

export function OccupancyBar({
  sold,
  capacity,
  percent,
  compact = false,
  className,
}: OccupancyBarProps) {
  const ratio = Math.max(0, Math.min(100, percent))

  return (
    <div
      className={cn(
        "flex flex-col",
        compact ? "gap-0.5" : "gap-1.5",
        className
      )}
    >
      <div
        role="progressbar"
        aria-valuemin={0}
        aria-valuemax={100}
        aria-valuenow={ratio}
        aria-label={`Occupancy: ${ratio.toFixed(1)}% of ${capacity} seats`}
        className={cn(
          "relative overflow-hidden rounded-full bg-muted",
          compact ? "h-1.5 w-16" : "h-3 w-full"
        )}
      >
        <div
          className="h-full rounded-full bg-primary"
          style={{ width: `${ratio}%` }}
        />
      </div>
      {compact ? (
        <span className="text-xs tabular-nums text-muted-foreground">
          {ratio.toFixed(1)}%
        </span>
      ) : (
        <span className="text-xs text-muted-foreground tabular-nums">
          {sold.toLocaleString()} sold ·{" "}
          {Math.max(0, capacity - sold).toLocaleString()} free of{" "}
          {capacity.toLocaleString()} seats ({ratio.toFixed(1)}%)
        </span>
      )}
    </div>
  )
}