import { useMemo, useState } from 'react'
import { PageHeader } from '../components/PageHeader'
import { fullDate } from '../lib/format'
import { clockTime, dayLabel } from '../lib/present'
import type { ActivityItem } from '../types'

interface ActivityPageProps {
  items: ActivityItem[]
  navigate: (path: string) => void
}

const typeLabels: Record<ActivityItem['type'], string> = {
  capture: 'Archived',
  delivery: 'Delivery',
  conflict: 'Conflict',
  restore: 'Restore',
  missing: 'Missing',
  system: 'System',
}

type Filter = 'all' | 'saves' | 'system'

export function ActivityPage({ items, navigate }: ActivityPageProps) {
  const [filter, setFilter] = useState<Filter>('all')

  const days = useMemo(() => {
    const filtered = items
      .filter((item) => (filter === 'system' ? item.type === 'system' : filter === 'saves' ? item.type !== 'system' : true))
      .sort((a, b) => new Date(b.occurredAt).getTime() - new Date(a.occurredAt).getTime())
    const groups: { label: string; items: ActivityItem[] }[] = []
    for (const item of filtered) {
      const label = dayLabel(item.occurredAt)
      const current = groups[groups.length - 1]
      if (current?.label === label) current.items.push(item)
      else groups.push({ label, items: [item] })
    }
    return groups
  }, [filter, items])

  return (
    <>
      <PageHeader
        title="Activity"
        description="Every save ThorSync archived, delivered, held back or restored, newest first."
        actions={
          <div className="segmented" role="group" aria-label="Filter activity">
            {(['all', 'saves', 'system'] as const).map((value) => (
              <button type="button" key={value} aria-pressed={filter === value} onClick={() => setFilter(value)}>
                {value === 'all' ? 'Everything' : value === 'saves' ? 'Saves' : 'System'}
              </button>
            ))}
          </div>
        }
      />
      {days.length === 0 ? (
        <div className="empty empty--quiet"><h3>Nothing recorded yet</h3><p>Events appear here as soon as either device writes a save.</p></div>
      ) : (
        days.map((day) => (
          <section className="log-day" key={day.label} aria-label={day.label}>
            <h2 className="log-day__label">{day.label}</h2>
            <ol className="log">
              {day.items.map((item) => (
                <li className={`log__entry log__entry--${item.tone}`} key={item.id}>
                  <time dateTime={item.occurredAt} title={fullDate(item.occurredAt)}>{clockTime(item.occurredAt)}</time>
                  <span className="log__type">{typeLabels[item.type]}</span>
                  <div className="log__body">
                    <strong>{item.title}</strong>
                    <p>{item.detail}</p>
                    {(item.gameTitle || item.deviceName) && (
                      <p className="log__refs">
                        {item.gameTitle && item.gameId
                          ? <button type="button" className="link-button" onClick={() => navigate(`/games/${encodeURIComponent(item.gameId!)}`)}>{item.gameTitle}</button>
                          : item.gameTitle && <span>{item.gameTitle}</span>}
                        {item.deviceName && <span>{item.deviceName}</span>}
                      </p>
                    )}
                  </div>
                </li>
              ))}
            </ol>
          </section>
        ))
      )}
    </>
  )
}
