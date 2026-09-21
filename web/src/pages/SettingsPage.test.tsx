import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { SettingsPage } from './SettingsPage'

const api = vi.hoisted(() => ({
  setPropagation: vi.fn(),
  setWindowsGbaProfile: vi.fn(),
}))

vi.mock('../lib/api', () => api)

describe('SettingsPage existing-install emulator setup', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    api.setPropagation.mockResolvedValue(undefined)
    api.setWindowsGbaProfile.mockResolvedValue(undefined)
  })

  it('focuses the one-time choice and will not migrate mappings without acknowledgement', async () => {
    const user = userEvent.setup()
    render(<SettingsPage
      propagationEnabled
      quotaBytes={16 * 1024 ** 3}
      emulatorSettings={{ windowsGbaProfileId: 'windows-mgba', configured: false, affectedBindings: 3 }}
      onSaved={vi.fn().mockResolvedValue(undefined)}
    />)

    expect(screen.getByText(/One-time setup required/)).toBeInTheDocument()
    expect(screen.getByText(/3 existing Windows GBA mappings/)).toBeInTheDocument()

    await user.click(screen.getByRole('button', { name: 'Save preferences' }))
    expect(screen.getByText(/Close mGBA, VBA-M, and RetroArch/)).toBeInTheDocument()
    expect(api.setWindowsGbaProfile).not.toHaveBeenCalled()

    await user.click(screen.getByRole('checkbox', { name: /closed all emulators/i }))
    await user.click(screen.getByRole('button', { name: 'Save preferences' }))
    await waitFor(() => expect(api.setWindowsGbaProfile).toHaveBeenCalledWith('windows-mgba', true))
  })
})
