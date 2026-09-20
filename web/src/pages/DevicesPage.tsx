import { CheckCircle2, Clock3, FolderSync, Monitor, Plus, RefreshCw, Smartphone, Wifi } from 'lucide-react'
import { PageHeader } from '../components/PageHeader'
import { HealthBadge } from '../components/StatusBadge'
import { runEndpointAction } from '../lib/api'
import { timeAgo } from '../lib/format'
import type { Endpoint } from '../types'
import { useState } from 'react'

export function DevicesPage({ endpoints, navigate, onRefresh }: { endpoints: Endpoint[]; navigate: (path: string) => void; onRefresh: () => Promise<void> }) {
  const [busy, setBusy] = useState<string | null>(null)
  const [error, setError] = useState<string | null>(null)
  const action = async (endpointId: string, next: 'scan' | 'pause' | 'resume') => {
    setBusy(`${endpointId}-${next}`); setError(null)
    try { await runEndpointAction(endpointId, next); await onRefresh() }
    catch (cause) { setError(cause instanceof Error ? cause.message : 'Syncthing action failed') }
    finally { setBusy(null) }
  }
  return (
    <>
      <PageHeader
        eyebrow="Syncthing endpoints"
        title="Devices"
        description="Each device has its own isolated folder. ThorSync safely brokers compatible saves between them."
        actions={<button type="button" className="button button--primary" onClick={() => navigate('/onboarding')}><Plus size={17} /> Add device</button>}
      />
      {error && <p className="form-error">{error}</p>}
      <div className="device-grid">
        {endpoints.map((endpoint) => {
          const Icon = endpoint.kind === 'thor' ? Smartphone : Monitor
          return (
            <article className="device-card" key={endpoint.id}>
              <div className="device-card__top">
                <span className={`device-card__icon device-card__icon--${endpoint.kind}`}><Icon size={26} /></span>
                <HealthBadge state={endpoint.status} />
              </div>
              <h2>{endpoint.name}</h2>
              <p>{endpoint.profile}</p>
              <dl>
                <div><dt><FolderSync size={14} /> Hub folder</dt><dd><code>{endpoint.folder}</code></dd></div>
                <div><dt><Clock3 size={14} /> Last seen</dt><dd>{timeAgo(endpoint.lastSeenAt)}</dd></div>
                <div><dt><Wifi size={14} /> Client</dt><dd>{endpoint.version ?? 'Unknown version'}</dd></div>
              </dl>
              <div className="device-card__footer">
                {endpoint.pendingFiles ? (
                  <span className="pending-copy"><RefreshCw size={14} className="spin-slow" />{endpoint.pendingFiles} file pending</span>
                ) : (
                  <span className="ready-copy"><CheckCircle2 size={14} />Fully up to date</span>
                )}
                <span className="device-actions"><button type="button" className="text-button" disabled={busy !== null} onClick={() => void action(endpoint.id, 'scan')}><RefreshCw size={13} />Scan</button><button type="button" className="text-button" disabled={busy !== null} onClick={() => void action(endpoint.id, endpoint.status === 'offline' ? 'resume' : 'pause')}>{endpoint.status === 'offline' ? 'Resume' : 'Pause'}</button></span>
              </div>
            </article>
          )
        })}
      </div>
      <section className="info-strip">
        <FolderSync size={20} />
        <div><strong>Syncthing handles transport</strong><p>Pairing, discovery, and network transfer stay in Syncthing. ThorSync observes the hub folders, archives every version, and prevents divergent progress from overwriting itself.</p></div>
      </section>
    </>
  )
}
