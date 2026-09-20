import {
  AlertTriangle,
  ArrowLeft,
	BookmarkPlus,
  Check,
  CheckCircle2,
  Clock3,
  Copy,
  FileArchive,
  History,
  Monitor,
  RefreshCw,
  RotateCcw,
  ShieldCheck,
  Smartphone,
  Upload,
} from 'lucide-react'
import { useCallback, useEffect, useState } from 'react'
import { DeliveryBadge } from '../components/StatusBadge'
import { Modal } from '../components/Modal'
import { PlatformArt } from '../components/PlatformArt'
import { createIdempotencyKey, createManualSnapshot, loadGame, restoreRevision, uploadArtwork } from '../lib/api'
import { fileSize, fullDate, sentenceCase, timeAgo } from '../lib/format'
import type { GameDetail, Revision } from '../types'

interface GameDetailPageProps {
  gameId: string
  navigate: (path: string) => void
  demoMode: boolean
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
      setNotice(`Restore queued from revision ${selectedRevision.shortHash}`)
      closeRestore()
      window.setTimeout(() => setNotice(null), 4_000)
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
			setNotice('Manual snapshot added to the timeline')
			window.setTimeout(() => setNotice(null), 4_000)
		} catch (cause) {
			setError(cause instanceof Error ? cause.message : 'Snapshot could not be created')
		} finally {
			setSnapshotting(false)
		}
	}

  if (loading) return <GameDetailSkeleton />

  if (error || !game) {
    return (
      <div className="detail-error">
        <span><AlertTriangle size={24} /></span>
        <h1>We couldn’t open this game</h1>
        <p>{error ?? 'This game may have been removed.'}</p>
        <button className="button button--primary" type="button" onClick={() => navigate('/')}>
          Back to library
        </button>
      </div>
    )
  }

  return (
    <>
      <button className="back-button" type="button" onClick={() => navigate('/')}>
        <ArrowLeft size={16} /> Library
      </button>

      <section className="game-hero" style={{ '--game-accent': game.accent } as React.CSSProperties}>
        <div className="game-hero__art"><PlatformArt platform={game.platform} title={game.title} accent={game.accent} size="hero" artworkUrl={game.artworkUrl} /><label className="art-upload"><input type="file" accept="image/png,image/jpeg,image/webp" disabled={uploadingArt} onChange={async (event) => { const file = event.target.files?.[0]; if (!file) return; setUploadingArt(true); try { if (!demoMode) { await uploadArtwork(game.id, file); await fetchGame() } setNotice('Custom cover updated') } catch (cause) { setError(cause instanceof Error ? cause.message : 'Artwork upload failed') } finally { setUploadingArt(false); event.target.value = '' } }} /><Upload size={14} />{uploadingArt ? 'Uploading…' : 'Custom cover'}</label></div>
        <div className="game-hero__copy">
          <div className="game-hero__tags">
            <span className="platform-chip">{game.platform}</span>
            <DeliveryBadge state={game.deliveryState} />
          </div>
          <h1>{game.title}</h1>
          <p>{game.emulator}</p>
          <div className="game-hero__facts">
            <span><Clock3 size={15} /><small>Last save</small><strong>{timeAgo(game.updatedAt)}</strong></span>
            <span>{game.sourceDeviceId === 'thor' ? <Smartphone size={15} /> : <Monitor size={15} />}<small>Saved on</small><strong>{game.sourceDeviceName}</strong></span>
            <span><History size={15} /><small>History</small><strong>{game.revisionCount} revisions</strong></span>
          </div>
        </div>
        <div className="game-hero__status">
          <span className={`hero-health hero-health--${game.hasConflict ? 'warn' : 'ok'}`}>
            {game.hasConflict ? <AlertTriangle size={20} /> : <ShieldCheck size={20} />}
          </span>
          <div>
            <strong>{game.hasConflict ? 'Sync paused safely' : 'Progress protected'}</strong>
            <p>{game.healthMessage ?? 'The current save is archived and delivered to both devices.'}</p>
          </div>
        </div>
      </section>

      <div className="detail-layout">
        <section className="panel timeline-panel">
          <div className="panel__header">
            <div><span className="eyebrow">Immutable history</span><h2>Save timeline</h2></div>
			<div className="timeline-actions"><button type="button" className="button button--quiet" disabled={snapshotting} onClick={() => void takeSnapshot()}>{snapshotting ? <RefreshCw size={14} className="spin-slow" /> : <BookmarkPlus size={14} />}{snapshotting ? 'Saving…' : 'Snapshot now'}</button><span className="count-pill">{game.revisions.length} shown</span></div>
          </div>
          <div className="timeline">
            {game.revisions.map((revision, index) => (
              <article className={`timeline-item ${revision.state === 'head' ? 'timeline-item--head' : ''}`} key={revision.id}>
                <div className="timeline-item__rail">
                  <span>{revision.state === 'head' ? <Check size={13} /> : null}</span>
                  {index < game.revisions.length - 1 && <i />}
                </div>
                <div className="timeline-item__body">
                  <div className="timeline-item__top">
                    <div>
                      <strong>{revision.state === 'head' ? 'Current save' : sentenceCase(revision.kind)}</strong>
                      <span className={`provenance provenance--${revision.provenance}`}>{revision.provenance}</span>
                    </div>
                    <time title={fullDate(revision.observedAt)}>{timeAgo(revision.observedAt)}</time>
                  </div>
                  <p>
                    {revision.sourceDeviceId === 'thor' ? <Smartphone size={14} /> : <Monitor size={14} />}
                    {revision.sourceDeviceName}
                    <span>·</span>
                    {fileSize(revision.size)}
                  </p>
                  <div className="timeline-item__bottom">
                    <code><Copy size={12} /> {revision.shortHash}</code>
                    {revision.state !== 'head' && revision.state !== 'quarantined' && (
                      <button type="button" className="text-button" onClick={() => setSelectedRevision(revision)}>
                        <RotateCcw size={14} /> Restore this save
                      </button>
                    )}
                  </div>
                  {revision.note && <small className="timeline-item__note">{revision.note}</small>}
                </div>
              </article>
            ))}
          </div>
        </section>

        <aside className="detail-aside">
          <section className="panel bindings-panel">
            <div className="panel__header"><div><span className="eyebrow">Destinations</span><h2>Device copies</h2></div></div>
            <div className="binding-list">
              {game.bindings.map((binding) => (
                <div className="binding" key={binding.id}>
                  <span className="binding__icon">
                    {binding.endpointId === 'thor' ? <Smartphone size={18} /> : <Monitor size={18} />}
                  </span>
                  <div>
                    <strong>{binding.endpointName}</strong>
                    <code>{binding.relativePath}</code>
                    <small>{binding.lastDeliveredAt ? `Delivered ${timeAgo(binding.lastDeliveredAt)}` : 'No copy found'}</small>
                  </div>
                  <DeliveryBadge state={binding.deliveryState} />
                </div>
              ))}
            </div>
          </section>

          <section className="panel safety-panel">
            <FileArchive size={20} />
            <div><strong>Originals stay immutable</strong><p>A restore creates a new revision. It never erases or rewinds this history.</p></div>
          </section>
        </aside>
      </div>

      <Modal
        open={Boolean(selectedRevision)}
        title="Restore this save?"
        description="ThorSync will make this older progress the newest revision and deliver it to both devices."
        onClose={closeRestore}
        footer={
          <>
            <button type="button" className="button button--quiet" onClick={closeRestore} disabled={restoring}>Cancel</button>
            <button type="button" className="button button--danger" disabled={!acknowledged || restoring} onClick={() => void confirmRestore()}>
              {restoring ? <RefreshCw size={16} className="spin-slow" /> : <RotateCcw size={16} />}
              {restoring ? 'Queuing restore…' : 'Restore and deliver'}
            </button>
          </>
        }
      >
        {selectedRevision && (
          <div className="restore-summary">
            <div><span>Revision</span><code>{selectedRevision.shortHash}</code></div>
            <div><span>Captured</span><strong>{fullDate(selectedRevision.observedAt)}</strong></div>
            <div><span>From</span><strong>{selectedRevision.sourceDeviceName}</strong></div>
          </div>
        )}
        <label className="confirm-check">
          <input type="checkbox" checked={acknowledged} onChange={(event) => setAcknowledged(event.target.checked)} />
          <span><CheckCircle2 size={19} /><strong>I’ve closed this game on both devices</strong><small>This prevents an emulator from overwriting the restored save with stale data.</small></span>
        </label>
      </Modal>

      {notice && <div className="toast" role="status"><CheckCircle2 size={17} />{notice}</div>}
    </>
  )
}

function GameDetailSkeleton() {
  return (
    <div className="detail-skeleton" aria-label="Loading game">
      <div className="skeleton skeleton--line" />
      <div className="skeleton skeleton--hero" />
      <div className="detail-layout">
        <div className="skeleton skeleton--panel" />
        <div className="skeleton skeleton--panel" />
      </div>
    </div>
  )
}
