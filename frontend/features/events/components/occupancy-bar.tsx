/* eslint-disable jsx-a11y/prefer-tag-over-role — custom inner bar is not styleable with <progress> */

export interface OccupancyBarProps {
  sold: number
  capacity: number
  percent: number
}

export function OccupancyBar({ sold, capacity, percent }: OccupancyBarProps) {
  const ratio = Math.max(0, Math.min(100, percent))

  return (
    <div className="flex flex-col gap-1.5">
      <div
        role="progressbar"
        aria-valuemin={0}
        aria-valuemax={100}
        aria-valuenow={ratio}
        aria-label={`Occupancy: ${ratio.toFixed(1)}% of ${capacity} seats`}
        className="relative overflow-hidden rounded-full bg-muted h-3 w-full"
      >
        <div
          className="h-full rounded-full bg-primary"
          style={{ width: `${ratio}%` }}
        />
      </div>
      <span className="text-xs text-muted-foreground tabular-nums">
        {sold.toLocaleString()} sold ·{" "}
        {Math.max(0, capacity - sold).toLocaleString()} free of{" "}
        {capacity.toLocaleString()} seats ({ratio.toFixed(1)}%)
      </span>
    </div>
  )
}
