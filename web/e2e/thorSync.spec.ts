import { expect, test, type Page } from '@playwright/test'

async function useDemoData(page: Page) {
  await page.route('**/api/v1/**', (route) => route.abort('connectionrefused'))
}

test.beforeEach(async ({ page }) => {
  await useDemoData(page)
})

test('filters and searches the visual game library', async ({ page }) => {
  await page.goto('/')
  await expect(page.getByRole('heading', { name: 'Welcome back' })).toBeVisible()
  await expect(page.getByRole('button', { name: /^Open / })).toHaveCount(6)

  await page.getByRole('group', { name: 'Filter games' }).getByRole('button', { name: 'NDS' }).click()
  await expect(page.getByRole('button', { name: /^Open / })).toHaveCount(3)
  await expect(page.getByRole('button', { name: 'Open Signal Drift' })).toBeVisible()

  await page.getByRole('group', { name: 'Filter games' }).getByRole('button', { name: 'All' }).click()
  await page.getByRole('textbox', { name: 'Search games' }).fill('Luma')
  await expect(page.getByRole('button', { name: 'Open Luma Isles' })).toBeVisible()
  await expect(page.getByRole('button', { name: /^Open / })).toHaveCount(1)
})

test('walks through onboarding without enabling delivery early', async ({ page }) => {
  await page.goto('/onboarding')
  await expect(page.getByRole('heading', { name: 'Check the ZimaOS hub' })).toBeVisible()

  const continueButton = page.getByRole('button', { name: 'Continue' })
  await continueButton.click()
  await expect(page.getByRole('heading', { name: 'Connect both playing devices' })).toBeVisible()
  await continueButton.click()
  await expect(page.getByRole('heading', { name: 'Keep each endpoint separate' })).toBeVisible()
  await continueButton.click()
  await continueButton.click()
  await continueButton.click()
  await continueButton.click()

  await expect(page.getByRole('heading', { name: 'Ready to leave import mode' })).toBeVisible()
  await expect(page.getByRole('checkbox', { name: /Enable automatic delivery/ })).toBeChecked()
  await page.getByRole('button', { name: 'Finish setup' }).click()
  await expect(page).toHaveURL(/\/$/)
  await expect(page.getByRole('heading', { name: 'Welcome back' })).toBeVisible()
})

test('promotes a protected conflict branch only after acknowledgement', async ({ page }) => {
  await page.goto('/')
  await page.locator('.sidebar__nav').getByRole('button', { name: /Conflicts/ }).click()
  const card = page.locator('.conflict-card').filter({ hasText: 'Amber Circuit' })
  await expect(card).toBeVisible()
  await card.getByRole('button', { name: 'Choose this save' }).nth(1).click()

  const promote = page.getByRole('button', { name: 'Use this version' })
  await expect(promote).toBeDisabled()
  await page.getByRole('checkbox', { name: /closed this game/ }).check()
  await promote.click()
  await expect(page.getByRole('heading', { name: 'No conflicts to review' })).toBeVisible()
})

test('requires emulator confirmation before restoring history', async ({ page }) => {
  await page.goto('/')
  await page.getByRole('button', { name: 'Open Luma Isles' }).click()
  await expect(page.getByRole('heading', { name: 'Luma Isles' })).toBeVisible()
  await page.getByRole('button', { name: 'Restore this save' }).first().click()

  const restore = page.getByRole('button', { name: 'Restore and deliver' })
  await expect(restore).toBeDisabled()
  await page.getByRole('checkbox', { name: /closed this game/ }).check()
  await restore.click()
  await expect(page.locator('.toast')).toContainText('Restore queued')
})
