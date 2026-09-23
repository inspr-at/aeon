// SPDX-License-Identifier: AGPL-3.0-only
import { createRouter, createWebHistory } from 'vue-router'
import { useSession } from './stores/session'
import HomeView from './views/HomeView.vue'
import SignInView from './views/SignInView.vue'
import NotFoundView from './views/NotFoundView.vue'

export const router = createRouter({
  history: createWebHistory(),
  routes: [
    { path: '/', component: HomeView, meta: { title: 'Workspace' } },
    { path: '/projects/:projectId', component: () => import('./views/JourneyView.vue'), meta: { title: 'Journey' } },
    { path: '/projects/:projectId/journey/:stage', component: () => import('./views/JourneyView.vue'), meta: { title: 'Journey' } },
    { path: '/agents', component: () => import('./views/AgentsView.vue'), meta: { title: 'Agents' } },
    { path: '/runs/:runId?', component: () => import('./views/RunsView.vue'), meta: { title: 'Sessions & runs' } },
    { path: '/approvals', component: () => import('./views/ApprovalsView.vue'), meta: { title: 'Approvals' } },
    { path: '/pacing', component: () => import('./views/PacingView.vue'), meta: { title: 'Pacing' } },
    { path: '/signin', component: SignInView, meta: { title: 'Sign in' } },
    { path: '/:pathMatch(.*)*', component: NotFoundView, meta: { title: 'Page not found' } },
  ],
})

router.beforeEach(async (to) => {
  const session = useSession()
  await session.refresh()
  if (session.error) return true // The shell shows a retry screen, never protected content.
  if (!session.identity && to.path !== '/signin') return '/signin'
  if (session.identity && to.path === '/signin') return '/'
})
router.afterEach((to) => { document.title = `${to.meta.title} · PAIMOS AEON` })
