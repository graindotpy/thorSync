import { FileText, Link2 } from 'lucide-react'
import { useMemo, useState } from 'react'
import { ImportGames } from '../components/ImportGames'
import { Modal } from '../components/Modal'
import { SectionHeader } from '../components/PageHeader'
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
  demoMode?: boolean
}

// Rendered as the second section of the Review page.
export function UnassignedPage({ files, games, profiles, onMapped, demoMode = false }: UnassignedPageProps) {
  const [selections, setSelections] = useState<Record<string, MappingSelection>>({})
  const [busy, setBusy] = useState<string | null>(null)
  const [error, setError] = useState<string | null>(null)
  const [importing, setImporting] = useState(false)
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

  return (
    <section className="review-section" aria-labelledby="unassigned-heading">
      <SectionHeader
        id="unassigned-heading"
        title="Unmatched and quarantined files"
        count={files.length}
        description="Files ThorSync archived but won’t deliver yet: saves it can’t tie to a game, and writes that looked incomplete or unexpected."
        actions={<button className="button" type="button" onClick={() => setImporting(true)}><FileText size={16} />Import game names</button>}
      />
      {error && <p className="form-error" role="alert">{error}</p>}
      {!files.length ? (
        <div className="empty empty--quiet">
          <h3>Everything is assigned</h3>
          <p>New save files show up here before ThorSync moves them anywhere.</p>
        </div>
      ) : (
        <div className="file-list">
          {files.map((file) => {
            const selection = selections[file.id]
            const game = games.find((candidate) => candidate.id === selection?.gameId)
            const candidates = game ? compatibleProfiles(file, game, profiles) : []
            const profile = candidates.find((candidate) => candidate.id === selection?.profileId)
            return (
              <article className={`file-row${file.reviewOnly ? ' file-row--quarantine' : ''}`} key={file.id}>
                <div className="file-row__copy">
                  <span className="file-row__kind">{file.reviewOnly ? 'Quarantined' : 'Unmatched'}</span>
                  <code className="file-row__path">{file.relativePath}</code>
                  <small>{file.endpointId === 'thor' ? 'AYN Thor' : 'Windows PC'} · {fileSize(file.size)} · seen {timeAgo(file.observedAt)}</small>
                  {file.detail && <p>{file.detail}</p>}
                  {file.compatibleProfileIds.length === 1 && file.compatibleProfileIds[0] === 'windows-mgba' && <small className="file-row__hint">Standalone mGBA RTC format detected</small>}
                </div>
                {file.reviewOnly ? (
                  <p className="file-row__held">Delivery is paused for this file. ThorSync will try again by itself after the emulator’s next complete save.</p>
                ) : (
                  <div className="file-row__mapping">
                    <select aria-label={`Game for ${file.relativePath}`} value={selection?.gameId ?? ''} onChange={(event) => selectGame(file, event.target.value)}>
                      <option value="">Choose a game…</option>
                      {sortedGames.map((candidate) => <option value={candidate.id} key={candidate.id}>{candidate.title} ({candidate.platform})</option>)}
                    </select>
                    {game && candidates.length > 1 && (
                      <select aria-label={`Emulator for ${file.relativePath}`} value={selection?.profileId ?? ''} onChange={(event) => selectProfile(file.id, event.target.value)}>
                        <option value="">Choose the emulator…</option>
                        {candidates.map((candidate) => <option value={candidate.id} key={candidate.id}>{candidate.name}</option>)}
                      </select>
                    )}
                    {game && candidates.length === 1 && <small className="file-row__adapter">Adapter: {candidates[0].name}</small>}
                    {game && candidates.length === 0 && <small className="file-row__adapter file-row__adapter--error">This file is not compatible with the selected game.</small>}
                    <button className="button button--primary" type="button" disabled={!game || !profile || busy === file.id} onClick={() => void mapFile(file)}>
                      <Link2 size={15} />{busy === file.id ? 'Mapping…' : 'Map save'}
                    </button>
                  </div>
                )}
              </article>
            )
          })}
        </div>
      )}

      <Modal
        open={importing}
        wide
        title="Import game names"
        description="Once a game is known, its unmatched saves can be mapped here."
        onClose={() => setImporting(false)}
        footer={<button type="button" className="button button--primary" onClick={() => setImporting(false)}>Done</button>}
      >
        <ImportGames demoMode={demoMode} onImported={() => void onMapped()} />
      </Modal>
    </section>
  )
}

function compatibleProfiles(file: UnassignedFile, game: Game, profiles: EmulatorProfile[]): EmulatorProfile[] {
  const allowed = new Set(file.compatibleProfileIds ?? [])
  return profiles.filter((profile) => (
    profile.endpointId === file.endpointId
    && profile.platform.toUpperCase() === game.platform
    && allowed.has(profile.id)
  ))
}
