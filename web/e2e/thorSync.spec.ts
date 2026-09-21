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
  await expect(page.getByRole('radio', { name: /Standalone mGBA Recommended/ })).toBeChecked()
  await page.getByRole('checkbox', { name: /closed all emulators/ }).check()
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

test('confirms an existing-install mGBA setup and clears its recovered quarantine', async ({ page }) => {
  await page.unroute('**/api/v1/**')
  let configured = false
  let quarantined = true
  let updateBody: Record<string, unknown> | undefined
  await page.route('**/api/v1/**', async (route) => {
    const request = route.request()
    const path = new URL(request.url()).pathname
    if (path === '/api/v1/events') return route.abort()
    if (path === '/api/v1/settings/emulators/windows-gba' && request.method() === 'PUT') {
      updateBody = request.postDataJSON() as Record<string, unknown>
      configured = true
      quarantined = false
      return route.fulfill({ json: { windowsGbaProfileId: 'windows-mgba', configured: true, affectedBindings: 0 } })
    }
    if (path === '/api/v1/settings/propagation' && request.method() === 'POST') return route.fulfill({ json: { enabled: true } })
    const responses: Record<string, unknown> = {
      '/api/v1/games': [],
      '/api/v1/endpoints': [],
      '/api/v1/activity': [],
      '/api/v1/conflicts': [],
      '/api/v1/diagnostics': [],
      '/api/v1/archive/usage': { usedBytes: 131088, quotaBytes: 5368709120, freeBytes: 10737418240, reserveBytes: 1073741824, blobCount: 1, revisionCount: 1 },
      '/api/v1/onboarding': { complete: true, currentStep: 6, syncthingConnected: true, storageWritable: true, endpointsConfigured: true, inventoryComplete: true, propagationEnabled: true },
      '/api/v1/unassigned': quarantined ? [{ id: 'wrapped-save', endpointId: 'windows', relativePath: 'Pokemon Lazarus.sav', size: 131088, sourceModifiedAt: null, observedAt: '2026-09-21T10:00:00Z', provenance: 'unknown', state: 'unassigned', detail: 'Quarantined: unsupported GBA save size', suggestedProfileId: 'windows-mgba', compatibleProfileIds: ['windows-mgba'] }] : [],
      '/api/v1/settings/emulators': { windowsGbaProfileId: 'windows-mgba', configured, detectedProfileId: 'windows-mgba', affectedBindings: configured ? 0 : 1 },
      '/api/v1/profiles': [
        { id: 'windows-mgba', name: 'Standalone mGBA', endpointId: 'windows', platform: 'gba', extension: '.sav', format: 'raw-battery+opaque-rtc' },
        { id: 'windows-vbam', name: 'VBA-M', endpointId: 'windows', platform: 'gba', extension: '.sav', format: 'raw-battery' },
      ],
    }
    if (path in responses) return route.fulfill({ json: responses[path] })
    return route.fulfill({ status: 404, body: 'not found' })
  })

  await page.goto('/unassigned')
  await expect(page.getByText('Pokemon Lazarus.sav')).toBeVisible()
  await expect(page.getByText('Standalone mGBA RTC format detected')).toBeVisible()
  await page.locator('.sidebar__footer').getByRole('button', { name: 'Settings' }).click()
  await expect(page.getByText('One-time setup required.', { exact: false })).toBeVisible()

  await page.getByRole('button', { name: 'Save preferences' }).click()
  await expect(page.getByText(/Close mGBA, VBA-M, and RetroArch/)).toBeVisible()
  await page.getByRole('checkbox', { name: /closed all emulators/ }).check()
  await page.getByRole('button', { name: 'Save preferences' }).click()
  await expect.poll(() => updateBody).toEqual({ profileId: 'windows-mgba', emulatorClosed: true, applyToExisting: true })

  await page.locator('.sidebar__nav').getByRole('button', { name: 'Unassigned' }).click()
  await expect(page.getByRole('heading', { name: 'Everything is assigned' })).toBeVisible()
})

test('changes one game to VBA-M only after the emulator-closed acknowledgement', async ({ page }) => {
  await page.goto('/')
  await page.getByRole('button', { name: 'Open Luma Isles' }).click()
  await page.getByRole('button', { name: 'Change profile' }).click()
  await expect(page.getByRole('heading', { name: 'Change this game’s Windows emulator?' })).toBeVisible()
  await page.getByRole('radio', { name: /VBA-M/ }).check()
  const update = page.getByRole('button', { name: 'Update profile' })
  await expect(update).toBeDisabled()
  await page.getByRole('checkbox', { name: /closed this game in every emulator/ }).check()
  await update.click()
  await expect(page.locator('.toast')).toContainText('Windows profile changed to VBA-M')
})
