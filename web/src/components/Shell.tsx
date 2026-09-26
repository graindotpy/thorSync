import { History, Inbox, Library, Server, SlidersHorizontal } from 'lucide-react'
import type { ReactNode } from 'react'
import type { SystemHealth } from '../lib/present'
import { Brand } from './Brand'

interface ShellProps {
  children: ReactNode
  path: string
  navigate: (path: string) => void
  reviewCount: number
  isDemo: boolean
  liveConnected: boolean
  health: SystemHealth
}

const navigation = [
  { label: 'Library', path: '/', icon: Library },
  { label: 'Review', path: '/review', icon: Inbox, badge: true },
  { label: 'Activity', path: '/activity', icon: History },
  { label: 'System', path: '/system', icon: Server },
  { label: 'Settings', path: '/settings', icon: SlidersHorizontal },
]

function isActive(current: string, target: string) {
  if (target === '/') return current === '/' || current.startsWith('/games/')
  return current.startsWith(target)
}

export function Shell({ children, path, navigate, reviewCount, isDemo, liveConnected, health }: ShellProps) {
  const connection = isDemo ? 'Demo data' : liveConnected ? 'Live' : 'Reconnecting'
  const statusTone = health.state !== 'healthy' ? health.state : isDemo || !liveConnected ? 'degraded' : 'healthy'

  return (
    <div className="app">
      <a className="skip-link" href="#main">Skip to content</a>
      <header className="masthead">
        <div className="masthead__inner">
          <button type="button" className="masthead__brand" onClick={() => navigate('/')} aria-label="ThorSync library">
            <Brand />
          </button>
          <nav className="masthead__nav" aria-label="Main navigation">
            {navigation.map((item) => (
              <button
                type="button"
                key={item.path}
                className="masthead__link"
                aria-current={isActive(path, item.path) ? 'page' : undefined}
                onClick={() => navigate(item.path)}
              >
                {item.label}
                {item.badge && reviewCount > 0 && <span className="count-badge" aria-label={`${reviewCount} waiting`}>{reviewCount}</span>}
              </button>
            ))}
          </nav>
          <button type="button" className={`masthead__status masthead__status--${statusTone}`} onClick={() => navigate('/system')} title="Open system status">
            <span className="state__dot" aria-hidden="true" />
            <span className="masthead__status-text">{connection}<span aria-hidden="true"> · </span>{health.label}</span>
          </button>
        </div>
      </header>

      {isDemo && (
        <div className="notice-bar" role="status">
          The ThorSync API isn’t reachable, so you’re looking at demo data. Nothing you do here changes real saves.
        </div>
      )}
      {!isDemo && !liveConnected && (
        <div className="notice-bar notice-bar--quiet" role="status">
          Live updates are reconnecting. The figures below are still current as of the last refresh.
        </div>
      )}

      <main id="main" className="page">{children}</main>

      <nav className="tabbar" aria-label="Mobile navigation">
        {navigation.map((item) => {
          const Icon = item.icon
          return (
            <button
              key={item.path}
              type="button"
              className="tabbar__item"
              aria-current={isActive(path, item.path) ? 'page' : undefined}
              onClick={() => navigate(item.path)}
            >
              <span className="tabbar__icon">
                <Icon size={20} strokeWidth={1.6} />
                {item.badge && reviewCount > 0 && <i>{reviewCount}</i>}
              </span>
              <span>{item.label}</span>
            </button>
          )
        })}
      </nav>
    </div>
  )
}
