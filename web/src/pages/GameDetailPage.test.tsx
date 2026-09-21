import { render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import type { GameDetail } from '../types'
import { GameDetailPage } from './GameDetailPage'

const api = vi.hoisted(() => ({
  createIdempotencyKey: vi.fn(() => 'test-key'),
  createManualSnapshot: vi.fn(),
  loadGame: vi.fn(),
  mapSaveToGame: vi.fn(),
  restoreRevision: vi.fn(),
  uploadArtwork: vi.fn(),
}))

vi.mock('../lib/api', () => api)

const game: GameDetail = {
  id: 'game-1',
  title: 'Advance Wars',
  platform: 'GBA',
  saveName: 'Advance Wars.sav',
  emulator: 'RetroArch mGBA ↔ mGBA',
  updatedAt: '2026-09-21T10:00:00.000Z',
  sourceDeviceId: 'windows',
  sourceDeviceName: 'Windows PC',
  deliveryState: 'delivered',
  currentRevisionId: 'revision-current',
  revisionCount: 1,
  hasConflict: false,
  accent: '#e5b95c',
  revisions: [{
    id: 'revision-current',
    gameId: 'game-1',
    shortHash: 'aabbccdd',
    sourceDeviceId: 'windows',
    sourceDeviceName: 'Windows PC',
    sourceModifiedAt: '2026-09-21T10:00:00.000Z',
    observedAt: '2026-09-21T10:00:00.000Z',
    size: 131_088,
    provenance: 'confirmed',
    kind: 'capture',
    state: 'head',
  }],
  bindings: [{
    id: 'binding-windows',
    endpointId: 'windows',
    endpointName: 'Windows PC',
    relativePath: 'GBA/Advance Wars.sav',
    extension: '.sav',
    baselineRevisionId: 'revision-baseline',
    deliveryState: 'delivered',
    lastDeliveredAt: '2026-09-21T10:00:00.000Z',
    profileId: 'windows-mgba',
    profileName: 'Standalone mGBA',
    format: 'raw-battery+opaque-rtc',
    hasRtc: true,
  }],
}

describe('GameDetailPage Windows GBA override', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    api.loadGame.mockResolvedValue({ data: game, isDemo: false })
    api.mapSaveToGame.mockResolvedValue(undefined)
  })

  it('requires acknowledgement and preserves the binding path when changing profile', async () => {
    const user = userEvent.setup()
    render(<GameDetailPage gameId={game.id} navigate={vi.fn()} demoMode={false} />)

    await user.click(await screen.findByRole('button', { name: 'Change profile' }))
    const dialog = screen.getByRole('dialog')
    expect(within(dialog).getByText('GBA/Advance Wars.sav')).toBeInTheDocument()
    expect(within(dialog).getByText('revision-baseline')).toBeInTheDocument()

    await user.click(within(dialog).getByRole('radio', { name: /VBA-M/ }))
    const update = within(dialog).getByRole('button', { name: 'Update profile' })
    expect(update).toBeDisabled()

    await user.click(within(dialog).getByRole('checkbox', { name: /closed this game in every emulator/i }))
    expect(update).toBeEnabled()
    await user.click(update)

    await waitFor(() => expect(api.mapSaveToGame).toHaveBeenCalledWith(
      'game-1',
      'windows',
      'windows-vbam',
      'GBA/Advance Wars.sav',
      true,
    ))
    expect(await screen.findByRole('status')).toHaveTextContent('save path and delivery baseline were kept')
  })
})
