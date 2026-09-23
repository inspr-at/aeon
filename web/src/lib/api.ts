// SPDX-License-Identifier: AGPL-3.0-only
export interface Identity {
  principal: { id: string; name: string; email?: string }
  tenant: { id: string; name: string }
}

export interface Version { version: string; scheme: string }

export async function api(path: string, init: RequestInit = {}) {
  return fetch(`/api${path}`, {
    credentials: 'same-origin',
    cache: 'no-store',
    signal: AbortSignal.timeout(10_000),
    ...init,
    headers: { Accept: 'application/json', ...init.headers },
  })
}

// Keep the P0.3 auth wire contract here, separate from view components.
export async function getSession(): Promise<{ identity: Identity | null; devMode: boolean }> {
  const response = await api('/me')
  if (!response.ok && response.status !== 401) throw new Error('Session unavailable')
  // A 401 may have no JSON body; it still means sign-in is required.
  const body = await response.json().catch(() => {
    if (response.status === 401) return {}
    throw new Error('Invalid session response')
  })
  const devMode = body.dev_mode === true
  if (response.status === 401) return { identity: null, devMode }
  if (typeof body.principal?.id !== 'string' || typeof body.principal?.name !== 'string'
    || typeof body.tenant?.id !== 'string' || typeof body.tenant?.name !== 'string') {
    throw new Error('Invalid session response')
  }
  return { identity: body as Identity, devMode }
}
