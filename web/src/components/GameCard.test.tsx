import { render, screen } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import { GameCard } from './GameCard'
import type { Game } from '../types'

const game: Game = {
  id: 'game-1', title: 'Advance Wars', platform: 'GBA', saveName: 'Advance Wars.srm',
  emulator: 'mGBA', updatedAt: new Date().toISOString(), sourceDeviceId: 'thor',
  sourceDeviceName: 'AYN Thor', deliveryState: 'delivered', currentRevisionId: 'rev-1',
  revisionCount: 4, hasConflict: false, accent: '#e5b95c',
}

describe('GameCard', () => {
  it('shows the game and its save source', () => {
    render(<GameCard game={game} onOpen={vi.fn()} />)
    expect(screen.getByText('Advance Wars')).toBeInTheDocument()
    expect(screen.getByText(/AYN Thor/)).toBeInTheDocument()
  })
})
