// SPDX-License-Identifier: AGPL-3.0-only
import { defineStore } from 'pinia'
import { computed, ref } from 'vue'
import { avatarColor } from '../lib/avatar'
import { getProfile, patchProfile, removeAvatar, type Profile, type ProfilePatch } from '../lib/profile'

// The signed-in person's profile, loaded once per session; every avatar of theirs
// (header, menu, chips) reads its picture version and initials from here.
export const useProfile = defineStore('profile', () => {
  const profile = ref<Profile | null>(null)
  const error = ref(false)
  let request: Promise<void> | null = null
  function load(force = false): Promise<void> {
    if (profile.value && !force) return Promise.resolve()
    return request ??= (async () => {
      try { profile.value = await getProfile(); error.value = false }
      catch { error.value = true }
      finally { request = null }
    })()
  }
  async function save(fields: ProfilePatch) { profile.value = await patchProfile(fields); return profile.value }
  function adopt(next: Profile) { profile.value = next }
  async function dropAvatar() { profile.value = await removeAvatar(); return profile.value }
  function reset() { profile.value = null; error.value = false }
  const id = computed(() => profile.value?.principal_id ?? null)
  const hasPicture = computed(() => !!profile.value && Object.keys(profile.value.avatar_hashes ?? {}).length > 0)
  const color = computed(() => profile.value?.avatar_color ?? avatarColor(id.value))
  return { profile, error, load, save, adopt, dropAvatar, reset, id, hasPicture, color }
})
