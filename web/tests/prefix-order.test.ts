// SPDX-License-Identifier: AGPL-3.0-only
// QA4 (AEON-140): the production CSS minifier (lightningcss) merges a property
// with its -webkit- twin and keeps only the spelling declared last. Written as
// "backdrop-filter: x; -webkit-backdrop-filter: x" the build shipped only the
// -webkit- form, which Chrome ignores, so every glass surface (header, footer,
// sticky toolbar and table headers, palette, toasts) lost its blur. The
// prefixed declaration therefore comes first in every rule.
import { test } from 'node:test'
import assert from 'node:assert/strict'
import { readdirSync, readFileSync, statSync } from 'node:fs'
import { join, relative } from 'node:path'

const root = new URL('../src/', import.meta.url).pathname
// Files other packages own right now; each entry names its owner and must still match.
const ALLOW: { file: string; why: string }[] = [
  { file: 'components/journey/GateCard.vue', why: 'Journey view, AEON-139 (builder A)' },
  { file: 'components/journey/JourneyRail.vue', why: 'Journey view, AEON-139 (builder A)' },
  { file: 'components/journey/ReleaseWalker.vue', why: 'Journey view, AEON-139 (builder A)' },
  { file: 'components/journey/WalkerBar.vue', why: 'Journey view, AEON-139 (builder A)' },
  { file: 'styles/journey.css', why: 'Journey view, AEON-139 (builder A)' },
]

function files(dir: string): string[] {
  return readdirSync(dir).flatMap(name => {
    const path = join(dir, name)
    if (statSync(path).isDirectory()) return name === 'vendor' ? [] : files(path)
    return /\.(vue|css)$/.test(name) ? [path] : []
  })
}
export function lateWebkit(source: string): { line: number; property: string }[] {
  const out: { line: number; property: string }[] = []
  for (const block of source.matchAll(/\{([^{}]*)\}/g)) {
    const props = block[1]!.split(';').map(d => d.split(':')[0]!.trim()).filter(Boolean)
    props.forEach((prop, i) => {
      if (!prop.startsWith('-webkit-')) return
      const bare = props.indexOf(prop.slice('-webkit-'.length))
      if (bare !== -1 && bare < i) out.push({ line: source.slice(0, block.index).split('\n').length, property: prop.slice('-webkit-'.length) })
    })
  }
  return out
}

test('a -webkit- declaration never follows its unprefixed twin', () => {
  const found: string[] = []
  const used = new Set<string>()
  for (const path of files(root)) {
    const file = relative(root, path)
    const problems = lateWebkit(readFileSync(path, 'utf8'))
    if (!problems.length) continue
    if (ALLOW.some(entry => entry.file === file)) { used.add(file); continue }
    found.push(...problems.map(p => `${file}:${p.line} ${p.property}: put -webkit-${p.property} before ${p.property}`))
  }
  assert.deepEqual(found, [])
  assert.deepEqual(ALLOW.filter(entry => !used.has(entry.file)).map(entry => `${entry.file} no longer needs its exception (${entry.why})`), [])
})

test('the check catches the late prefix and lets the right order through', () => {
  assert.deepEqual(lateWebkit('.a { backdrop-filter: blur(4px); -webkit-backdrop-filter: blur(4px); }'), [{ line: 1, property: 'backdrop-filter' }])
  assert.deepEqual(lateWebkit('.a { -webkit-backdrop-filter: blur(4px); backdrop-filter: blur(4px); }'), [])
  assert.deepEqual(lateWebkit('.a { -webkit-line-clamp: 2; }'), [])
})
