import { RefreshCw } from 'lucide-react'
import { useState } from 'react'
import { ConfirmCheck, Modal } from '../components/Modal'
import { SectionHeader } from '../components/PageHeader'
import { createIdempotencyKey, promoteConflict } from '../lib/api'
import { fileSize, fullDate, timeAgo } from '../lib/format'
import type { Conflict, Revision } from '../types'

interface ConflictsPageProps {
  conflicts: Conflict[]
  demoMode: boolean
  navigate: (path: string) => void
}

// Rendered as the first section of the Review page.
export function ConflictsPage({ conflicts, demoMode, navigate }: ConflictsPageProps) {
  const [resolved, setResolved] = useState<string[]>([])
  const [choice, setChoice] = useState<{ conflict: Conflict; revision: Revision } | null>(null)
  const [acknowledged, setAcknowledged] = useState(false)
  const [submitting, setSubmitting] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const visible = conflicts.filter((conflict) => !resolved.includes(conflict.id))

  const close = () => {
    if (submitting) return
    setChoice(null)
    setAcknowledged(false)
    setError(null)
  }

  const confirm = async () => {
    if (!choice || !acknowledged) return
    setSubmitting(true)
    setError(null)
    try {
      if (!demoMode) {
        await promoteConflict(choice.conflict.id, {
          revisionId: choice.revision.id,
          expectedHeadId: choice.conflict.currentHead.id,
          emulatorClosed: true,
          idempotencyKey: createIdempotencyKey(),
        })
      }
      setResolved((items) => [...items, choice.conflict.id])
      setChoice(null)
      setAcknowledged(false)
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : 'Could not resolve this conflict')
    } finally {
      setSubmitting(false)
    }
  }

  return (
    <section className="review-section" aria-labelledby="conflicts-heading">
      <SectionHeader
        id="conflicts-heading"
        title="Conflicts"
        count={visible.length}
        description="Both devices moved on from the same save. ThorSync kept both and paused delivery for that game."
      />
      {visible.length ? (
        <div className="conflict-list">
          {visible.map((conflict) => (
            <article className="conflict-card" key={conflict.id}>
              <header className="conflict-card__header">
                <div>
                  <h3>{conflict.gameTitle}</h3>
                  <p>{conflict.reason}</p>
                </div>
                <div className="conflict-card__meta">
                  <span>{conflict.platform} · paused {timeAgo(conflict.openedAt)}</span>
                  <button type="button" className="link-button" onClick={() => navigate(`/games/${encodeURIComponent(conflict.gameId)}`)}>View save history</button>
                </div>
              </header>
              <div className="versions">
                <VersionChoice label="Current head" revision={conflict.currentHead} onChoose={() => setChoice({ conflict, revision: conflict.currentHead })} />
                <span className="versions__or" aria-hidden="true">or</span>
                <VersionChoice label="Protected branch" revision={conflict.branch} onChoose={() => setChoice({ conflict, revision: conflict.branch })} />
              </div>
              <p className="conflict-card__foot">Whichever you choose, both saves stay in the archive byte for byte.</p>
            </article>
          ))}
        </div>
      ) : (
        <div className="empty empty--quiet">
          <h3>No conflicts to review</h3>
          <p>Every game has one clear current save, so automatic delivery can carry on.</p>
        </div>
      )}

      <Modal
        open={Boolean(choice)}
        title="Make this the current save?"
        description="Your choice becomes a new head revision and is delivered to both devices. The other version stays in history."
        onClose={close}
        footer={<>
          <button className="button" type="button" disabled={submitting} onClick={close}>Cancel</button>
          <button className="button button--primary" type="button" disabled={!acknowledged || submitting} onClick={() => void confirm()}>
            {submitting && <RefreshCw size={16} className="spin" />}{submitting ? 'Promoting…' : 'Use this version'}
          </button>
        </>}
      >
        {choice && (
          <dl className="summary-list">
            <div><dt>Game</dt><dd>{choice.conflict.gameTitle}</dd></div>
            <div><dt>Saved on</dt><dd>{choice.revision.sourceDeviceName}</dd></div>
            <div><dt>Modified</dt><dd>{fullDate(choice.revision.sourceModifiedAt)}</dd></div>
            <div><dt>Size</dt><dd>{fileSize(choice.revision.size)}</dd></div>
            <div><dt>Revision</dt><dd><code>{choice.revision.shortHash}</code></dd></div>
          </dl>
        )}
        <ConfirmCheck checked={acknowledged} onChange={setAcknowledged} title="I’ve closed this game on both devices" detail="Otherwise an open emulator could overwrite the chosen save with what it still holds in memory." />
        {error && <p className="form-error" role="alert">{error}</p>}
      </Modal>
    </section>
  )
}

function VersionChoice({ label, revision, onChoose }: { label: string; revision: Revision; onChoose: () => void }) {
  return (
    <div className="version">
      <span className="version__label">{label}</span>
      <strong className="version__device">{revision.sourceDeviceName}</strong>
      <span className="version__date">{fullDate(revision.sourceModifiedAt)}</span>
      <dl className="version__facts">
        <div><dt>Observed</dt><dd>{timeAgo(revision.observedAt)}</dd></div>
        <div><dt>Size</dt><dd>{fileSize(revision.size)}</dd></div>
        <div><dt>Revision</dt><dd><code>{revision.shortHash}</code></dd></div>
      </dl>
      <button type="button" className="button button--wide" onClick={onChoose}>Choose this save</button>
    </div>
  )
}
