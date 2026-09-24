// SPDX-License-Identifier: AGPL-3.0-only
import { createRouter, createWebHistory } from 'vue-router'
import { useSession } from './stores/session'
import ProjectsView from './views/ProjectsView.vue'
import SignInView from './views/SignInView.vue'
import NotFoundView from './views/NotFoundView.vue'

export const router = createRouter({
  history: createWebHistory(),
  routes: [
    { path: '/', component: ProjectsView, meta: { title: 'Projects' } },
    // One record for the list and its open ticket, so opening the panel never remounts the list.
    { path: '/p/:projectKey/:ticketKey?', component: () => import('./views/ProjectView.vue'), meta: { title: 'Project' } },
    // Earlier workspace tree and list, kept reachable but unlinked.
    { path: '/workspace', component: () => import('./views/HomeView.vue'), meta: { title: 'Workspace', legacySearch: true, fill: true } },
    { path: '/projects/:projectId', component: () => import('./views/JourneyView.vue'), meta: { title: 'Journey', fill: true } },
    { path: '/projects/:projectId/journey/:stage', component: () => import('./views/JourneyView.vue'), meta: { title: 'Journey', fill: true } },
    { path: '/business', component: () => import('./views/business/BusinessHome.vue'), meta: { title: 'Business', fill: true } },
    { path: '/business/crm', component: () => import('./views/business/CRMView.vue'), meta: { title: 'Organisations', fill: true } },
    { path: '/business/quotes', component: () => import('./views/business/QuotesView.vue'), meta: { title: 'Quotes', fill: true } },
    { path: '/business/costs', alias: '/business/cost-units', component: () => import('./views/business/CostUnitsView.vue'), meta: { title: 'Cost units', fill: true } },
    { path: '/business/hours', component: () => import('./views/business/HoursView.vue'), meta: { title: 'Hours', fill: true } },
    { path: '/crm', redirect: '/business/crm' },
    { path: '/agents', component: () => import('./views/AgentsView.vue'), meta: { title: 'Agents' } },
    { path: '/runs/:runId?', component: () => import('./views/RunsView.vue'), meta: { title: 'Sessions & runs' } },
    { path: '/approvals', component: () => import('./views/ApprovalsView.vue'), meta: { title: 'Approvals' } },
    { path: '/pacing', component: () => import('./views/PacingView.vue'), meta: { title: 'Pacing' } },
    { path: '/signin', component: SignInView, meta: { title: 'Sign in', bare: true } },
    { path: '/:pathMatch(.*)*', component: NotFoundView, meta: { title: 'Page not found' } },
  ],
})

router.beforeEach(async (to) => {
  const session = useSession()
  const wasSignedIn = !!session.identity
  await session.refresh()
  if (session.error) return true // The shell shows a retry screen, never protected content.
  // Losing a session without signing out means it expired; say so on the sign-in page.
  if (!session.identity && to.path !== '/signin') return wasSignedIn ? { path: '/signin', query: { error: 'expired' } } : '/signin'
  if (session.identity && to.path === '/signin') return '/'
})
router.afterEach((to) => { document.title = `${to.meta.title} · PAIMOS AEON` })
