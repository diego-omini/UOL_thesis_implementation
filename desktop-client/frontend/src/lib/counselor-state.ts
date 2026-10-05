// counselor-state.ts provides deterministic single-owner request and result transitions.

import type { AIResponseView } from './types'
import type { CounselorWorkflowID } from './counselor-workflows'

export interface CounselorRequestState {
  generation: number
  active: CounselorWorkflowID | null
  loading: boolean
  response: AIResponseView | null
  error: string
}

export function emptyCounselorState(generation = 0): CounselorRequestState {
  return { generation, active: null, loading: false, response: null, error: '' }
}

export function beginCounselorRequest(state: CounselorRequestState, workflow: CounselorWorkflowID): CounselorRequestState {
  return { generation: state.generation + 1, active: workflow, loading: true, response: null, error: '' }
}

export function acceptCounselorResponse(state: CounselorRequestState, generation: number, response: AIResponseView): CounselorRequestState {
  if (generation !== state.generation) return state
  return { ...state, loading: false, response, error: '' }
}

export function rejectCounselorResponse(state: CounselorRequestState, generation: number, error: string): CounselorRequestState {
  if (generation !== state.generation) return state
  return { ...state, loading: false, response: null, error }
}
