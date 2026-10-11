// counselor-workflows.test.ts verifies grouping, availability, switching, stale rejection, and error categories.

import assert from 'node:assert/strict'
import test from 'node:test'
import { acceptCounselorResponse, beginCounselorRequest, emptyCounselorState, rejectCounselorResponse } from './counselor-state'
import { classifyCounselorError } from './counselor-errors'
import { counselorWorkflows, workflowAvailability, workflowCopy } from './counselor-workflows'

test('analysis library has distinct workflows grouped by user purpose', () => {
  assert.equal(counselorWorkflows.length, 10)
  assert.equal(new Set(counselorWorkflows.map(workflow => workflow.id)).size, 10)
  assert.deepEqual(new Set(counselorWorkflows.map(workflow => workflow.purpose)), new Set([
    'Portfolio overview', 'Risk', 'Diversification', 'History', 'Research', 'Stock analysis', 'Proposed-order analysis',
  ]))
  for (const workflow of counselorWorkflows) {
    assert.ok(workflow.description)
    assert.ok(workflow.requiredData)
  }
  assert.deepEqual(counselorWorkflows.filter(workflow => workflow.research).map(workflow => workflow.id), [
    'portfolio_resume', 'portfolio_news', 'stock_news', 'order_analysis',
  ])
  assert.match(workflowCopy('portfolio_risk').description, /not a realized portfolio-return/)
  assert.match(workflowCopy('currency_exposure').description, /instrument currencies.*not each issuer’s full economic currency exposure/)
  assert.match(workflowCopy('position_risk').description, /not unmeasured business risk/)
})

test('availability rules explain missing portfolio, position, history, risk, and order inputs', () => {
  assert.deepEqual(workflowAvailability(workflowCopy('portfolio_resume'), { positionCount: 0 }), { available: false, reason: 'Requires at least one open position.' })
  assert.equal(workflowAvailability(workflowCopy('portfolio_resume'), { positionCount: 1 }).available, true)
  assert.equal(workflowAvailability(workflowCopy('portfolio_news'), { positionCount: 13 }).available, true)
  assert.match(workflowCopy('portfolio_news').description, /12 largest open positions/)
  assert.match(workflowAvailability(workflowCopy('position_risk'), { stockSelected: true, positionHeld: false }).reason, /currently held/)
  assert.match(workflowAvailability(workflowCopy('portfolio_risk'), { riskObservations: 19 }).reason, /20 common daily returns/)
  assert.match(workflowAvailability(workflowCopy('portfolio_history_summary'), { historyPoints: 0 }).reason, /No observations/)
  assert.match(workflowAvailability(workflowCopy('order_analysis'), { validOrder: false }).reason, /whole-share quantity/)
})

test('switching analyses clears the previous result and stale responses are rejected', () => {
  const empty = emptyCounselorState()
  assert.deepEqual(empty, { generation: 0, active: null, loading: false, response: null, error: '' })
  const first = beginCounselorRequest(empty, 'portfolio_resume')
  assert.equal(first.loading, true)
  const switched = beginCounselorRequest(first, 'currency_exposure')
  assert.equal(switched.active, 'currency_exposure')
  assert.equal(switched.response, null)
  const stale = acceptCounselorResponse(switched, first.generation, { PromptID: 'portfolio_resume' } as never)
  assert.equal(stale, switched)
  const current = acceptCounselorResponse(switched, switched.generation, { PromptID: 'currency_exposure' } as never)
  assert.equal(current.response?.PromptID, 'currency_exposure')
  const failed = rejectCounselorResponse(beginCounselorRequest(current, 'portfolio_news'), current.generation + 1, 'provider failed')
  assert.equal(failed.loading, false)
  assert.equal(failed.response, null)
  assert.equal(failed.error, 'provider failed')
})

test('counselor failures distinguish session, research, provider, market data, broker, and validation', () => {
  assert.equal(classifyCounselorError('requires an authenticated IBKR paper session').kind, 'session')
  assert.equal(classifyCounselorError('research API key is missing').kind, 'research')
  assert.equal(classifyCounselorError('Ollama connection refused').kind, 'provider')
  assert.equal(classifyCounselorError('context deadline exceeded').kind, 'provider')
  assert.equal(classifyCounselorError("OpenAI request failed: Unsupported parameter: 'temperature'").kind, 'provider')
  assert.equal(classifyCounselorError('insufficient common return observations').kind, 'market_data')
  assert.equal(classifyCounselorError('IBKR portfolio request failed').kind, 'broker')
  const validation = classifyCounselorError('provider could not produce a valid final response')
  assert.equal(validation.kind, 'validation')
  assert.equal(validation.title, 'Response could not be recovered')
  assert.match(validation.guidance, /automatic cleanup/i)
})
