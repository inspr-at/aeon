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
    to: '/business/rates',
    summary: 'Bill and internal rates per cost unit, each valid from a date. Quotes and hours take the rate in force.',
    icon: 'costs',
    requires: [],
  },
  {
    id: 'crm',
    pluginId: 'business_crm',
    label: 'Customers',
    to: '/business/customers',
    summary: 'Customers with their contacts and addresses, linked to their projects, quotes and hours.',
    icon: 'crm',
    requires: [],
  },
  {
    id: 'quotes',
    pluginId: 'business_quotes',
    label: 'Quotes',
    to: '/business/quotes',
    summary: 'Offers with exact totals. Each version is frozen; issuing and the customer’s acceptance are recorded separately.',
    icon: 'quotes',
    requires: ['business_costs', 'business_crm'],
  },
  {
    id: 'hours',
    pluginId: 'business_hours',
    label: 'Hours',
    to: '/business/hours',
    summary: 'Time on tickets, priced at the rate of the day. An admin approves each period; approved periods stay closed.',
    icon: 'hours',
    requires: ['business_costs'],
  },
]

export const businessSections = businessAreas.filter((area): area is BusinessArea & { pluginId: string } => area.pluginId !== null)

// Routed business pages. Quotes are parked until the quote editor is ported.
export const businessViews = {
  overview: { path: '/business', title: 'Business', view: 'web/src/views/business/BusinessHome.vue' },
  crm: { path: '/business/customers', title: 'Customers', view: 'web/src/views/business/CustomersView.vue' },
  costs: { path: '/business/rates', title: 'Rates', view: 'web/src/views/business/CostUnitsView.vue' },
  hours: { path: '/business/hours', title: 'Hours', view: 'web/src/views/business/HoursView.vue' },
} as const

export function pluginLabel(pluginId: string): string {
  return businessAreas.find(area => area.pluginId === pluginId)?.label ?? pluginId
}
