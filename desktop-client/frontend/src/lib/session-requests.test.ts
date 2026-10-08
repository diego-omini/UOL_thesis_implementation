// session-requests.test.ts verifies logout, session-loss, and refresh invalidation rules.

import assert from 'node:assert/strict'
import { describe, it } from 'node:test'
import { acceptsAuthenticatedResponse } from './session-requests'

describe('authenticated request transitions', () => {
  const request = { session: 4, generation: 9 }

  it('accepts only the active ready session and data generation', () => {
    assert.equal(acceptsAuthenticatedResponse(request, 4, 9, true), true)
    assert.equal(acceptsAuthenticatedResponse(request, 4, 10, true), false)
  })

  it('rejects a late response after logout or session loss', () => {
    assert.equal(acceptsAuthenticatedResponse(request, 5, 10, false), false)
    assert.equal(acceptsAuthenticatedResponse(request, 4, 9, false), false)
  })
})
