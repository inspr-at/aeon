// SPDX-License-Identifier: AGPL-3.0-only
import { test } from 'node:test'
import assert from 'node:assert/strict'
import { clearFatal, fatal, isRenderError, isStaleBuild, reference, reportFatal } from '../src/lib/fatal.ts'

test('render-breaking errors are recognised in development and production form', () => {
  for (const info of ['render function', 'setup function', 'mounted hook', 'async component loader', 'https://vuejs.org/error-reference/#runtime-1', 'https://vuejs.org/error-reference/#runtime-m', 'https://vuejs.org/error-reference/#runtime-15']) {
    assert.equal(isRenderError(info), true, info)
  }
  for (const info of ['native event handler', 'component event handler', 'watcher callback', 'https://vuejs.org/error-reference/#runtime-5', 'https://vuejs.org/error-reference/#runtime-um', '']) {
    assert.equal(isRenderError(info), false, info)
  }
})

test('failed chunk loads read as a new version', () => {
  assert.equal(isStaleBuild(new TypeError('Failed to fetch dynamically imported module: https://x/assets/a.js')), true)
  assert.equal(isStaleBuild(new TypeError('Importing a module script failed.')), true)
  assert.equal(isStaleBuild(new Error('Cannot read properties of undefined')), false)
})

test('the report keeps a short first line, a reference and the retry path, never a stack', () => {
  const error = new RangeError(`Out of range\n    at render (App.vue:12:3)\n${'x'.repeat(400)}`)
  reportFatal(error, 'navigation', '/p/PHAROS')
  assert.equal(fatal.value?.kind, 'navigation')
  assert.equal(fatal.value?.name, 'RangeError')
  assert.equal(fatal.value?.message, 'Out of range')
  assert.equal(fatal.value?.path, '/p/PHAROS')
  assert.match(fatal.value?.reference ?? '', /^E-[0-9A-Z]{1,6}$/)
  assert.doesNotMatch(JSON.stringify(fatal.value), /App\.vue|at render/)
  reportFatal(new TypeError('Failed to fetch dynamically imported module: /a.js'), 'navigation')
  assert.equal(fatal.value?.kind, 'update')
  assert.equal(fatal.value?.path, undefined)
  reportFatal('plain text', 'render')
  assert.equal(fatal.value?.message, 'plain text')
  clearFatal()
  assert.equal(fatal.value, null)
  assert.equal(reference(Date.UTC(2026, 8, 24)), `E-${Date.UTC(2026, 8, 24).toString(36).toUpperCase().slice(-6)}`)
})
