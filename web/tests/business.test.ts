// SPDX-License-Identifier: AGPL-3.0-only
import { test } from 'node:test'
import assert from 'node:assert/strict'
import { businessAreas } from '../src/components/business/areas.ts'
import {
  availability, disablePermissions, installationWrite, isTenantAdmin, parseCatalog, pinAction, pluginGate, statusLabel,
  type BusinessPlugin,
} from '../src/components/business/catalog.ts'
import { formatDuration } from '../src/components/business/duration.ts'
import { addDecimal, formatDecimal, formatMinor, multiplyDecimal, parseJSONExact } from '../src/components/business/money.ts'

const build = 'a'.repeat(64)
const pinned = 'b'.repeat(64)
const space = '\u202f'

function plugin(id: string, patch: Partial<BusinessPlugin> = {}, installation: Partial<BusinessPlugin['installation']> = {}): BusinessPlugin {
  return {
    id,
    version: '1',
    digest_sha256: build,
    owner: 'aeon',
    permissions: ['views.provide', 'nodes.contribute'],
    node_kinds: [],
    views: [],
    installation: {
      manifest_digest_sha256: build,
      enabled: true,
      permissions: ['views.provide', 'nodes.contribute'],
      plugin_id: id,
      version: '1',
      updated_at: '2026-09-23T18:00:00Z',
      ...installation,
    },
    ...patch,
  }
}

test('formats exact decimals and minor units without binary floats', () => {
  assert.equal(formatDecimal('1250.50', 'EUR'), `1${space}250.50 EUR`)
  assert.equal(formatDecimal('0.10', 'USD'), '0.10 USD')
  assert.equal(addDecimal('0.10', '0.20'), '0.30')
  assert.equal(addDecimal('1.5', '2.25'), '3.75')
  assert.equal(multiplyDecimal('1.25', '8'), '10.00')
  assert.equal(formatMinor('100', 'USD'), '1.00 USD')
  assert.equal(formatMinor('1250', 'JPY'), `1${space}250 JPY`)
  assert.equal(formatMinor('10', 'BHD'), '0.010 BHD')
  assert.throws(() => formatDecimal('1e2', 'EUR'), /exact decimal/)
  assert.throws(() => formatDecimal('-1.00', 'EUR'), /exact decimal/)
  assert.throws(() => formatMinor('1.0', 'USD'), /integer string/)
  assert.throws(() => formatMinor('10', 'XXX'), /Unknown currency scale/)
  assert.throws(() => formatDecimal(1.5 as unknown as string, 'EUR'), /exact decimal/)
})

test('parseJSONExact keeps number spellings and rejects exponents', () => {
  assert.deepEqual(parseJSONExact('{"amount":12.50,"ok":true,"n":0,"label":"12"}'), {
    amount: '12.50', ok: true, n: '0', label: '12',
  })
  assert.deepEqual(parseJSONExact('{"note":"1e2","rows":[1.50,2]}'), { note: '1e2', rows: ['1.50', '2'] })
  assert.throws(() => parseJSONExact('{"n":1e2}'), /Exponent/)
  assert.throws(() => parseJSONExact('{"n":01}'), /not usable/)
})

test('duration uses whole seconds', () => {
  assert.equal(formatDuration(0), '0s')
  assert.equal(formatDuration(3661), '1h 1m 1s')
  assert.equal(formatDuration(3600), '1h')
  assert.throws(() => formatDuration(1.5), /whole number/)
})

test('plugin gates fail closed and quotes wait on both dependencies', () => {
  const costs = plugin('business_costs')
  const crm = plugin('business_crm', {}, { enabled: false, permissions: [], updated_at: '0001-01-01T00:00:00Z' })
  const quotes = plugin('business_quotes')
  const hours = plugin('business_hours', { permissions: ['views.provide', 'steps.apply'] }, { permissions: ['steps.apply'] })
  const mismatched = plugin('business_hours', {}, { manifest_digest_sha256: pinned })
  assert.equal(pluginGate(undefined).state, 'absent')
  assert.equal(pluginGate(crm).state, 'unpinned')
  assert.equal(pluginGate(plugin('business_costs', {}, { enabled: false })).state, 'disabled')
  assert.equal(pluginGate(mismatched).state, 'digest_mismatch')
  assert.deepEqual(pluginGate(hours), { state: 'under_granted', missing: ['views.provide'] })
  assert.equal(pluginGate(costs).state, 'open')

  const catalog = [costs, crm, quotes, hours]
  const quoteArea = businessAreas.find(area => area.id === 'quotes')!
  const hourArea = businessAreas.find(area => area.id === 'hours')!
  const quoteState = availability(quoteArea, catalog)
  assert.equal(quoteState.open, false)
  assert.equal(quoteState.reason, 'Waiting on Organisations.')
  assert.equal(statusLabel(quoteState), 'Waiting')
  assert.equal(pinAction(quoteState), 'disable')
  const hourState = availability(hourArea, [plugin('business_costs', {}, { enabled: false }), plugin('business_hours')])
  assert.equal(hourState.open, false)
  assert.match(hourState.reason, /Waiting on Cost units/)
  assert.equal(availability(hourArea, [costs, plugin('business_hours', { permissions: ['views.provide'] }, { permissions: ['views.provide'] })]).open, true)
})

test('installation write pins the compiled digest and declared permissions', () => {
  const item = plugin('business_crm', { permissions: ['views.provide', 'nodes.contribute'] }, {
    manifest_digest_sha256: pinned,
    enabled: false,
    permissions: ['views.provide', 'not_declared'],
  })
  assert.deepEqual(installationWrite(item, true), {
    manifest_digest_sha256: build,
    enabled: true,
    permissions: ['views.provide', 'nodes.contribute'],
  })
  assert.deepEqual(disablePermissions(item), ['views.provide'])
  assert.deepEqual(installationWrite(plugin('business_costs'), false).manifest_digest_sha256, build)
  assert.equal(isTenantAdmin({ principal: { kind: 'person', roles: ['admin'] } }), true)
  assert.equal(isTenantAdmin({ principal: { kind: 'person', roles: ['customer'] } }), false)
  assert.equal(isTenantAdmin({ principal: { kind: 'agent', roles: ['admin'] } }), false)
  assert.equal(isTenantAdmin(null), false)
})

test('parseCatalog rejects a malformed plugin list', () => {
  const row = plugin('business_costs')
  assert.equal(parseCatalog([row])[0]?.id, 'business_costs')
  assert.equal(parseCatalog([{ ...row, permissions: null }])[0]?.permissions.length, 0)
  assert.throws(() => parseCatalog({}), /not usable/)
  assert.throws(() => parseCatalog([row, row]), /not usable/)
  assert.throws(() => parseCatalog([{ ...row, digest_sha256: 'ab' }]), /not usable/)
})
