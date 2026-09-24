// SPDX-License-Identifier: AGPL-3.0-only
// The product's name comes from brand.json through the server (lib/brand.ts), so a
// deployment can rename it. No visible string in web/src spells it: comments,
// ticket keys (AEON-72), code identifiers, CSS classes and routes are fine.
import { test } from 'node:test'
import assert from 'node:assert/strict'
import { readdirSync, readFileSync, statSync } from 'node:fs'
import { join, relative } from 'node:path'

const root = new URL('../src/', import.meta.url).pathname
// The one place allowed to spell it: the fallback until the server answers.
const ALLOWED = new Set(['lib/brand.ts'])

function files(dir: string): string[] {
  return readdirSync(dir).flatMap(name => {
    const path = join(dir, name)
    if (statSync(path).isDirectory()) return name === 'vendor' ? [] : files(path)
    return /\.(vue|ts)$/.test(name) ? [path] : []
  })
}
// Drop comments so that notes like "(AEON-70)" or "classic Paimos" in prose never count.
export function withoutComments(text: string) {
  return text
    .replace(/<!--[\s\S]*?-->/g, m => m.replace(/[^\n]/g, ' '))
    .replace(/\/\*[\s\S]*?\*\//g, m => m.replace(/[^\n]/g, ' '))
    .replace(/(^|[\s;{}(),])\/\/[^\n]*/g, (m, lead) => lead + ' '.repeat(m.length - lead.length))
}
export const PRODUCT_NAME = /\bAEON\b(?!-\d)|\bAeon\b/

test('no hardcoded product name in the web app', () => {
  const problems: string[] = []
  for (const path of files(root)) {
    const file = relative(root, path)
    if (ALLOWED.has(file)) continue
    withoutComments(readFileSync(path, 'utf8')).split('\n').forEach((line, i) => {
      if (PRODUCT_NAME.test(line)) problems.push(`${file}:${i + 1} ${line.trim().slice(0, 120)}`)
    })
  }
  assert.deepEqual(problems, [], `Use the brand (lib/brand.ts) instead:\n${problems.join('\n')}`)
})

test('the check ignores comments and ticket keys but catches the name', () => {
  assert.equal(PRODUCT_NAME.test(withoutComments('// shipped in AEON-72, see Aeon docs')), false)
  assert.equal(PRODUCT_NAME.test(withoutComments('<!-- Parked (AEON-70) -->')), false)
  assert.equal(PRODUCT_NAME.test(withoutComments("const url = 'https://aeon.example' // Aeon")), false)
  assert.equal(PRODUCT_NAME.test(withoutComments("title: 'Welcome to Aeon'")), true)
  assert.equal(PRODUCT_NAME.test(withoutComments('<span>PAIMOS AEON</span>')), true)
})
