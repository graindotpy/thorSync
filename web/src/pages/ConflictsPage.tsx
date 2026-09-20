import {
  AlertTriangle,
  CheckCircle2,
  Clock3,
  GitBranch,
  Monitor,
  RefreshCw,
  ShieldCheck,
  Smartphone,
} from 'lucide-react'
import { useState } from 'react'
import { Modal } from '../components/Modal'
import { PageHeader } from '../components/PageHeader'
import { createIdempotencyKey, promoteConflict } from '../lib/api'
import { fileSize, fullDate, timeAgo } from '../lib/format'
import type { Conflict, Revision } from '../types'

interface ConflictsPageProps {
  conflicts: Conflict[]
  demoMode: boolean
  navigate: (path: string) => void
}

export function ConflictsPage({ conflicts, demoMode, navigate }: ConflictsPageProps) {
  const [resolved, setResolved] = useState<string[]>([])
  const [choice, setChoice] = useState<{ conflict: Conflict; revision: Revision } | null>(null)
  const [acknowledged, setAcknowledged] = useState(false)
  const [submitting, setSubmitting] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const visible = conflicts.filter((conflict) => !resolved.includes(conflict.id))

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
    <>
      <PageHeader
        eyebrow="Protected branches"
        title="Conflicts"
        description="When both devices move forward independently, ThorSync keeps both saves and waits for your choice."
      />
      {visible.length ? (
        <div className="conflict-list">
          {visible.map((conflict) => (
            <article className="conflict-card" key={conflict.id}>
              <div className="conflict-card__heading">
                <span className="conflict-card__icon"><GitBranch size={21} /></span>
                <div><span className="eyebrow">{conflict.platform} · paused {timeAgo(conflict.openedAt)}</span><h2>{conflict.gameTitle}</h2><p>{conflict.reason}</p></div>
                <button type="button" className="text-button" onClick={() => navigate(`/games/${conflict.gameId}`)}>Open history</button>
              </div>
              <div className="version-compare">
                <VersionChoice label="Current head" revision={conflict.currentHead} onChoose={() => setChoice({ conflict, revision: conflict.currentHead })} />
                <span className="version-compare__divider">or</span>
                <VersionChoice label="Protected branch" revision={conflict.branch} onChoose={() => setChoice({ conflict, revision: conflict.branch })} />
              </div>
              <div className="conflict-card__note"><ShieldCheck size={15} />Both byte-for-byte saves remain archived after your choice.</div>
            </article>
          ))}
        </div>
      ) : (
        <div className="empty-state empty-state--large">
          <span className="empty-state__success"><CheckCircle2 size={27} /></span>
          <h2>No conflicts to review</h2>
          <p>Every game has one clear current revision. Automatic delivery can continue safely.</p>
          <button type="button" className="button button--quiet" onClick={() => navigate('/')}>Return to library</button>
        </div>
      )}

      <Modal
        open={Boolean(choice)}
        title="Make this the current save?"
        description="Your selection becomes a new immutable head. The other version stays in history."
        onClose={() => !submitting && setChoice(null)}
        footer={<><button className="button button--quiet" type="button" disabled={submitting} onClick={() => setChoice(null)}>Cancel</button><button className="button button--primary" type="button" disabled={!acknowledged || submitting} onClick={() => void confirm()}>{submitting ? <RefreshCw size={16} className="spin-slow" /> : <CheckCircle2 size={16} />}{submitting ? 'Promoting…' : 'Use this version'}</button></>}
      >
        {choice && <div className="chosen-version"><span>{choice.revision.sourceDeviceId === 'thor' ? <Smartphone size={20} /> : <Monitor size={20} />}</span><div><strong>{choice.revision.sourceDeviceName}</strong><p>{fullDate(choice.revision.sourceModifiedAt)} · {fileSize(choice.revision.size)}</p><code>{choice.revision.shortHash}</code></div></div>}
        <label className="confirm-check"><input type="checkbox" checked={acknowledged} onChange={(event) => setAcknowledged(event.target.checked)} /><span><CheckCircle2 size={19} /><strong>I’ve closed this game on both devices</strong><small>ThorSync can safely deliver the chosen revision.</small></span></label>
        {error && <p className="form-error"><AlertTriangle size={14} />{error}</p>}
      </Modal>
    </>
  )
}

function VersionChoice({ label, revision, onChoose }: { label: string; revision: Revision; onChoose: () => void }) {
  const Icon = revision.sourceDeviceId === 'thor' ? Smartphone : Monitor
  return (
    <div className="version-choice">
      <span className="version-choice__label">{label}</span>
      <div className="version-choice__device"><span><Icon size={20} /></span><div><strong>{revision.sourceDeviceName}</strong><small><Clock3 size={12} />{fullDate(revision.sourceModifiedAt)}</small></div></div>
      <dl><div><dt>Observed</dt><dd>{timeAgo(revision.observedAt)}</dd></div><div><dt>Size</dt><dd>{fileSize(revision.size)}</dd></div><div><dt>Revision</dt><dd><code>{revision.shortHash}</code></dd></div></dl>
      <button type="button" className="button button--quiet button--wide" onClick={onChoose}>Choose this save</button>
    </div>
  )
}
