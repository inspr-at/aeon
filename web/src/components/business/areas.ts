// SPDX-License-Identifier: AGPL-3.0-only

export type BusinessIconName = 'work' | 'overview' | 'costs' | 'crm' | 'quotes' | 'hours' | 'check' | 'refresh'

export interface BusinessArea {
  id: 'overview' | 'costs' | 'crm' | 'quotes' | 'hours'
  pluginId: string | null
  label: string
  to: string
  summary: string
  icon: BusinessIconName
  requires: readonly string[]
}

// Paths are the contract the coordinator registers in web/src/router.ts.
// This package owns only the overview view.
export const businessAreas: readonly BusinessArea[] = [
  {
    id: 'overview',
    pluginId: null,
    label: 'Overview',
    to: '/business',
    summary: '',
    icon: 'overview',
    requires: [],
  },
  {
    id: 'costs',
    pluginId: 'business_costs',
    label: 'Cost units',
    to: '/business/costs',
    summary: 'Effective-dated internal and bill rates for each cost unit. Totals stay in their own currency.',
    icon: 'costs',
    requires: [],
  },
  {
    id: 'crm',
    pluginId: 'business_crm',
    label: 'Organisations',
    to: '/business/crm',
    summary: 'Organisations and contacts. Accepting an offer requires an explicit link to a person.',
    icon: 'crm',
    requires: [],
  },
  {
    id: 'quotes',
    pluginId: 'business_quotes',
    label: 'Quotes',
    to: '/business/quotes',
    summary: 'Frozen offers. Issue and customer acceptance are separate events.',
    icon: 'quotes',
    requires: ['business_costs', 'business_crm'],
  },
  {
    id: 'hours',
    pluginId: 'business_hours',
    label: 'Hours',
    to: '/business/hours',
    summary: 'Time on a node, with the rate from the day the entry starts. Approved periods stay closed.',
    icon: 'hours',
    requires: ['business_costs'],
  },
]

export const businessSections = businessAreas.filter((area): area is BusinessArea & { pluginId: string } => area.pluginId !== null)

// View files for the coordinator. Sibling packages own every view except BusinessHome.
export const businessViews = {
  overview: { path: '/business', title: 'Business', view: 'web/src/views/business/BusinessHome.vue' },
  costs: { path: '/business/costs', title: 'Cost units', view: 'web/src/views/business/CostUnitsView.vue' },
  crm: { path: '/business/crm', title: 'Organisations', view: 'web/src/views/business/CRMView.vue' },
  quotes: { path: '/business/quotes', title: 'Quotes', view: 'web/src/views/business/QuotesView.vue' },
  hours: { path: '/business/hours', title: 'Hours', view: 'web/src/views/business/HoursView.vue' },
} as const

export function pluginLabel(pluginId: string): string {
  return businessAreas.find(area => area.pluginId === pluginId)?.label ?? pluginId
}
