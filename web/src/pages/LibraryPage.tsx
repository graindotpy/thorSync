import { ArrowRight, ArrowUpDown, FileText, LayoutGrid, Rows3, Search, X } from 'lucide-react'
import { useMemo, useState } from 'react'
import { GameCard, GameLedger } from '../components/GameCard'
import { ImportGames } from '../components/ImportGames'
import { Modal } from '../components/Modal'
import { PageHeader } from '../components/PageHeader'
import { fileSize, percent } from '../lib/format'
import { needsAttention } from '../lib/present'
import { readLocal, writeLocal } from '../lib/theme'
import type { ArchiveUsage, Game } from '../types'

interface LibraryPageProps {
  games: Game[]
  archive: ArchiveUsage
  conflictCount?: number
  unassignedCount?: number
  demoMode?: boolean
  navigate: (path: string) => void
  onRefresh?: () => Promise<void>
}

type Filter = 'all' | 'GBA' | 'NDS' | 'attention'
type Sort = 'recent' | 'title'
type View = 'shelf' | 'ledger'

const filterLabels: Record<Filter, string> = { all: 'All', GBA: 'GBA', NDS: 'NDS', attention: 'Attention' }

export function LibraryPage({ games, archive, conflictCount = 0, unassignedCount = 0, demoMode = false, navigate, onRefresh }: LibraryPageProps) {
  const [query, setQuery] = useState('')
  const [filter, setFilter] = useState<Filter>('all')
  const [sort, setSort] = useState<Sort>('recent')
  const [view, setView] = useState<View>(() => readLocal('thorsync.library-view', ['shelf', 'ledger'] as const, 'shelf'))
  const [importing, setImporting] = useState(false)

  const visibleGames = useMemo(() => {
    const normalized = query.trim().toLowerCase()
    return games
      .filter((game) => {
        if (filter === 'attention') return needsAttention(game)
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

  const inStep = games.filter((game) => game.deliveryState === 'delivered').length
  const delivering = games.filter((game) => game.deliveryState === 'syncing').length
  const attention = games.filter(needsAttention).length
  const waiting = conflictCount + unassignedCount
  const usedPercent = percent(archive.usedBytes, archive.quotaBytes)

  const changeView = (next: View) => {
    setView(next)
    writeLocal('thorsync.library-view', next)
  }

  const open = (id: string) => navigate(`/games/${encodeURIComponent(id)}`)

  const waitingDetail = [
    conflictCount ? `${conflictCount} ${conflictCount === 1 ? 'conflict' : 'conflicts'} between devices` : null,
    unassignedCount ? `${unassignedCount} ${unassignedCount === 1 ? 'file' : 'files'} not yet matched to a game` : null,
  ].filter(Boolean).join(' and ')

  return (
    <>
      <PageHeader
        title="Library"
        description={games.length
          ? `${games.length} ${games.length === 1 ? 'game' : 'games'} in the archive. ${inStep} in step on both devices${delivering ? `, ${delivering} delivering` : ''}${attention ? `, ${attention} ${attention === 1 ? 'needs' : 'need'} attention` : ''}.`
          : 'Nothing archived yet.'}
        actions={<button className="button" type="button" onClick={() => setImporting(true)}><FileText size={16} />Import game names</button>}
      />

      {waiting > 0 && (
        <aside className="callout callout--attention">
          <div>
            <strong>{waiting === 1 ? 'One thing is' : `${waiting} things are`} waiting for a decision.</strong>
            <p>{waitingDetail}. Affected saves stay where they are until you choose.</p>
          </div>
          <button type="button" className="button button--primary" onClick={() => navigate('/review')}>Go to review<ArrowRight size={16} /></button>
        </aside>
      )}

      <dl className="figures" aria-label="Library summary">
        <div><dt>Games</dt><dd>{games.length}</dd></div>
        <div><dt>In step</dt><dd>{inStep}</dd></div>
        <div className={attention ? 'figures__item--attention' : ''}><dt>Need attention</dt><dd>{attention}</dd></div>
        <div className="figures__wide">
          <dt>Archive</dt>
          <dd>{fileSize(archive.usedBytes)}<small> of {fileSize(archive.quotaBytes)}</small></dd>
          <div className="meter" role="meter" aria-label="Archive quota used" aria-valuenow={usedPercent} aria-valuemin={0} aria-valuemax={100}><span style={{ width: `${usedPercent}%` }} /></div>
        </div>
      </dl>

      {games.length > 0 ? (
        <section aria-labelledby="library-games">
          <h2 id="library-games" className="visually-hidden">Games</h2>
          <div className="toolbar">
            <label className="search">
              <Search size={16} aria-hidden="true" />
              <input value={query} onChange={(event) => setQuery(event.target.value)} placeholder="Search by game or device" aria-label="Search games" />
              {query && <button type="button" className="icon-button" aria-label="Clear search" onClick={() => setQuery('')}><X size={15} /></button>}
            </label>
            <div className="segmented" role="group" aria-label="Filter games">
              {(['all', 'GBA', 'NDS', 'attention'] as Filter[]).map((value) => (
                <button type="button" key={value} aria-pressed={filter === value} onClick={() => setFilter(value)}>{filterLabels[value]}</button>
              ))}
            </div>
            <div className="toolbar__end">
              <button type="button" className="button button--ghost" onClick={() => setSort((value) => (value === 'recent' ? 'title' : 'recent'))}>
                <ArrowUpDown size={15} />{sort === 'recent' ? 'Most recent' : 'A–Z'}
              </button>
              <div className="segmented segmented--icons" role="group" aria-label="Library view">
                <button type="button" aria-pressed={view === 'shelf'} onClick={() => changeView('shelf')} title="Shelf view"><LayoutGrid size={16} /><span className="visually-hidden">Shelf</span></button>
                <button type="button" aria-pressed={view === 'ledger'} onClick={() => changeView('ledger')} title="Ledger view"><Rows3 size={16} /><span className="visually-hidden">Ledger</span></button>
              </div>
            </div>
          </div>

          {visibleGames.length === 0 ? (
            <div className="empty">
              <h3>No games match</h3>
              <p>Try another title, or clear the filters.</p>
              <button type="button" className="button" onClick={() => { setQuery(''); setFilter('all') }}>Clear filters</button>
            </div>
          ) : view === 'shelf' ? (
            <div className="shelf">
              {visibleGames.map((game) => <GameCard key={game.id} game={game} onOpen={open} />)}
            </div>
          ) : (
            <GameLedger games={visibleGames} onOpen={open} />
          )}
        </section>
      ) : (
        <div className="empty empty--large">
          <h2>No games in the archive yet</h2>
          <p>ThorSync names games from a RetroArch playlist or from ROM hashes worked out in your browser. Any saves it finds before then wait in Review.</p>
          <div className="empty__actions">
            <button type="button" className="button button--primary" onClick={() => setImporting(true)}><FileText size={16} />Import game names</button>
            {unassignedCount > 0 && <button type="button" className="button" onClick={() => navigate('/review')}>See {unassignedCount} unmatched {unassignedCount === 1 ? 'file' : 'files'}</button>}
          </div>
        </div>
      )}

      <Modal
        open={importing}
        wide
        title="Import game names"
        description="Match save files to games. Nothing here moves or changes a save."
        onClose={() => setImporting(false)}
        footer={<button type="button" className="button button--primary" onClick={() => setImporting(false)}>Done</button>}
      >
        <ImportGames demoMode={demoMode} onImported={() => void onRefresh?.()} />
      </Modal>
    </>
  )
}
