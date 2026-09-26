import type { CSSProperties } from 'react'
import { coverCloth, initialOf } from '../lib/present'
import type { Platform } from '../types'

interface PlatformArtProps {
  platform: Platform
  title: string
  seed: string
  size?: 'card' | 'hero' | 'mini'
  artworkUrl?: string
}

// A cloth-bound cover: the game's initial set like a book's ornamental capital.
export function PlatformArt({ platform, title, seed, size = 'card', artworkUrl }: PlatformArtProps) {
  return (
    <div className={`cover cover--${size}`} style={{ '--cloth': coverCloth(seed) } as CSSProperties}>
      {artworkUrl ? (
        <img className="cover__image" src={artworkUrl} alt={`${title} cover`} />
      ) : (
        <div className="cover__cloth" aria-hidden="true">
          <span className="cover__platform">{platform === 'NDS' ? 'Nintendo DS' : 'Game Boy Advance'}</span>
          <span className="cover__initial">{initialOf(title)}</span>
          <span className="cover__rule" />
        </div>
      )}
    </div>
  )
}
