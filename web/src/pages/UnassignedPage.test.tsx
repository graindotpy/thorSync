import { render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import type { EmulatorProfile, Game, UnassignedFile } from '../types'
import { UnassignedPage } from './UnassignedPage'

const api = vi.hoisted(() => ({ mapSaveToGame: vi.fn() }))
vi.mock('../lib/api', () => api)

const profiles: EmulatorProfile[] = [
  { id: 'windows-vbam', name: 'VBA-M', endpointId: 'windows', platform: 'gba', extension: '.sav', format: 'raw-battery' },
  { id: 'windows-mgba', name: 'Standalone mGBA', endpointId: 'windows', platform: 'gba', extension: '.sav', format: 'raw-battery+opaque-rtc' },
  { id: 'windows-melonds', name: 'melonDS', endpointId: 'windows', platform: 'nds', extension: '.sav', format: 'raw-battery' },
]

const games: Game[] = [{
  id: 'gba-game', title: 'Advance Wars', platform: 'GBA', saveName: 'Advance Wars.sav', emulator: 'mGBA',
  updatedAt: '2026-09-21T10:00:00.000Z', sourceDeviceId: 'windows', sourceDeviceName: 'Windows PC',
  deliveryState: 'paused', currentRevisionId: '', revisionCount: 0, hasConflict: false, accent: '#e5b95c',
}]

const file: UnassignedFile = {
  id: 'unassigned-1', endpointId: 'windows', relativePath: 'Advance Wars.sav', size: 64 * 1024,
  sourceModifiedAt: null, observedAt: '2026-09-21T10:00:00.000Z', provenance: 'unknown', state: 'unassigned',
  detail: 'Raw Windows GBA saves are ambiguous.',
  compatibleProfileIds: ['windows-vbam', 'windows-mgba', 'windows-melonds'],
}

describe('UnassignedPage profile choices', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    api.mapSaveToGame.mockResolvedValue(undefined)
  })

  it('filters candidates by game platform and requires an explicit choice when ambiguous', async () => {
    const user = userEvent.setup()
    render(<UnassignedPage files={[file]} games={games} profiles={profiles} onMapped={vi.fn().mockResolvedValue(undefined)} navigate={vi.fn()} />)

    await user.selectOptions(screen.getByRole('combobox', { name: 'Game for Advance Wars.sav' }), 'gba-game')
    const emulator = screen.getByRole('combobox', { name: 'Emulator for Advance Wars.sav' })
    expect(screen.getByRole('option', { name: 'VBA-M' })).toBeInTheDocument()
    expect(screen.getByRole('option', { name: 'Standalone mGBA' })).toBeInTheDocument()
    expect(screen.queryByRole('option', { name: 'melonDS' })).not.toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Map save' })).toBeDisabled()
    expect(screen.queryByText('Standalone mGBA RTC format detected')).not.toBeInTheDocument()

    await user.selectOptions(emulator, 'windows-mgba')
    await user.click(screen.getByRole('button', { name: 'Map save' }))
    await waitFor(() => expect(api.mapSaveToGame).toHaveBeenCalledWith(
      'gba-game',
      'windows',
      'windows-mgba',
      'Advance Wars.sav',
    ))
  })

  it('does not offer adapters for an unsupported quarantined file', async () => {
    const user = userEvent.setup()
    const unsupported: UnassignedFile = {
      ...file,
      id: 'unsupported',
      relativePath: 'broken.sav',
      size: 12345,
      detail: 'Quarantined: unsupported save size',
      compatibleProfileIds: [],
    }
    render(<UnassignedPage files={[unsupported]} games={games} profiles={profiles} onMapped={vi.fn()} navigate={vi.fn()} />)

    await user.selectOptions(screen.getByRole('combobox', { name: 'Game for broken.sav' }), 'gba-game')
    expect(screen.getByText('This file is not compatible with the selected game.')).toBeInTheDocument()
    const row = screen.getByText('broken.sav').closest('article')
    expect(row).not.toBeNull()
    expect(within(row as HTMLElement).getByRole('button', { name: 'Map save' })).toBeDisabled()
    expect(screen.queryByRole('combobox', { name: 'Emulator for broken.sav' })).not.toBeInTheDocument()
  })
})
