import { Archive, CheckCircle2, Database, ExternalLink, HardDrive, RefreshCw, Server, ShieldCheck } from 'lucide-react'
import { PageHeader } from '../components/PageHeader'
import { HealthBadge } from '../components/StatusBadge'
import { fileSize, percent, timeAgo } from '../lib/format'
import type { ArchiveUsage, DiagnosticCheck } from '../types'

export function DiagnosticsPage({ checks, archive, onRefresh }: { checks: DiagnosticCheck[]; archive: ArchiveUsage; onRefresh: () => Promise<void> }) {
  const usage = percent(archive.usedBytes, archive.quotaBytes)
  return (
    <>
      <PageHeader
        eyebrow="System health"
        title="Diagnostics"
        description="Storage, database, authentication, and sync-service readiness at a glance."
        actions={<button type="button" className="button button--quiet" onClick={() => void onRefresh()}><RefreshCw size={16} /> Run checks</button>}
      />
      <div className="diagnostics-layout">
        <section className="panel check-panel">
          <div className="panel__header"><div><span className="eyebrow">Readiness</span><h2>Dependency checks</h2></div><span className="all-good"><CheckCircle2 size={14} />All operational</span></div>
          <div className="check-list">
            {checks.map((check) => {
              const Icon = check.id === 'database' ? Database : check.id === 'archive' ? Archive : check.id === 'access' ? ShieldCheck : Server
              return (
                <article className="check-row" key={check.id}>
                  <span className="check-row__icon"><Icon size={19} /></span>
                  <div><strong>{check.name}</strong><p>{check.detail}</p></div>
                  <div className="check-row__state"><HealthBadge state={check.state} /><small>{check.latencyMs ? `${check.latencyMs} ms · ` : ''}{timeAgo(check.checkedAt)}</small></div>
                </article>
              )
            })}
          </div>
        </section>
        <aside className="panel capacity-card">
          <span className="eyebrow">Archive capacity</span>
          <div className="capacity-card__gauge" style={{ '--usage': `${usage * 3.6}deg` } as React.CSSProperties}>
            <div><strong>{usage}%</strong><span>used</span></div>
          </div>
          <h2>{fileSize(archive.usedBytes)} protected</h2>
          <p>{archive.blobCount} deduplicated blobs across {archive.revisionCount} revision observations.</p>
          <div className="capacity-card__stats">
            <span><small>Soft quota</small><strong>{fileSize(archive.quotaBytes)}</strong></span>
            <span><small>Disk free</small><strong>{fileSize(archive.freeBytes)}</strong></span>
            <span><small>Safety reserve</small><strong>{fileSize(archive.reserveBytes)}</strong></span>
          </div>
        </aside>
      </div>
      <section className="backup-callout">
        <span><HardDrive size={21} /></span>
        <div><strong>Your revision archive still needs a backup</strong><p>The archive protects against overwrites, but it lives on this server. Include <code>/DATA/AppData/thorsync/data</code> and <code>/archive</code> in your ZimaOS backup.</p></div>
        <a href="https://www.zimaspace.com/docs/zimaos" target="_blank" rel="noreferrer">ZimaOS docs <ExternalLink size={14} /></a>
      </section>
    </>
  )
}
