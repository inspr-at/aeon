// SPDX-License-Identifier: AGPL-3.0-only
import { createRouter, createWebHistory } from 'vue-router'
import { setPageTitle } from './lib/brand'
import { useProjects } from './stores/projects'
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
    // The earlier workspace tree and list are gone; the projects page replaces them.
    { path: '/workspace', redirect: '/' },
    // The journey lives in the project page (?view=journey); earlier journey links lead there.
    {
      path: '/projects/:projectId/:rest(.*)*', component: NotFoundView, meta: { title: 'Journey' },
      beforeEnter: async to => {
        const projects = useProjects()
        await projects.load()
        const project = projects.byId(String(to.params.projectId))
        const rest = Array.isArray(to.params.rest) ? to.params.rest : []
        const stage = rest[0] === 'journey' && rest[1] ? { stage: rest[1] } : {}
        return project ? { path: `/p/${encodeURIComponent(project.routeKey)}`, query: { view: 'journey', ...stage }, replace: true } : true
      },
    },
    // Business: an overview, hours and rates. Quotes and organisations are parked
    // (their views stay in views/business, unrouted) until they are ported from classic Paimos.
    { path: '/business', component: () => import('./views/business/BusinessHome.vue'), meta: { title: 'Business' } },
    { path: '/business/hours', component: () => import('./views/business/HoursView.vue'), meta: { title: 'Hours' } },
    { path: '/business/rates', component: () => import('./views/business/CostUnitsView.vue'), meta: { title: 'Rates' } },
    { path: '/business/costs', redirect: '/business/rates' },
    { path: '/business/cost-units', redirect: '/business/rates' },
    { path: '/business/:parked(quotes|organisations|crm)/:rest(.*)*', redirect: '/business' },
    { path: '/crm', redirect: '/business' },
    // One record for the overview and its open session, so opening the panel never remounts the page.
    { path: '/agents/:sessionId?', component: () => import('./views/AgentsView.vue'), meta: { title: 'Agents', fill: false } },
    // Earlier separate pages now live inside Agents.
    { path: '/runs/:runId?', redirect: '/agents' },
    { path: '/approvals', redirect: '/agents' },
    { path: '/pacing', redirect: '/agents' },
    // The release history is a sheet over the page (App.vue); its own links open it over Projects.
    { path: '/releases/:version?', component: ProjectsView, meta: { title: 'Releases' } },
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
// A new page names the tab; a query change (filters, the release sheet) keeps the page's own title.
router.afterEach((to, from) => { if (to.path !== from.path || !from.matched.length) setPageTitle(String(to.meta.title ?? '')) })
