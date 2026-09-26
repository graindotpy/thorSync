import { timeAgo } from '../lib/format'
import { gameStateLine, needsAttention } from '../lib/present'
import type { Game } from '../types'
import { DeliveryBadge } from './StatusBadge'
import { PlatformArt } from './PlatformArt'

interface GameCardProps {
  game: Game
  onOpen: (gameId: string) => void
}

export function GameCard({ game, onOpen }: GameCardProps) {
  const attention = needsAttention(game)
  return (
    <article
      className={`game-card ${attention ? 'game-card--attention' : ''}`}
      tabIndex={0}
      role="button"
      aria-label={`Open ${game.title}`}
      onClick={() => onOpen(game.id)}
      onKeyDown={(event) => {
        if (event.key === 'Enter' || event.key === ' ') {
          event.preventDefault()
          onOpen(game.id)
        }
      }}
    >
      <div className="game-card__cover">
        <PlatformArt platform={game.platform} title={game.title} seed={game.id} artworkUrl={game.artworkUrl} />
        {game.hasConflict && <span className="stamp">Needs review</span>}
      </div>
      <div className="game-card__body">
        <h3>{game.title}</h3>
        <div className="game-card__state">
          <DeliveryBadge state={game.deliveryState} />
          <time dateTime={game.updatedAt}>{timeAgo(game.updatedAt)}</time>
        </div>
        <p className="game-card__meta">Last saved on {game.sourceDeviceName}</p>
        {attention && <p className="game-card__note">{gameStateLine(game)}</p>}
      </div>
    </article>
  )
}

export function GameLedger({ games, onOpen }: { games: Game[]; onOpen: (gameId: string) => void }) {
  return (
    <div className="ledger-wrap">
      <table className="ledger">
        <thead>
          <tr>
            <th scope="col">Game</th>
            <th scope="col">System</th>
            <th scope="col">State</th>
            <th scope="col">Last saved on</th>
            <th scope="col" className="ledger__num">Revisions</th>
            <th scope="col" className="ledger__num">When</th>
          </tr>
        </thead>
        <tbody>
          {games.map((game) => (
            <tr key={game.id} className={needsAttention(game) ? 'ledger__row--attention' : ''} onClick={() => onOpen(game.id)}>
              <td>
                <button type="button" className="ledger__title" aria-label={`Open ${game.title}`} onClick={(event) => { event.stopPropagation(); onOpen(game.id) }}>
                  {game.title}
                </button>
                {needsAttention(game) && <small className="ledger__note">{gameStateLine(game)}</small>}
              </td>
              <td className="ledger__muted">{game.platform}</td>
              <td><DeliveryBadge state={game.deliveryState} /></td>
              <td className="ledger__muted">{game.sourceDeviceName}</td>
              <td className="ledger__num">{game.revisionCount}</td>
              <td className="ledger__num ledger__muted"><time dateTime={game.updatedAt}>{timeAgo(game.updatedAt)}</time></td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  )
}
