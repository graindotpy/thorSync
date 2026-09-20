import {
  AlertTriangle,
  Archive,
  CheckCircle2,
  Clock3,
  Gamepad2,
  Monitor,
  RefreshCw,
  Smartphone,
  Trash2,
} from 'lucide-react'
import { useMemo, useState } from 'react'
import { PageHeader } from '../components/PageHeader'
import { fullDate, timeAgo } from '../lib/format'
import type { ActivityItem } from '../types'

interface ActivityPageProps {
  items: ActivityItem[]
  navigate: (path: string) => void
}

const activityIcons = {
  capture: Archive,
  delivery: RefreshCw,
  conflict: AlertTriangle,
  restore: Clock3,
  missing: Trash2,
  system: CheckCircle2,
}

export function ActivityPage({ items, navigate }: ActivityPageProps) {
  const [filter, setFilter] = useState<'all' | 'saves' | 'system'>('all')
  const filtered = useMemo(
    () =>
      items.filter((item) => {
        if (filter === 'system') return item.type === 'system'
        if (filter === 'saves') return item.type !== 'system'
        return true
      }),
    [filter, items],
  )

  return (
    <>
      <PageHeader
        eyebrow="Observed by ThorSync"
        title="Recent activity"
        description="A clear trail of every captured, delivered, and protected change."
        actions={
          <div className="segmented-control">
            {(['all', 'saves', 'system'] as const).map((value) => (
              <button type="button" key={value} className={filter === value ? 'active' : ''} onClick={() => setFilter(value)}>
                {value === 'all' ? 'Everything' : value === 'saves' ? 'Save events' : 'System'}
              </button>
            ))}
          </div>
        }
      />
      <section className="panel activity-panel">
        <div className="activity-list">
          {filtered.map((item) => {
            const Icon = activityIcons[item.type]
            return (
              <article className="activity-row" key={item.id}>
                <span className={`activity-row__icon activity-row__icon--${item.tone}`}><Icon size={17} /></span>
                <div className="activity-row__content">
                  <div><strong>{item.title}</strong><time title={fullDate(item.occurredAt)}>{timeAgo(item.occurredAt)}</time></div>
                  <p>{item.detail}</p>
                  <div className="activity-row__tags">
                    {item.gameTitle && <button type="button" onClick={() => navigate(`/games/${item.gameId}`)}><Gamepad2 size={13} />{item.gameTitle}</button>}
                    {item.deviceName && <span>{item.deviceName.includes('Thor') ? <Smartphone size={13} /> : <Monitor size={13} />}{item.deviceName}</span>}
                  </div>
                </div>
              </article>
            )
          })}
        </div>
      </section>
    </>
  )
}
