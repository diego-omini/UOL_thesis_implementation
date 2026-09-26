// trading.ts contains pure order-entry rules used before backend validation.

export type OrderSide = 'BUY' | 'SELL'

export function canPreviewOrder(side: OrderSide, quantity: number, availableToSell: number): boolean {
  if (!Number.isFinite(quantity) || !Number.isInteger(quantity) || quantity <= 0) return false
  return side !== 'SELL' || quantity <= Math.max(availableToSell, 0)
}
