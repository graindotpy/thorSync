import { ArrowLeft, BookmarkPlus, Check, ImageUp, RefreshCw, RotateCcw } from 'lucide-react'
import { useCallback, useEffect, useState } from 'react'
import { ConfirmCheck, Modal } from '../components/Modal'
import { SectionHeader } from '../components/PageHeader'
import { DeliveryBadge } from '../components/StatusBadge'
import { PlatformArt } from '../components/PlatformArt'
import { createIdempotencyKey, createManualSnapshot, loadGame, mapSaveToGame, restoreRevision, uploadArtwork } from '../lib/api'
import { fileSize, fullDate, sentenceCase, timeAgo } from '../lib/format'
import { clockTime, shortDate } from '../lib/present'
import type { GameDetail, Revision, SaveBinding, WindowsGbaProfileId } from '../types'

interface GameDetailPageProps {
  gameId: string
  navigate: (path: string) => void
  demoMode: boolean
}

const revisionKinds: Record<Revision['kind'], string> = {
  capture: 'Captured',
  restore: 'Restored',
  promotion: 'Chosen in a conflict',
  snapshot: 'Snapshot',
}

const provenanceLabels: Record<Revision['provenance'], string> = {
  confirmed: 'Source confirmed',
  inferred: 'Source inferred',
  unknown: 'Source unknown',
}

