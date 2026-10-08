// session-requests.ts defines the acceptance rule for responses crossing an authenticated session boundary.

export interface AuthenticatedRequestIdentity {
  session: number
  generation: number
}

export function acceptsAuthenticatedResponse(
  request: AuthenticatedRequestIdentity,
  currentSession: number,
  currentGeneration: number,
  sessionReady: boolean,
): boolean {
  return sessionReady && request.session === currentSession && request.generation === currentGeneration
}
