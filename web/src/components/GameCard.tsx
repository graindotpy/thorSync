import { AlertTriangle, ArrowRight, Clock3, Monitor, Smartphone } from 'lucide-react'
import { timeAgo } from '../lib/format'
import type { Game } from '../types'
import { DeliveryBadge } from './StatusBadge'
import { PlatformArt } from './PlatformArt'

interface GameCardProps {
  game: Game
  onOpen: (gameId: string) => void
}

export function GameCard({ game, onOpen }: GameCardProps) {
  const DeviceIcon = game.sourceDeviceId === 'thor' ? Smartphone : Monitor
  return (
    <article
      className={`game-card ${game.hasConflict ? 'game-card--attention' : ''}`}
      tabIndex={0}
      role="button"
      aria-label={`Open ${game.title}`}
      onClick={() => onOpen(game.id)}
      onKeyDown={(event) => {
        if (event.key === 'Enter' || event.key === ' ') onOpen(game.id)
      }}
    >
      <div className="game-card__visual">
        <PlatformArt platform={game.platform} title={game.title} accent={game.accent} artworkUrl={game.artworkUrl} />
        <div className="game-card__badges">
          <DeliveryBadge state={game.deliveryState} />
          {game.hasConflict && (
            <span className="status-badge status-badge--paused">
              <AlertTriangle size={12} /> Review
            </span>
          )}
        </div>
        <button className="game-card__open" type="button" tabIndex={-1} aria-hidden="true">
          <ArrowRight size={17} />
        </button>
      </div>
      <div className="game-card__body">
        <div className="game-card__heading">
          <div>
            <span className="game-card__platform">{game.platform}</span>
            <h3>{game.title}</h3>
          </div>
          <span className="game-card__revisions">{game.revisionCount} saves</span>
        </div>
        <div className="game-card__meta">
          <span>
            <DeviceIcon size={14} /> {game.sourceDeviceName}
          </span>
          <span>
            <Clock3 size={14} /> {timeAgo(game.updatedAt)}
          </span>
        </div>
        {game.healthMessage && (
          <p className={`game-card__message game-card__message--${game.deliveryState}`}>
            {game.healthMessage}
          </p>
        )}
      </div>
    </article>
  )
}
