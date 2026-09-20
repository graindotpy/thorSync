import {
  AlertTriangle,
  Archive,
  ArrowUpDown,
  CheckCircle2,
  Gamepad2,
  Plus,
  Search,
  SlidersHorizontal,
  X,
} from 'lucide-react'
import { useMemo, useState } from 'react'
import { fileSize, percent } from '../lib/format'
import type { ArchiveUsage, Game } from '../types'
import { GameCard } from '../components/GameCard'
import { PageHeader } from '../components/PageHeader'

interface LibraryPageProps {
  games: Game[]
  archive: ArchiveUsage
  navigate: (path: string) => void
}

type Filter = 'all' | 'GBA' | 'NDS' | 'attention'
type Sort = 'recent' | 'title'

export function LibraryPage({ games, archive, navigate }: LibraryPageProps) {
  const [query, setQuery] = useState('')
  const [filter, setFilter] = useState<Filter>('all')
  const [sort, setSort] = useState<Sort>('recent')

  const visibleGames = useMemo(() => {
    const normalized = query.trim().toLowerCase()
    return games
      .filter((game) => {
        if (filter === 'attention') return game.hasConflict || game.deliveryState === 'missing'
        if (filter !== 'all' && game.platform !== filter) return false
        return (
          !normalized ||
          game.title.toLowerCase().includes(normalized) ||
          game.sourceDeviceName.toLowerCase().includes(normalized)
        )
      })
      .sort((a, b) =>
        sort === 'title'
          ? a.title.localeCompare(b.title)
          : new Date(b.updatedAt).getTime() - new Date(a.updatedAt).getTime(),
      )
  }, [filter, games, query, sort])

  const deliveredCount = games.filter((game) => game.deliveryState === 'delivered').length
  const attentionCount = games.filter(
    (game) => game.hasConflict || game.deliveryState === 'missing',
  ).length
  const usedPercent = percent(archive.usedBytes, archive.quotaBytes)

  return (
    <>
      <PageHeader
        eyebrow="Save library"
        title="Welcome back"
        description="Your handheld progress, safe and ready wherever you play next."
        actions={
          <button className="button button--primary" type="button" onClick={() => navigate('/onboarding')}>
            <Plus size={17} /> Add games
          </button>
        }
      />

      <section className="overview-grid" aria-label="Library summary">
        <div className="summary-card summary-card--hero">
          <div className="summary-card__icon"><Gamepad2 size={19} /></div>
          <div><strong>{games.length}</strong><span>Games protected</span></div>
          <div className="summary-card__spark" aria-hidden="true"><i /><i /><i /><i /><i /><i /><i /></div>
        </div>
        <div className="summary-card">
          <div className="summary-card__icon summary-card__icon--green"><CheckCircle2 size={19} /></div>
          <div><strong>{deliveredCount}</strong><span>Ready everywhere</span></div>
        </div>
        <div className={`summary-card ${attentionCount ? 'summary-card--warn' : ''}`}>
          <div className="summary-card__icon summary-card__icon--amber"><AlertTriangle size={19} /></div>
          <div><strong>{attentionCount}</strong><span>Need attention</span></div>
        </div>
        <div className="summary-card summary-card--archive">
          <div className="summary-card__icon summary-card__icon--blue"><Archive size={19} /></div>
          <div className="summary-card__archive-copy">
            <span><strong>{fileSize(archive.usedBytes)}</strong> of {fileSize(archive.quotaBytes)}</span>
            <div className="progress-track"><span style={{ width: `${usedPercent}%` }} /></div>
            <small>{archive.revisionCount} immutable revisions</small>
          </div>
        </div>
      </section>

      <section className="library-section">
        <div className="section-heading">
          <div><h2>Your games</h2><span>{visibleGames.length} shown</span></div>
          <div className="view-label"><SlidersHorizontal size={15} /> Library controls</div>
        </div>

        <div className="library-toolbar">
          <label className="search-field">
            <Search size={17} />
            <input
              value={query}
              onChange={(event) => setQuery(event.target.value)}
              placeholder="Search games or devices"
              aria-label="Search games"
            />
            {query && (
              <button type="button" aria-label="Clear search" onClick={() => setQuery('')}><X size={15} /></button>
            )}
          </label>
          <div className="filter-tabs" role="group" aria-label="Filter games">
            {(['all', 'GBA', 'NDS', 'attention'] as Filter[]).map((value) => (
              <button
                type="button"
                key={value}
                className={filter === value ? 'filter-tabs__active' : ''}
                onClick={() => setFilter(value)}
              >
                {value === 'all' ? 'All' : value === 'attention' ? 'Attention' : value}
              </button>
            ))}
          </div>
          <button
            type="button"
            className="button button--quiet sort-button"
            onClick={() => setSort((value) => (value === 'recent' ? 'title' : 'recent'))}
          >
            <ArrowUpDown size={15} /> {sort === 'recent' ? 'Recent' : 'A–Z'}
          </button>
        </div>

        {visibleGames.length ? (
          <div className="game-grid">
            {visibleGames.map((game) => (
              <GameCard key={game.id} game={game} onOpen={(id) => navigate(`/games/${id}`)} />
            ))}
          </div>
        ) : (
          <div className="empty-state">
            <span><Search size={24} /></span>
            <h3>No games found</h3>
            <p>Try another title or clear the active filters.</p>
            <button type="button" className="button button--quiet" onClick={() => { setQuery(''); setFilter('all') }}>
              Clear filters
            </button>
          </div>
        )}
      </section>
    </>
  )
}
