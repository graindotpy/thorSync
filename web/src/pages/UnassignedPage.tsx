import { FileQuestion, Gamepad2, Link2, ShieldAlert } from 'lucide-react'
import { useMemo, useState } from 'react'
import { PageHeader } from '../components/PageHeader'
import { mapSaveToGame } from '../lib/api'
import { fileSize, timeAgo } from '../lib/format'
import type { EmulatorProfile, Game, UnassignedFile } from '../types'

interface MappingSelection {
  gameId: string
  profileId: string
}

interface UnassignedPageProps {
  files: UnassignedFile[]
  games: Game[]
  profiles: EmulatorProfile[]
  onMapped: () => Promise<void>
  navigate: (path: string) => void
}

export function UnassignedPage({ files, games, profiles, onMapped, navigate }: UnassignedPageProps) {
  const [selections, setSelections] = useState<Record<string, MappingSelection>>({})
  const [busy, setBusy] = useState<string | null>(null)
  const [error, setError] = useState<string | null>(null)
  const sortedGames = useMemo(() => [...games].sort((a, b) => a.title.localeCompare(b.title)), [games])

  const selectGame = (file: UnassignedFile, gameId: string) => {
    const game = games.find((candidate) => candidate.id === gameId)
    const candidates = game ? compatibleProfiles(file, game, profiles) : []
    const suggested = candidates.find((profile) => profile.id === file.suggestedProfileId)
    setSelections((current) => ({
      ...current,
      [file.id]: {
        gameId,
        profileId: suggested?.id ?? (candidates.length === 1 ? candidates[0].id : ''),
      },
    }))
  }

  const selectProfile = (fileId: string, profileId: string) => {
    setSelections((current) => ({
      ...current,
      [fileId]: { gameId: current[fileId]?.gameId ?? '', profileId },
    }))
  }

  const mapFile = async (file: UnassignedFile) => {
    const selection = selections[file.id]
    const game = games.find((candidate) => candidate.id === selection?.gameId)
    const validProfile = game
      ? compatibleProfiles(file, game, profiles).some((profile) => profile.id === selection.profileId)
      : false
    if (!game || !validProfile) return
    setBusy(file.id)
    setError(null)
    try {
      await mapSaveToGame(game.id, file.endpointId, selection.profileId, file.relativePath)
      await onMapped()
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : 'Could not map this save')
    } finally {
      setBusy(null)
    }
  }

  return <>
    <PageHeader eyebrow="Needs review" title="Unassigned and quarantined files" description="Recognized bytes are archived when safe. Empty, locked, ambiguous, or unexpected files remain visible here and are never delivered automatically." actions={<button className="button button--quiet" type="button" onClick={() => navigate('/onboarding')}><Gamepad2 size={16} />Import games</button>} />
    {error && <p className="form-error"><ShieldAlert size={15} />{error}</p>}
    {!files.length ? <section className="empty-panel"><FileQuestion size={28} /><h2>Everything is assigned</h2><p>Newly discovered save files will appear here before ThorSync moves them.</p></section> :
      <div className="unassigned-list">{files.map((file) => {
        const selection = selections[file.id]
        const game = games.find((candidate) => candidate.id === selection?.gameId)
        const candidates = game ? compatibleProfiles(file, game, profiles) : []
        const profile = candidates.find((candidate) => candidate.id === selection?.profileId)
        return <article className={`panel unassigned-row${file.reviewOnly ? ' unassigned-row--quarantine' : ''}`} key={file.id}>
          <span className="unassigned-row__icon">{file.reviewOnly ? <ShieldAlert size={21} /> : <FileQuestion size={21} />}</span>
          <div className="unassigned-row__copy"><strong>{file.relativePath}</strong><small>{file.endpointId === 'thor' ? 'AYN Thor' : 'Windows PC'} · {fileSize(file.size)} · observed {timeAgo(file.observedAt)}</small><p>{file.detail}</p>{file.compatibleProfileIds.length === 1 && file.compatibleProfileIds[0] === 'windows-mgba' && <small className="format-hint">Standalone mGBA RTC format detected</small>}</div>
          {file.reviewOnly ? <div className="unassigned-row__mapping"><small className="profile-choice profile-choice--error">Delivery is paused for this file. ThorSync will retry it automatically after the next complete emulator save.</small></div> : <>
            <div className="unassigned-row__mapping">
              <select aria-label={`Game for ${file.relativePath}`} value={selection?.gameId ?? ''} onChange={(event) => selectGame(file, event.target.value)}><option value="">Choose a game…</option>{sortedGames.map((candidate) => <option value={candidate.id} key={candidate.id}>{candidate.title} ({candidate.platform})</option>)}</select>
              {game && candidates.length > 1 && <select aria-label={`Emulator for ${file.relativePath}`} value={selection?.profileId ?? ''} onChange={(event) => selectProfile(file.id, event.target.value)}><option value="">Choose the emulator…</option>{candidates.map((candidate) => <option value={candidate.id} key={candidate.id}>{candidate.name}</option>)}</select>}
              {game && candidates.length === 1 && <small className="profile-choice">Adapter: {candidates[0].name}</small>}
              {game && candidates.length === 0 && <small className="profile-choice profile-choice--error">This file is not compatible with the selected game.</small>}
            </div>
            <button className="button button--primary" type="button" disabled={!game || !profile || busy === file.id} onClick={() => void mapFile(file)}><Link2 size={15} />{busy === file.id ? 'Mapping…' : 'Map save'}</button>
          </>}
        </article>
      })}</div>}
  </>
}

function compatibleProfiles(file: UnassignedFile, game: Game, profiles: EmulatorProfile[]): EmulatorProfile[] {
  const allowed = new Set(file.compatibleProfileIds ?? [])
  return profiles.filter((profile) => (
    profile.endpointId === file.endpointId
    && profile.platform.toUpperCase() === game.platform
    && allowed.has(profile.id)
  ))
}
