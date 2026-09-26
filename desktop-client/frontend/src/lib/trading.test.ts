// trading.test.ts verifies whole-share and long-only order-entry rules.

import assert from 'node:assert/strict'
import { describe, it } from 'node:test'
import { canPreviewOrder } from './trading'

describe('paper order entry', () => {
  it('accepts only positive whole-share quantities', () => {
    assert.equal(canPreviewOrder('BUY', 1, 0), true)
    assert.equal(canPreviewOrder('BUY', 0, 0), false)
    assert.equal(canPreviewOrder('BUY', 1.5, 0), false)
    assert.equal(canPreviewOrder('BUY', Number.NaN, 0), false)
  })

  it('prevents sell entry above the available long position', () => {
    assert.equal(canPreviewOrder('SELL', 5, 5), true)
    assert.equal(canPreviewOrder('SELL', 6, 5), false)
    assert.equal(canPreviewOrder('SELL', 1, -2), false)
  })
})
