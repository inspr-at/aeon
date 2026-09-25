// SPDX-License-Identifier: AGPL-3.0-only
// U27 (AEON-154): people payloads say whether a person has a picture, and an
// avatar asks the server for one only then. Someone without a photo costs no
// request (and no 404 in the console); someone with one still shows it.
import { test, expect, type Page } from '@playwright/test'
import { fixtures, me, mockWork, PNG, watchErrors } from './work-fixtures'

test.beforeEach(async ({ page }) => { await page.clock.setSystemTime(new Date('2026-09-23T12:00:00Z')) })

const MIRA = '22222222-2222-4222-8222-222222222222'

async function countAvatars(page: Page) {
  const asked = new Map<string, number>()
  await page.route('**/api/people/*/avatar/*', route => {
    const id = new URL(route.request().url()).pathname.split('/')[3]
    asked.set(id, (asked.get(id) ?? 0) + 1)
    return id === MIRA ? route.fulfill({ contentType: 'image/png', body: PNG }) : route.fulfill({ status: 404, json: { error: 'avatar not found' } })
  })
  return asked
}

test('no avatar request is made for someone without a photo; one with a photo is shown', async ({ page }) => {
  const errors = watchErrors(page)
  const failed: string[] = []
  page.on('response', response => { if (response.url().includes('/avatar/') && response.status() >= 400) failed.push(response.url()) })
  const data = fixtures()
  // Mira has a picture; Markus (the signed-in person, no profile picture) has none.
  data.people = data.people.map(person => ({ ...person, has_avatar: person.id === MIRA }))
  for (const entry of data.activity['n-1']) entry.author = { ...entry.author, has_avatar: entry.author.id === MIRA } as typeof entry.author
  await mockWork(page, data)
  // Registered after the API mock, so it answers first.
  const asked = await countAvatars(page)
  // PHAROS-11: Markus assigned; Mira and Markus comment.
  await page.goto('/p/PHAROS/PHAROS-11')
  await expect(page.getByRole('button', { name: /Assignee: Markus Barta/ }).locator('.avatar')).toContainText('MB')
  const activity = page.getByRole('region', { name: 'Activity' })
  await expect(activity.locator('.entry.comment').last()).toContainText('Mira Holm')
  await expect(activity.locator('.entry.comment').last().locator('.avatar.loaded')).toHaveCount(1)
  await expect(activity.locator('.entry.comment').first().locator('.avatar')).toContainText('MB')
  // The assignee menu offers both.
  await page.getByRole('button', { name: /Assignee: Markus Barta/ }).click()
  const menu = page.getByRole('menu', { name: /Assignee/ })
  await expect(menu.getByRole('menuitemradio', { name: /Mira Holm/ }).locator('.avatar.loaded')).toHaveCount(1)
  await page.keyboard.press('Escape')
  // PHAROS-13: Mira assigned.
  await page.goto('/p/PHAROS/PHAROS-13')
  await expect(page.getByRole('button', { name: /Assignee: Mira Holm/ }).locator('.avatar.loaded')).toHaveCount(1)
  // Project cards stack the recent people.
  await page.goto('/')
  await page.getByRole('radio', { name: 'Cards view' }).click()
  await expect(page.locator('.people .avatar.loaded')).toHaveCount(1)
  await expect(page.locator('.people .avatar').filter({ hasText: 'MB' }).first()).toBeVisible()
  expect(asked.get(me.id) ?? 0).toBe(0)
  expect(asked.get(MIRA) ?? 0).toBeGreaterThan(0)
  expect(failed).toEqual([])
  expect(errors).toEqual([])
})

test('a person nobody said has a photo shows initials without asking', async ({ page }) => {
  // An older server sends no has_avatar: initials, and still no request.
  await mockWork(page, fixtures())
  const asked = await countAvatars(page)
  await page.goto('/p/PHAROS/PHAROS-13')
  await expect(page.getByRole('button', { name: /Assignee: Mira Holm/ }).locator('.avatar')).toContainText('MH')
  expect([...asked.values()].reduce((a, b) => a + b, 0)).toBe(0)
})
