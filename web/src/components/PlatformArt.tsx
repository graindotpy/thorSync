import { Gamepad2 } from 'lucide-react'
import type { Platform } from '../types'

interface PlatformArtProps {
  platform: Platform
  title: string
  accent: string
  size?: 'card' | 'hero' | 'mini'
  artworkUrl?: string
}

export function PlatformArt({ platform, title, accent, size = 'card', artworkUrl }: PlatformArtProps) {
  const initials = title
    .split(/\s+/)
    .slice(0, 2)
    .map((part) => part[0])
    .join('')

  return (
    <div
      className={`platform-art platform-art--${size}`}
      style={{ '--game-accent': accent } as React.CSSProperties}
      aria-label={`${title} placeholder artwork`}
    >
      {artworkUrl && <img className="platform-art__cover" src={artworkUrl} alt={`${title} custom cover`} />}
      {!artworkUrl && <>
      <div className="platform-art__grid" />
      <span className="platform-art__platform">{platform}</span>
      <span className="platform-art__initials">{initials}</span>
      <Gamepad2 className="platform-art__icon" aria-hidden="true" />
      <span className="platform-art__stripe" />
      </>}
    </div>
  )
}
