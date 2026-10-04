// counselor-workflows.test.ts verifies distinct catalog ownership and tool intent in the frontend.

import assert from 'node:assert/strict'
import test from 'node:test'
import { counselorWorkflows } from './counselor-workflows'

test('catalog workflows have ten distinct IDs and relevant interface owners', () => {
  assert.equal(counselorWorkflows.length, 10)
  assert.equal(new Set(counselorWorkflows.map(workflow => workflow.id)).size, 10)
  const addedOwners = counselorWorkflows.slice(5).map(workflow => workflow.owner)
  assert.deepEqual(addedOwners, ['currency', 'effective-exposure', 'history', 'current-position', 'data-quality'])
})

test('new deterministic workflows do not advertise external research', () => {
  for (const workflow of counselorWorkflows.slice(5)) assert.equal(workflow.research, false, workflow.id)
})
