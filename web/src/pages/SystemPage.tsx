import { ExternalLink, RefreshCw } from 'lucide-react'
import { useState } from 'react'
import { PageHeader, SectionHeader } from '../components/PageHeader'
import { HealthBadge } from '../components/StatusBadge'
import { runEndpointAction } from '../lib/api'
import { fileSize, percent, timeAgo } from '../lib/format'
import { systemHealth } from '../lib/present'
import type { ArchiveUsage, DiagnosticCheck, Endpoint } from '../types'

interface SystemPageProps {
  endpoints: Endpoint[]
  checks: DiagnosticCheck[]
  archive: ArchiveUsage
  onRefresh: () => Promise<void>
}

export function SystemPage({ endpoints, checks, archive, onRefresh }: SystemPageProps) {
  const [busy, setBusy] = useState<string | null>(null)
  const [checking, setChecking] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const health = systemHealth(checks)
  const usage = percent(archive.usedBytes, archive.quotaBytes)
  const thor = endpoints.find((endpoint) => endpoint.kind === 'thor')
  const windows = endpoints.find((endpoint) => endpoint.kind === 'windows')

  const action = async (endpointId: string, next: 'scan' | 'pause' | 'resume') => {
    setBusy(`${endpointId}-${next}`)
    setError(null)
    try {
      await runEndpointAction(endpointId, next)
      await onRefresh()
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : 'Syncthing action failed')
    } finally {
      setBusy(null)
    }
  }

  const runChecks = async () => {
    setChecking(true)
    try { await onRefresh() } finally { setChecking(false) }
  }

  return (
    <>
      <PageHeader
        title="System"
        description={health.state === 'healthy'
          ? 'The hub, both devices and the archive, and whether each is doing its job.'
          : `${health.label}: ${health.failing.map((check) => check.name).join(', ')}.`}
        actions={<button type="button" className="button" disabled={checking} onClick={() => void runChecks()}><RefreshCw size={16} className={checking ? 'spin' : ''} />Run checks</button>}
      />

      <figure className="topology" aria-label="Sync topology">
        <TopologyNode label={thor?.name ?? 'AYN Thor'} detail={thor ? thor.version ?? 'Syncthing client' : 'Not paired'} state={thor?.status ?? 'offline'} />
        <span className="topology__wire" aria-hidden="true" />
        <TopologyNode label="ZimaOS hub" detail="Syncthing + ThorSync" state={health.state} hub />
        <span className="topology__wire" aria-hidden="true" />
        <TopologyNode label={windows?.name ?? 'Windows PC'} detail={windows ? windows.version ?? 'Syncthing client' : 'Not paired'} state={windows?.status ?? 'offline'} />
        <figcaption>Each device syncs only with the hub, into its own folder. ThorSync moves checked saves between the two folders.</figcaption>
      </figure>

      <section className="system-section" aria-labelledby="devices-heading">
        <SectionHeader id="devices-heading" title="Devices" description="Pairing and transfer happen in Syncthing. These controls act on each device’s hub folder." />
        {error && <p className="form-error" role="alert">{error}</p>}
        {endpoints.length ? (
          <div className="device-list">
            {endpoints.map((endpoint) => (
              <article className="device" key={endpoint.id}>
                <header className="device__header">
                  <h3>{endpoint.name}</h3>
                  <HealthBadge state={endpoint.status} />
                </header>
                <p className="device__profile">{endpoint.profile}</p>
                <dl className="facts">
                  <div><dt>Hub folder</dt><dd><code>{endpoint.folder}</code></dd></div>
                  <div><dt>Last seen</dt><dd>{timeAgo(endpoint.lastSeenAt)}</dd></div>
                  <div><dt>Client</dt><dd>{endpoint.version ?? 'Unknown version'}</dd></div>
                  <div><dt>Waiting to sync</dt><dd>{endpoint.pendingFiles ? `${endpoint.pendingFiles} ${endpoint.pendingFiles === 1 ? 'file' : 'files'}` : 'Nothing'}</dd></div>
                </dl>
                <div className="device__actions">
                  <button type="button" className="button button--small" disabled={busy !== null} onClick={() => void action(endpoint.id, 'scan')}>
                    <RefreshCw size={14} className={busy === `${endpoint.id}-scan` ? 'spin' : ''} />Scan now
                  </button>
                  <button type="button" className="button button--small button--ghost" disabled={busy !== null} onClick={() => void action(endpoint.id, endpoint.status === 'offline' ? 'resume' : 'pause')}>
                    {endpoint.status === 'offline' ? 'Resume' : 'Pause'}
                  </button>
                </div>
              </article>
            ))}
          </div>
        ) : (
          <div className="empty empty--quiet"><h3>No devices yet</h3><p>Finish setup to pair the Thor and your PC with the hub.</p></div>
        )}
      </section>

      <section className="system-section" aria-labelledby="checks-heading">
        <SectionHeader id="checks-heading" title="Health checks" description={health.label} />
        <ul className="check-list">
          {checks.map((check) => (
            <li className={`check check--${check.state}`} key={check.id}>
              <div className="check__copy"><strong>{check.name}</strong><p>{check.detail}</p></div>
              <div className="check__state">
                <HealthBadge state={check.state} />
                <small>{check.latencyMs ? `${check.latencyMs} ms · ` : ''}{timeAgo(check.checkedAt)}</small>
              </div>
            </li>
          ))}
        </ul>
      </section>

      <section className="system-section" aria-labelledby="archive-heading">
        <SectionHeader id="archive-heading" title="Archive" description={`${archive.blobCount} unique files stored for ${archive.revisionCount} recorded revisions. Identical bytes are only stored once.`} />
        <div className="capacity">
          <div className="capacity__bar">
            <div className="capacity__label"><strong>{fileSize(archive.usedBytes)}</strong> used of a {fileSize(archive.quotaBytes)} soft quota<span>{usage}%</span></div>
            <div className="meter meter--large" role="meter" aria-label="Archive quota used" aria-valuenow={usage} aria-valuemin={0} aria-valuemax={100}><span style={{ width: `${usage}%` }} /><i style={{ left: '80%' }} /><i style={{ left: '90%' }} /></div>
            <small>Warnings appear at 80% and 90%.</small>
          </div>
          <dl className="facts facts--row">
            <div><dt>Disk free</dt><dd>{fileSize(archive.freeBytes)}</dd></div>
            <div><dt>Safety reserve</dt><dd>{fileSize(archive.reserveBytes)}</dd></div>
          </dl>
        </div>
        <aside className="callout">
          <div>
            <strong>The archive still needs a backup.</strong>
            <p>It protects saves from sync mistakes, but it lives on this server. Include <code>/DATA/AppData/thorsync/data</code> and <code>/archive</code> in your ZimaOS backup.</p>
          </div>
          <a className="button" href="https://www.zimaspace.com/docs/zimaos" target="_blank" rel="noreferrer">ZimaOS docs<ExternalLink size={14} /></a>
        </aside>
      </section>
    </>
  )
}

function TopologyNode({ label, detail, state, hub = false }: { label: string; detail: string; state: Endpoint['status']; hub?: boolean }) {
  return (
    <div className={`topology__node ${hub ? 'topology__node--hub' : ''}`}>
      <span className={`topology__dot state--${state}`} aria-hidden="true" />
      <strong>{label}</strong>
      <small>{detail}</small>
    </div>
  )
}
