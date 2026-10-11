// format.test.ts verifies currency formatting remains safe for unexpected broker currency labels.
import assert from 'node:assert/strict'
import { describe, it } from 'node:test'
import { formatCurrencyAmount, formatMoney, formatResponseNumbers } from './format'
import type { MoneyView } from './types'

describe('frontend formatting', () => {
  it('falls back to a number and currency suffix when Intl rejects a currency code', () => {
    const formatted = formatCurrencyAmount(1234.5, 'BASE', 2)

    assert.match(formatted, /BASE$/)
    assert.match(formatted, /1.*234/)
  })

  it('keeps unavailable money distinct from a numeric zero', () => {
    assert.equal(formatMoney({ Amount: 0, Currency: 'USD', Available: false } as MoneyView), '-')
    assert.notEqual(formatMoney({ Amount: 0, Currency: 'USD', Available: true } as MoneyView), '-')
  })

  it('limits response values to two decimals and expands scientific notation', () => {
    const formatted = formatResponseNumbers('6.619693851887816 percent and 1.009493375e+06 GBP on 2026-10-11T12:34:56.123Z')

    assert.match(formatted, /6[.,]62 percent/)
    assert.match(formatted, /1.*009.*493[.,]38 GBP/)
    assert.match(formatted, /2026-10-11T12:34:56\.123Z/)
  })
})
