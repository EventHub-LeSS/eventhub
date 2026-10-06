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
  maximumFractionDigits: 0,
})

export function formatEventDate(startTime: string) {
  const date = new Date(startTime)

  if (Number.isNaN(date.getTime())) {
    return startTime
  }

  return dateFormatter.format(date)
}

export function formatEventPrice(price: string) {
  const amount = Number(price)

  if (!Number.isFinite(amount) || amount <= 0) {
    return "Kostenlos"
  }

  return currencyFormatter.format(amount)
}
