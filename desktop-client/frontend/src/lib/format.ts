// format.ts centralizes resilient presentation formatting for portfolio and market data.
import type { MoneyView, OptionalNumberView, RatioView } from './types'

const decimalFormatters = new Map<string, Intl.NumberFormat>()
const currencyFormatters = new Map<string, Intl.NumberFormat>()

function decimalFormatter(minimumFractionDigits: number, maximumFractionDigits: number): Intl.NumberFormat {
  const key = `${minimumFractionDigits}:${maximumFractionDigits}`
  let formatter = decimalFormatters.get(key)
  if (!formatter) {
    formatter = new Intl.NumberFormat(undefined, { minimumFractionDigits, maximumFractionDigits })
    decimalFormatters.set(key, formatter)
  }
  return formatter
}

function currencyFormatter(currency: string, minimumFractionDigits: number, maximumFractionDigits: number): Intl.NumberFormat | null {
  const normalizedCurrency = currency.trim().toUpperCase() || 'USD'
  const key = `${normalizedCurrency}:${minimumFractionDigits}:${maximumFractionDigits}`
  const existing = currencyFormatters.get(key)
  if (existing) return existing

  try {
    const formatter = new Intl.NumberFormat(undefined, {
      style: 'currency',
      currency: normalizedCurrency,
      minimumFractionDigits,
      maximumFractionDigits,
    })
    currencyFormatters.set(key, formatter)
    return formatter
  } catch {
    return null
  }
}

export function formatCurrencyAmount(value: number, currency: string, maximumFractionDigits = 2, minimumFractionDigits = 0): string {
  const formatter = currencyFormatter(currency, minimumFractionDigits, maximumFractionDigits)
  if (formatter) return formatter.format(value)

  const suffix = currency.trim().toUpperCase()
  return `${decimalFormatter(minimumFractionDigits, maximumFractionDigits).format(value)}${suffix ? ` ${suffix}` : ''}`
}

export function formatMoney(value: MoneyView | undefined): string {
  if (!value?.Available) return '-'
  return formatCurrencyAmount(value.Amount ?? 0, value.Currency || 'USD', 2)
}

export function formatOptionalCurrency(value: OptionalNumberView | undefined, currency: string): string {
  if (!value?.Available) return '-'
  return formatCurrencyAmount(value.Value, currency || 'USD', 4)
}

export function formatOptionalNumber(value: OptionalNumberView | undefined, maximumFractionDigits = 2): string {
  if (!value?.Available) return '-'
  return decimalFormatter(0, maximumFractionDigits).format(value.Value)
}

export function formatNumber(value: number | undefined, maximumFractionDigits = 4): string {
  return decimalFormatter(0, maximumFractionDigits).format(value ?? 0)
}

export function formatPercent(value: RatioView | undefined): string {
  if (!value?.Available) return '-'
  return new Intl.NumberFormat(undefined, { style: 'percent', maximumFractionDigits: 1 }).format(value.Value)
}

export function formatRatio(value: RatioView | undefined): string {
  if (!value?.Available) return '-'
  return decimalFormatter(3, 3).format(value.Value)
}

export function formatExchangeRate(value: number): string {
  return decimalFormatter(4, 6).format(value)
}

export function formatDate(value: string): string {
  const date = new Date(value)
  return Number.isNaN(date.getTime()) ? '-' : date.toLocaleString()
}

export function formatError(error: unknown): string {
  return error instanceof Error ? error.message : String(error)
}
