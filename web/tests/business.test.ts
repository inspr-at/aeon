// SPDX-License-Identifier: AGPL-3.0-only
import { test } from 'node:test'
import assert from 'node:assert/strict'
import { businessAreas } from '../src/components/business/areas.ts'
import {
  availability, disablePermissions, installationWrite, parseCatalog, pinAction, pluginGate, statusLabel,
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
  assert.equal(quoteState.reason, 'Waiting on Customers.')
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
})

test('parseCatalog rejects a malformed plugin list', () => {
  const row = plugin('business_costs')
  assert.equal(parseCatalog([row])[0]?.id, 'business_costs')
  assert.equal(parseCatalog([{ ...row, permissions: null }])[0]?.permissions.length, 0)
  assert.throws(() => parseCatalog({}), /not usable/)
  assert.throws(() => parseCatalog([row, row]), /not usable/)
  assert.throws(() => parseCatalog([{ ...row, digest_sha256: 'ab' }]), /not usable/)
})

// ---------- U9: exact amounts, durations and weeks ----------
import { compareAmounts, diffAmounts, formatAmount, lineNet, lineTax, parseAmountInput, parsePercentInput, parseQuantityInput, ratePercent, sumAmounts, toUnits } from '../src/components/business/money.ts'
import { formatClock, formatSpan, parseDurationInput } from '../src/components/business/duration.ts'
import { addDays, dayKey, isoWeek, parseDayKey, parseTimeInput, periodLabel, rangeLabel, startOfWeek, weekDays } from '../src/lib/week.ts'

test('amounts display exactly with en-GB grouping and the currency scale', () => {
  assert.equal(formatAmount('1200.0000', 'EUR'), '1,200.00')
  assert.equal(formatAmount('71.9640', 'EUR'), '71.964')
  assert.equal(formatAmount('95', 'EUR'), '95.00')
  assert.equal(formatAmount('1250', 'JPY'), '1,250')
  assert.equal(formatAmount('-4312.0000', 'EUR'), '−4,312.00')
  assert.equal(formatAmount('200', 'EUR', { signed: true }), '+200.00')
  assert.equal(formatAmount('0.0000', 'EUR', { signed: true }), '0.00')
  assert.throws(() => formatAmount('1e3', 'EUR'), /exact decimal/)
})

test('line and tax amounts round half up at four places like the server', () => {
  assert.equal(lineNet('19.99', '3'), '59.9700')
  assert.equal(lineNet('1.9999', '0.3333'), '0.6666')
  assert.equal(lineTax('59.9700', '0.20000'), '11.9940')
  assert.equal(lineTax('0.0001', '0.50000'), '0.0001')
  assert.equal(sumAmounts(['0.1', '0.2']), '0.3000')
  assert.equal(diffAmounts('28772', '33084'), '-4312.0000')
  assert.equal(compareAmounts('10.5', '10.50'), 0)
  assert.equal(compareAmounts('9.99', '10'), -1)
  assert.throws(() => toUnits('1.00001', 4), /too many decimal places/)
})

test('typed quantities, tax percentages and rates stay exact', () => {
  assert.equal(parseQuantityInput('1,5'), '1.5')
  assert.equal(parseQuantityInput('36.5000'), '36.5')
  assert.equal(parseQuantityInput('0'), null)
  assert.equal(parseQuantityInput('1.23456'), null)
  assert.equal(parsePercentInput('20'), '0.20000')
  assert.equal(parsePercentInput('7.7 %'), '0.07700')
  assert.equal(parsePercentInput('101'), null)
  assert.equal(ratePercent('0.20000'), '20')
  assert.equal(ratePercent('0.07700'), '7.7')
  assert.equal(parseAmountInput('1,200.50'), '1200.5000')
  assert.equal(parseAmountInput('58,5'), '58.5000')
  assert.equal(parseAmountInput('-3'), null)
})

test('durations are typed the way people say them', () => {
  assert.equal(parseDurationInput('1h30'), 5400)
  assert.equal(parseDurationInput('1h 30m'), 5400)
  assert.equal(parseDurationInput('90m'), 5400)
  assert.equal(parseDurationInput('1.5'), 5400)
  assert.equal(parseDurationInput('1,5h'), 5400)
  assert.equal(parseDurationInput('1:30'), 5400)
  assert.equal(parseDurationInput('2'), 7200)
  assert.equal(parseDurationInput('0.1'), 360)
  assert.equal(parseDurationInput('0.33'), null)
  assert.equal(parseDurationInput('45'), null)
  assert.equal(parseDurationInput('25h'), null)
  assert.equal(parseDurationInput('soon'), null)
  assert.equal(formatClock(5400), '1:30')
  assert.equal(formatClock(2700), '0:45')
  assert.equal(formatSpan(5400), '1h 30m')
  assert.equal(formatSpan(2700), '45m')
  assert.equal(formatSpan(28800), '8h')
})

test('weeks run Monday to Sunday in local time', () => {
  const thursday = new Date(2026, 8, 24, 15, 30)
  const monday = startOfWeek(thursday)
  assert.equal(dayKey(monday), '2026-09-21')
  assert.equal(dayKey(startOfWeek(new Date(2026, 8, 27, 23, 0))), '2026-09-21')
  assert.deepEqual(weekDays(monday).map(dayKey), ['2026-09-21', '2026-09-22', '2026-09-23', '2026-09-24', '2026-09-25', '2026-09-26', '2026-09-27'])
  assert.equal(isoWeek(thursday), 39)
  assert.equal(isoWeek(new Date(2027, 0, 1)), 53)
  assert.equal(rangeLabel(monday, addDays(monday, 6)), '21–27 Sep 2026')
  assert.equal(rangeLabel(new Date(2026, 8, 28), new Date(2026, 9, 4)), '28 Sep – 4 Oct 2026')
  assert.equal(periodLabel(new Date(2026, 8, 14).toISOString(), new Date(2026, 8, 21).toISOString()), '14–20 Sep 2026')
  assert.equal(parseDayKey('2026-02-30'), null)
  assert.equal(parseTimeInput('9'), 540)
  assert.equal(parseTimeInput('09:30'), 570)
  assert.equal(parseTimeInput('14.15'), 855)
  assert.equal(parseTimeInput('24:00'), null)
})