export function GameDetailPage({ gameId, navigate, demoMode }: GameDetailPageProps) {
  const [game, setGame] = useState<GameDetail | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [selectedRevision, setSelectedRevision] = useState<Revision | null>(null)
  const [acknowledged, setAcknowledged] = useState(false)
  const [restoring, setRestoring] = useState(false)
  const [notice, setNotice] = useState<string | null>(null)
  const [uploadingArt, setUploadingArt] = useState(false)
  const [snapshotting, setSnapshotting] = useState(false)
  const [profileBinding, setProfileBinding] = useState<SaveBinding | null>(null)
  const [selectedProfileId, setSelectedProfileId] = useState<WindowsGbaProfileId>('windows-mgba')
  const [profileAcknowledged, setProfileAcknowledged] = useState(false)
  const [updatingProfile, setUpdatingProfile] = useState(false)
  const [profileError, setProfileError] = useState<string | null>(null)

  const fetchGame = useCallback(async () => {
    setLoading(true)
    try {
      const result = await loadGame(gameId)
      setGame(result.data)
      setError(null)
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : 'Could not load this game')
    } finally {
      setLoading(false)
    }
  }, [gameId])

  useEffect(() => void fetchGame(), [fetchGame])

  const showNotice = (message: string) => {
    setNotice(message)
    window.setTimeout(() => setNotice(null), 4_000)
  }

  const closeRestore = () => {
    if (restoring) return
    setSelectedRevision(null)
    setAcknowledged(false)
  }

  const confirmRestore = async () => {
    if (!game || !selectedRevision || !acknowledged) return
    setRestoring(true)
    try {
      if (!demoMode) {
        await restoreRevision(game.id, {
          revisionId: selectedRevision.id,
          expectedHeadId: game.currentRevisionId,
          emulatorClosed: true,
          idempotencyKey: createIdempotencyKey(),
        })
        await fetchGame()
      }
      showNotice(`Restore queued from revision ${selectedRevision.shortHash}`)
      closeRestore()
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : 'Restore could not be queued')
    } finally {
      setRestoring(false)
      setSelectedRevision(null)
      setAcknowledged(false)
    }
  }

  const takeSnapshot = async () => {
    if (!game || snapshotting) return
    setSnapshotting(true)
    try {
      if (!demoMode) {
        await createManualSnapshot(game.id, game.currentRevisionId)
        await fetchGame()
      }
      showNotice('Manual snapshot added to the timeline')
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : 'Snapshot could not be created')
    } finally {
      setSnapshotting(false)
    }
  }

  const changeArtwork = async (event: React.ChangeEvent<HTMLInputElement>) => {
    if (!game) return
    const file = event.target.files?.[0]
    if (!file) return
    setUploadingArt(true)
    try {
      if (!demoMode) {
        await uploadArtwork(game.id, file)
        await fetchGame()
      }
      showNotice('Custom cover updated')
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : 'Artwork upload failed')
    } finally {
      setUploadingArt(false)
      event.target.value = ''
    }
  }

  const editProfile = (binding: SaveBinding) => {
    setProfileBinding(binding)
    setSelectedProfileId(binding.profileId === 'windows-vbam' ? 'windows-vbam' : 'windows-mgba')
    setProfileAcknowledged(false)
    setProfileError(null)
  }

  const closeProfileEditor = () => {
    if (updatingProfile) return
    setProfileBinding(null)
    setProfileAcknowledged(false)
    setProfileError(null)
  }

  const confirmProfileChange = async () => {
    if (!game || !profileBinding || !profileAcknowledged || selectedProfileId === profileBinding.profileId) return
    setUpdatingProfile(true)
    setProfileError(null)
    try {
      if (!demoMode) {
        await mapSaveToGame(game.id, profileBinding.endpointId as 'windows', selectedProfileId, profileBinding.relativePath, true)
        setProfileBinding(null)
        setProfileAcknowledged(false)
        await fetchGame()
      } else {
        setGame((current) => current ? {
          ...current,
          bindings: current.bindings.map((binding) => binding.id === profileBinding.id ? {
            ...binding,
            profileId: selectedProfileId,
            profileName: selectedProfileId === 'windows-mgba' ? 'Standalone mGBA' : 'VBA-M',
            format: selectedProfileId === 'windows-mgba' ? 'raw-battery+opaque-rtc' : 'raw-battery',
          } : binding),
        } : current)
        setProfileBinding(null)
        setProfileAcknowledged(false)
      }
      showNotice(`Windows profile changed to ${selectedProfileId === 'windows-mgba' ? 'Standalone mGBA' : 'VBA-M'}; the save path and delivery baseline were kept.`)
    } catch (cause) {
      setProfileError(cause instanceof Error ? cause.message : 'Could not change this game’s Windows emulator')
    } finally {
      setUpdatingProfile(false)
    }
  }

  if (loading && !game) return <GameDetailSkeleton />

  if (error && !game) {
    return (
      <div className="empty empty--large">
        <h1>We couldn’t open this game</h1>
        <p>{error}</p>
        <button className="button button--primary" type="button" onClick={() => navigate('/')}>Back to library</button>
      </div>
    )
  }

  if (!game) return null

  return (
    <>
      <nav className="crumbs" aria-label="Breadcrumb">
        <button className="link-button" type="button" onClick={() => navigate('/')}><ArrowLeft size={15} />Library</button>
        <span aria-hidden="true">/</span>
        <span aria-current="page">{game.title}</span>
      </nav>

      <header className="game-head">
        <div className="game-head__cover">
          <PlatformArt platform={game.platform} title={game.title} seed={game.id} size="hero" artworkUrl={game.artworkUrl} />
          <label className="link-button game-head__art">
            <input type="file" accept="image/png,image/jpeg,image/webp" disabled={uploadingArt} onChange={(event) => void changeArtwork(event)} />
            <ImageUp size={14} />{uploadingArt ? 'Uploading…' : game.artworkUrl ? 'Replace cover' : 'Add a cover image'}
          </label>
        </div>
        <div className="game-head__copy">
          <p className="game-head__kicker">{game.platform === 'NDS' ? 'Nintendo DS' : 'Game Boy Advance'} · {game.emulator}</p>
          <h1>{game.title}</h1>
          <p className={`game-head__status ${game.hasConflict ? 'game-head__status--attention' : ''}`}>
            <strong>{game.hasConflict ? 'Sync paused safely.' : 'Progress protected.'}</strong>{' '}
            {sentence(game.healthMessage ?? 'The current save is archived and delivered to both devices.')}
            {game.hasConflict && <> <button type="button" className="link-button" onClick={() => navigate('/review')}>Resolve in Review</button></>}
          </p>
          <dl className="facts facts--row">
            <div><dt>State</dt><dd><DeliveryBadge state={game.deliveryState} /></dd></div>
            <div><dt>Last save</dt><dd title={fullDate(game.updatedAt)}>{timeAgo(game.updatedAt)}</dd></div>
            <div><dt>Saved on</dt><dd>{game.sourceDeviceName}</dd></div>
            <div><dt>Revisions</dt><dd>{game.revisionCount}</dd></div>
          </dl>
          {error && <p className="form-error" role="alert">{error}</p>}
        </div>
      </header>

      <div className="game-body">
        <section className="history" aria-labelledby="history-heading">
          <SectionHeader
            id="history-heading"
            title="Save history"
            description="Every version ThorSync has seen. Restoring adds a new entry at the top; nothing below is ever erased."
            actions={
              <button type="button" className="button" disabled={snapshotting} onClick={() => void takeSnapshot()}>
                {snapshotting ? <RefreshCw size={15} className="spin" /> : <BookmarkPlus size={15} />}{snapshotting ? 'Saving…' : 'Snapshot now'}
              </button>
            }
          />
          <ol className="timeline">
            {game.revisions.map((revision) => (
              <li className={`timeline__entry timeline__entry--${revision.state}`} key={revision.id}>
                <time className="timeline__when" dateTime={revision.observedAt} title={fullDate(revision.observedAt)}>
                  <span>{shortDate(revision.observedAt)}</span>
                  <span>{clockTime(revision.observedAt)}</span>
                </time>
                <span className="timeline__mark" aria-hidden="true">{revision.state === 'head' && <Check size={11} strokeWidth={3} />}</span>
                <div className="timeline__body">
                  <div className="timeline__title">
                    <strong>{revision.state === 'head' ? 'Current save' : revisionKinds[revision.kind] ?? sentenceCase(revision.kind)}</strong>
                    {revision.state === 'branch' && <span className="tag tag--warn">Branch</span>}
                    {revision.state === 'quarantined' && <span className="tag tag--danger">Quarantined</span>}
                  </div>
                  <p>{revision.sourceDeviceName} · {fileSize(revision.size)} · <span className={`provenance provenance--${revision.provenance}`}>{provenanceLabels[revision.provenance]}</span></p>
                  <code className="timeline__hash">{revision.shortHash}</code>
                  {revision.note && <p className="timeline__note">{revision.note}</p>}
                </div>
                <div className="timeline__action">
                  {revision.state !== 'head' && revision.state !== 'quarantined' && (
                    <button type="button" className="button button--small button--ghost" onClick={() => setSelectedRevision(revision)}>
                      <RotateCcw size={14} />Restore this save
                    </button>
                  )}
                </div>
              </li>
            ))}
          </ol>
        </section>

        <aside className="copies" aria-labelledby="copies-heading">
          <SectionHeader id="copies-heading" title="On each device" />
          <ul className="copy-list">
            {game.bindings.map((binding) => (
              <li className="copy" key={binding.id}>
                <div className="copy__header">
                  <strong>{binding.endpointName}</strong>
                  <DeliveryBadge state={binding.deliveryState} />
                </div>
                <code className="copy__path">{binding.relativePath}</code>
                <dl className="facts facts--compact">
                  <div><dt>Emulator</dt><dd>{binding.profileName}{binding.hasRtc ? ' · RTC kept' : ''}</dd></div>
                  <div><dt>Delivered</dt><dd>{binding.lastDeliveredAt ? timeAgo(binding.lastDeliveredAt) : 'No copy found'}</dd></div>
                </dl>
                {game.platform === 'GBA' && binding.endpointId === 'windows' && (
                  <button type="button" className="link-button" onClick={() => editProfile(binding)}>Change profile</button>
                )}
              </li>
            ))}
          </ul>
          <p className="aside-note">A restore makes a new revision. It never rewinds or deletes the history on the left.</p>
        </aside>
      </div>

      <Modal
        open={Boolean(profileBinding)}
        title="Change this game’s Windows emulator?"
        description="This overrides the Windows GBA default for this game only. ThorSync keeps the existing save path and delivery baseline."
        onClose={closeProfileEditor}
        footer={
          <>
            <button type="button" className="button" onClick={closeProfileEditor} disabled={updatingProfile}>Cancel</button>
            <button type="button" className="button button--primary" disabled={!profileAcknowledged || updatingProfile || selectedProfileId === profileBinding?.profileId} onClick={() => void confirmProfileChange()}>
              {updatingProfile && <RefreshCw size={16} className="spin" />}
              {updatingProfile ? 'Updating…' : 'Update profile'}
            </button>
          </>
        }
      >
        {profileBinding && <>
          <div className="choices" role="radiogroup" aria-label="Windows GBA emulator for this game">
            <label className="choice"><input type="radio" name="game-windows-gba-profile" value="windows-mgba" checked={selectedProfileId === 'windows-mgba'} onChange={() => setSelectedProfileId('windows-mgba')} /><span><strong>Standalone mGBA</strong><small>Keeps its optional 16-byte RTC footer.</small></span></label>
            <label className="choice"><input type="radio" name="game-windows-gba-profile" value="windows-vbam" checked={selectedProfileId === 'windows-vbam'} onChange={() => setSelectedProfileId('windows-vbam')} /><span><strong>VBA-M</strong><small>Writes raw battery-save bytes.</small></span></label>
          </div>
          <dl className="summary-list">
            <div><dt>Save path</dt><dd><code>{profileBinding.relativePath}</code></dd></div>
            <div><dt>Delivery baseline</dt><dd><code>{profileBinding.baselineRevisionId ?? 'Not established'}</code></dd></div>
          </dl>
          <ConfirmCheck checked={profileAcknowledged} onChange={setProfileAcknowledged} title="I’ve closed this game in every emulator" detail="ThorSync will immediately re-check the existing Windows save with the selected adapter." />
          {profileError && <p className="form-error" role="alert">{profileError}</p>}
        </>}
      </Modal>

      <Modal
        open={Boolean(selectedRevision)}
        title="Restore this save?"
        description="ThorSync will make this older progress the newest revision and deliver it to both devices."
        onClose={closeRestore}
        footer={
          <>
            <button type="button" className="button" onClick={closeRestore} disabled={restoring}>Cancel</button>
            <button type="button" className="button button--danger" disabled={!acknowledged || restoring} onClick={() => void confirmRestore()}>
              {restoring ? <RefreshCw size={16} className="spin" /> : <RotateCcw size={16} />}
              {restoring ? 'Queuing restore…' : 'Restore and deliver'}
            </button>
          </>
        }
      >
        {selectedRevision && (
          <dl className="summary-list">
            <div><dt>Revision</dt><dd><code>{selectedRevision.shortHash}</code></dd></div>
            <div><dt>Captured</dt><dd>{fullDate(selectedRevision.observedAt)}</dd></div>
            <div><dt>From</dt><dd>{selectedRevision.sourceDeviceName}</dd></div>
          </dl>
        )}
        <ConfirmCheck checked={acknowledged} onChange={setAcknowledged} title="I’ve closed this game on both devices" detail="This stops an emulator from overwriting the restored save with stale data." />
      </Modal>

      {notice && <div className="toast" role="status">{notice}</div>}
    </>
  )
}

function sentence(value: string): string {
  return /[.!?…]$/.test(value.trim()) ? value : `${value.trim()}.`
}

function GameDetailSkeleton() {
  return (
    <div className="skeleton-page" aria-label="Loading game">
      <div className="skeleton skeleton--line" />
      <div className="skeleton-row">
        <div className="skeleton skeleton--cover" />
        <div className="skeleton-stack"><div className="skeleton skeleton--title" /><div className="skeleton skeleton--line" /><div className="skeleton skeleton--line" /></div>
      </div>
      <div className="skeleton skeleton--panel" />
    </div>
  )
}
