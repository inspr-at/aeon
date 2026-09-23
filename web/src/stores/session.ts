// SPDX-License-Identifier: AGPL-3.0-only
import { defineStore } from 'pinia'
import { ref } from 'vue'
import { api, getSession, type Identity } from '../lib/api'

export const useSession = defineStore('session', () => {
  const identity = ref<Identity | null>(null)
  const devMode = ref(false)
  const error = ref('')

  async function refresh() {
    error.value = ''
    try {
      const session = await getSession()
      identity.value = session.identity
      devMode.value = session.devMode
    } catch {
      identity.value = null
      devMode.value = false
      error.value = 'We couldn’t reach your workspace. Please try again.'
    }
  }

  async function signOut() {
    const response = await api('/auth/logout', { method: 'POST' })
    if (!response.ok) throw new Error('Sign out failed')
    identity.value = null
  }

  async function devLogin(email: string) {
    if (!devMode.value) throw new Error('Development sign-in is unavailable')
    const response = await api('/auth/dev-login', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ email }),
    })
    if (!response.ok) throw new Error('Sign in failed')
  }

  return { identity, devMode, error, refresh, signOut, devLogin }
})
