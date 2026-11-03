const dateFormatter = new Intl.DateTimeFormat("de-DE", {
  day: "2-digit",
  month: "2-digit",
  year: "numeric",
  hour: "2-digit",
  minute: "2-digit",
})

const currencyFormatter = new Intl.NumberFormat("de-DE", {
  style: "currency",
  currency: "EUR",
})

export function formatEventDate(startTime: string, endTime: string) {
  const start = new Date(startTime)
  const end = new Date(endTime)

  if (Number.isNaN(start.getTime())) {
    return startTime
  }

  if (Number.isNaN(end.getTime())) {
    return dateFormatter.format(start)
  }

  return dateFormatter.formatRange(start, end)
}

export function formatEventPrice(price: string) {
  const amount = Number(price)

  if (!Number.isFinite(amount) || amount <= 0) {
    return "Kostenlos"
  }

  return currencyFormatter.format(amount)
}
