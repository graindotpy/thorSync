import { FileQuestion, Gamepad2, Link2, ShieldAlert } from 'lucide-react'
import { useMemo, useState } from 'react'
import { PageHeader } from '../components/PageHeader'
import { mapSaveToGame } from '../lib/api'
import { fileSize, timeAgo } from '../lib/format'
import type { Game, UnassignedFile } from '../types'

export function UnassignedPage({ files, games, onMapped, navigate }: { files: UnassignedFile[]; games: Game[]; onMapped: () => Promise<void>; navigate: (path: string) => void }) {
  const [selection, setSelection] = useState<Record<string, string>>({})
  const [busy, setBusy] = useState<string | null>(null)
  const [error, setError] = useState<string | null>(null)
  const sortedGames = useMemo(() => [...games].sort((a, b) => a.title.localeCompare(b.title)), [games])
  const mapFile = async (file: UnassignedFile) => {
    const gameId = selection[file.id]
    const game = games.find((candidate) => candidate.id === gameId)
    if (!game) return
    const profileId = file.endpointId === 'thor'
      ? game.platform === 'GBA' ? 'thor-mgba' : 'thor-melonds-ds'
      : game.platform === 'GBA' ? 'windows-vbam' : 'windows-melonds'
    setBusy(file.id); setError(null)
    try { await mapSaveToGame(game.id, file.endpointId, profileId, file.relativePath); await onMapped() }
    catch (cause) { setError(cause instanceof Error ? cause.message : 'Could not map this save') }
    finally { setBusy(null) }
  }
  return <>
    <PageHeader eyebrow="Needs review" title="Unassigned and quarantined files" description="Recognized bytes are archived when safe. Empty, locked, ambiguous, or unexpected files remain visible here and are never delivered automatically." actions={<button className="button button--quiet" type="button" onClick={() => navigate('/onboarding')}><Gamepad2 size={16} />Import games</button>} />
    {error && <p className="form-error"><ShieldAlert size={15} />{error}</p>}
    {!files.length ? <section className="empty-panel"><FileQuestion size={28} /><h2>Everything is assigned</h2><p>Newly discovered save files will appear here before ThorSync moves them.</p></section> :
      <div className="unassigned-list">{files.map((file) => <article className="panel unassigned-row" key={file.id}>
        <span className="unassigned-row__icon"><FileQuestion size={21} /></span><div className="unassigned-row__copy"><strong>{file.relativePath}</strong><small>{file.endpointId === 'thor' ? 'AYN Thor' : 'Windows PC'} · {fileSize(file.size)} · observed {timeAgo(file.observedAt)}</small><p>{file.detail}</p></div>
        <select aria-label={`Game for ${file.relativePath}`} value={selection[file.id] ?? ''} onChange={(event) => setSelection((current) => ({ ...current, [file.id]: event.target.value }))}><option value="">Choose a game…</option>{sortedGames.map((game) => <option value={game.id} key={game.id}>{game.title} ({game.platform})</option>)}</select>
        <button className="button button--primary" type="button" disabled={!selection[file.id] || busy === file.id} onClick={() => void mapFile(file)}><Link2 size={15} />{busy === file.id ? 'Mapping…' : 'Map save'}</button>
      </article>)}</div>}
  </>
}
