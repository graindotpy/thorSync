import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, expect, it, vi } from 'vitest'
import { OnboardingPage } from './OnboardingPage'

describe('OnboardingPage emulator setup', () => {
  it('preselects recommended mGBA and requires the emulator-closed acknowledgement', async () => {
    const user = userEvent.setup()
    render(<OnboardingPage endpoints={[]} demoMode navigate={vi.fn()} />)

    const continueButton = screen.getByRole('button', { name: 'Continue' })
    await user.click(continueButton)
    await user.click(continueButton)
    await user.click(continueButton)

    expect(screen.getByRole('heading', { name: 'Choose your Windows GBA emulator' })).toBeInTheDocument()
    expect(screen.getByRole('radio', { name: /Standalone mGBA Recommended/ })).toBeChecked()

    await user.click(continueButton)
    expect(screen.getByText(/Close all emulators and confirm/)).toBeInTheDocument()

    await user.click(screen.getByRole('checkbox', { name: /closed all emulators/i }))
    await user.click(continueButton)
    expect(screen.getByRole('heading', { name: 'Inventory existing saves' })).toBeInTheDocument()
  })
})
