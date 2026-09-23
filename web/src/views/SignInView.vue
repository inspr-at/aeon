<!-- SPDX-License-Identifier: AGPL-3.0-only -->
<script setup lang="ts">
import { ref } from 'vue'
import { useRouter } from 'vue-router'
import { useSession } from '../stores/session'
import AppIcon from '../components/AppIcon.vue'
import VersionDisplay from '../components/VersionDisplay.vue'

const session = useSession()
const router = useRouter()
const email = ref('')
const busy = ref(false)
const error = ref('')
async function signIn() {
  busy.value = true
  error.value = ''
  try {
    await session.devLogin(email.value.trim())
    await router.replace('/')
    if (!session.identity && !session.error) error.value = 'Your session wasn’t established. Please try again.'
  } catch { error.value = 'Sign in didn’t complete. Check your email and try again.' }
  finally { busy.value = false }
}
</script>

<template>
  <section class="center-stage" aria-labelledby="signin-title">
    <div class="signin-card glass-card" :class="{ 'development-card': session.devMode }">
      <div v-if="!session.devMode" class="welcome-seal"><AppIcon name="compass" /></div>
      <p class="eyebrow">PAIMOS AEON</p>
      <h1 v-if="session.devMode" id="signin-title">Welcome back.</h1>
      <h1 v-else id="signin-title">A shared place.<br />A new beginning.</h1>
      <p class="intro">Sign in to your workspace.</p>
      <a class="button login-button" href="/api/auth/login">Sign in with INSPR<AppIcon name="arrow" /></a>
      <p v-if="!session.devMode" class="signin-note">Your INSPR account brings you in.</p>
      <form v-if="session.devMode" class="dev-form" @submit.prevent="signIn">
        <p class="eyebrow">Development sign-in</p>
        <label for="email">Email address</label>
        <input id="email" v-model="email" name="email" type="email" autocomplete="email" placeholder="you@example.com" required :disabled="busy" />
        <button class="button secondary" type="submit" :disabled="busy">{{ busy ? 'Signing in…' : 'Continue with email' }}<AppIcon name="arrow" /></button>
        <p v-if="error" class="error" role="alert">{{ error }}</p>
      </form>
      <div class="card-version"><VersionDisplay /></div>
    </div>
  </section>
</template>

<style scoped>
.signin-card { width: min(420px, 100%); padding: 32px 36px 14px; text-align: center; }
.welcome-seal { display: grid; place-items: center; margin: 0 auto 22px; width: 54px; height: 54px; border: 1px solid var(--gold); border-radius: 50%; color: var(--teal-ink); background: var(--glass); }
.welcome-seal svg { width: 26px; height: 26px; }
h1 { margin-top: 14px; font-size: 36px; }
.intro { margin-top: 16px; font-size: 14px; }
.login-button { display: flex; margin-top: 28px; }
.signin-note { margin-top: 12px; font-size: 12px; }
.card-version { margin-top: 14px; padding-top: 4px; border-top: 1px solid var(--line); }
.dev-form { margin-top: 20px; padding-top: 18px; border-top: 1px solid var(--line); display: grid; gap: 9px; text-align: left; }
.dev-form .eyebrow { margin-bottom: 3px; }
label { color: var(--ink-2); font-size: 12px; }
input { width: 100%; min-height: 46px; padding: 10px 12px; border: 1px solid var(--line-2); border-radius: var(--radius-s); background: var(--surface); color: var(--ink); }
input::placeholder { color: var(--ink-2); }
.dev-form .button { min-height: 44px; font-size: 13px; }
.development-card { padding-top: 24px; }
.development-card h1 { font-size: 30px; }
.development-card .intro { margin-top: 10px; }
.development-card .login-button { margin-top: 20px; }
@media (max-width: 480px) { .signin-card { padding: 28px 24px 12px; } h1 { font-size: 32px; } }
@media (max-height: 760px) and (min-width: 601px) { .center-stage { padding-block: 12px; } .signin-card { padding-top: 22px; } .welcome-seal { width: 44px; height: 44px; margin-bottom: 14px; } h1 { font-size: 32px; } .login-button { margin-top: 20px; } .dev-form { margin-top: 14px; padding-top: 12px; } .card-version { margin-top: 10px; } }
</style>
