import {
  Activity,
  AlertTriangle,
  Gamepad2,
  FileQuestion,
  HardDrive,
  HeartPulse,
  Menu,
  PanelLeftClose,
  Settings,
  ShieldCheck,
  X,
} from 'lucide-react'
import { useEffect, useState, type ReactNode } from 'react'
import { Brand } from './Brand'

interface ShellProps {
  children: ReactNode
  path: string
  navigate: (path: string) => void
  conflictCount: number
  isDemo: boolean
  liveConnected: boolean
}

const navigation = [
  { label: 'Library', path: '/', icon: Gamepad2 },
  { label: 'Activity', path: '/activity', icon: Activity },
  { label: 'Conflicts', path: '/conflicts', icon: AlertTriangle, badge: true },
  { label: 'Devices', path: '/devices', icon: HardDrive },
  { label: 'Unassigned', path: '/unassigned', icon: FileQuestion },
  { label: 'Diagnostics', path: '/diagnostics', icon: HeartPulse },
]

function isActive(current: string, target: string) {
  if (target === '/') return current === '/' || current.startsWith('/games/')
  return current.startsWith(target)
}

export function Shell({
  children,
  path,
  navigate,
  conflictCount,
  isDemo,
  liveConnected,
}: ShellProps) {
  const [menuOpen, setMenuOpen] = useState(false)
  const [sidebarCompact, setSidebarCompact] = useState(false)

  useEffect(() => setMenuOpen(false), [path])

  const go = (next: string) => {
    navigate(next)
    setMenuOpen(false)
  }

  return (
    <div className={`app-shell ${sidebarCompact ? 'app-shell--compact' : ''}`}>
      <aside className={`sidebar ${menuOpen ? 'sidebar--open' : ''}`}>
        <div className="sidebar__top">
          <Brand compact={sidebarCompact} />
          <button
            className="icon-button sidebar__close-mobile"
            type="button"
            aria-label="Close navigation"
            onClick={() => setMenuOpen(false)}
          >
            <X size={20} />
          </button>
        </div>

        <nav className="sidebar__nav" aria-label="Main navigation">
          {navigation.map((item) => {
            const Icon = item.icon
            return (
              <button
                type="button"
                key={item.path}
                className={`nav-item ${isActive(path, item.path) ? 'nav-item--active' : ''}`}
                onClick={() => go(item.path)}
                aria-current={isActive(path, item.path) ? 'page' : undefined}
                title={sidebarCompact ? item.label : undefined}
              >
                <Icon size={19} />
                <span>{item.label}</span>
                {item.badge && conflictCount > 0 && (
                  <span className="nav-item__badge">{conflictCount}</span>
                )}
              </button>
            )
          })}
        </nav>

        <div className="sidebar__footer">
          <button
            type="button"
            className="nav-item"
            onClick={() => go('/settings')}
            title={sidebarCompact ? 'Settings' : undefined}
          >
            <Settings size={19} />
            <span>Settings</span>
          </button>
          <div className="sidebar__server" title="ZimaOS server status">
            <span className="status-light status-light--ok" />
            {!sidebarCompact && (
              <span>
                <strong>ZimaOS</strong>
                <small>Broker is healthy</small>
              </span>
            )}
          </div>
          <button
            type="button"
            className="sidebar__collapse"
            onClick={() => setSidebarCompact((value) => !value)}
            aria-label={sidebarCompact ? 'Expand sidebar' : 'Collapse sidebar'}
          >
            <PanelLeftClose size={17} />
            {!sidebarCompact && <span>Collapse</span>}
          </button>
        </div>
      </aside>

      {menuOpen && <button className="sidebar-scrim" aria-label="Close menu" onClick={() => setMenuOpen(false)} />}

      <div className="app-main">
        <header className="mobile-header">
          <button
            type="button"
            className="icon-button"
            aria-label="Open navigation"
            onClick={() => setMenuOpen(true)}
          >
            <Menu size={21} />
          </button>
          <Brand />
          <span className={`live-pill ${liveConnected && !isDemo ? 'live-pill--connected' : ''}`}>
            <span />
            {isDemo ? 'Demo' : liveConnected ? 'Live' : 'Polling'}
          </span>
        </header>

        {(isDemo || !liveConnected) && (
          <div className="connection-banner" role="status">
            {isDemo ? (
              <>
                <ShieldCheck size={15} /> Demo data is shown while the ThorSync API is unavailable.
              </>
            ) : (
              <>
                <span className="status-light status-light--warning" /> Live updates reconnecting; data remains available.
              </>
            )}
          </div>
        )}

        <main className="page-wrap">{children}</main>
      </div>

      <nav className="bottom-nav" aria-label="Mobile navigation">
        {navigation.filter((item) => item.path !== '/unassigned').slice(0, 5).map((item) => {
          const Icon = item.icon
          return (
            <button
              key={item.path}
              type="button"
              className={isActive(path, item.path) ? 'bottom-nav__item--active' : ''}
              onClick={() => go(item.path)}
            >
              <span className="bottom-nav__icon">
                <Icon size={19} />
                {item.badge && conflictCount > 0 && <i>{conflictCount}</i>}
              </span>
              <small>{item.label}</small>
            </button>
          )
        })}
      </nav>
    </div>
  )
}
